import { useMemo } from "react";
import type { NodeRef } from "@/shared/api/graph";
import type {
  GraphEdge,
  GraphNode,
  GraphResponse,
  SubgraphNode,
} from "@/shared/contracts/graph";
import type { FetchState } from "@/shared/lib/fetchState";

type NodeId = string;

/**
 * 同じアカウントのノードをまとめた応答。`subgraph` は図と Nodes・Edges の表に渡す応答である。
 *
 * - `representativeOf`: まとめたノードの id から代表の id を探す表。まとめたノードの組の全員
 *   (代表を含む) を要素にする。まとめなかったノードを要素にしない。
 * - `representativeByKey`: アカウントの鍵から代表の id を探す表。1 ノードだけの鍵も要素にする。
 * - `membersOf`: 代表の id から、組の全員の id を昇順で探す表。
 * - `loopEdgesOf`: 代表の id から、まとめて両端が代表になったため図と Edges から外したエッジを探す表。
 */
export type AccountMerge = {
  subgraph: GraphResponse;
  representativeOf: Map<NodeId, NodeId>;
  representativeByKey: Map<string, NodeId>;
  membersOf: Map<NodeId, NodeId[]>;
  loopEdgesOf: Map<NodeId, GraphEdge[]>;
};

/** 同じアカウントとしてまとめる鍵の文字列。鍵を持たないノードでは undefined である。 */
export function accountMergeKey(node: GraphNode): string | undefined {
  const key = node.accountName;
  return key === undefined ? undefined : `${key.caseId ?? ""}\u0000${key.name}`;
}

/**
 * 同じ `accountName` を持つノードを 1 つにまとめる。
 *
 * 代表は、名前のノードのうち id が最小のものである。名前のノードが無ければ、SID のノードのうち
 * id が最小のものである。エッジは端点を代表へ付け替えるだけであり、id・根拠の件数・状態を
 * 変えず、1 本にまとめない。付け替えで両端が同じ代表になったエッジは `loopEdgesOf` へ移す。
 * 元から両端が同じノードのエッジは残す。第 2 引数に代表 id を指定すると、同じ鍵の組をその id に揃える。
 */
export function mergeSameAccounts(
  response: GraphResponse,
  preferredRepresentatives?: ReadonlyMap<string, NodeId>,
): AccountMerge {
  const groups = new Map<string, SubgraphNode[]>();
  for (const node of response.nodes) {
    const key = accountMergeKey(node);
    if (key === undefined) continue;
    const group = groups.get(key);
    if (group === undefined) groups.set(key, [node]);
    else group.push(node);
  }
  const representativeOf = new Map<NodeId, NodeId>();
  const representativeByKey = new Map<string, NodeId>(preferredRepresentatives);
  const membersOf = new Map<NodeId, NodeId[]>();
  const mergedNodes = new Map<NodeId, SubgraphNode>();
  for (const [key, group] of groups) {
    const localRepresentative = representativeIn(group);
    const representativeId =
      preferredRepresentatives?.get(key) ?? localRepresentative.id;
    representativeByKey.set(key, representativeId);
    if (group.length < 2 && representativeId === localRepresentative.id)
      continue;
    const members = group.map((node) => node.id).sort(compareIds);
    if (group.length > 1) membersOf.set(representativeId, members);
    for (const id of members) representativeOf.set(id, representativeId);
    mergedNodes.set(localRepresentative.id, {
      ...localRepresentative,
      id: representativeId,
      selection: group.some((node) => node.selection === "matched")
        ? "matched"
        : localRepresentative.selection,
    });
  }
  if (representativeOf.size === 0) {
    return {
      subgraph: response,
      representativeOf,
      representativeByKey,
      membersOf,
      loopEdgesOf: new Map(),
    };
  }
  const nodes = response.nodes.flatMap((node) => {
    const representative = representativeOf.get(node.id);
    if (representative === undefined) return [node];
    const merged = mergedNodes.get(node.id);
    return merged === undefined ? [] : [merged];
  });
  const loopEdgesOf = new Map<NodeId, GraphEdge[]>();
  const edges: GraphEdge[] = [];
  for (const edge of response.edges) {
    const source = representativeOf.get(edge.sourceNodeId) ?? edge.sourceNodeId;
    const target = representativeOf.get(edge.targetNodeId) ?? edge.targetNodeId;
    if (source === target && edge.sourceNodeId !== edge.targetNodeId) {
      const loops = loopEdgesOf.get(source);
      if (loops === undefined) loopEdgesOf.set(source, [edge]);
      else loops.push(edge);
      continue;
    }
    edges.push(
      source === edge.sourceNodeId && target === edge.targetNodeId
        ? edge
        : { ...edge, sourceNodeId: source, targetNodeId: target },
    );
  }
  return {
    subgraph: {
      ...response,
      nodes,
      edges,
      edgeCount: edges.length,
    },
    representativeOf,
    representativeByKey,
    membersOf,
    loopEdgesOf,
  };
}

function compareIds(left: string, right: string): number {
  return left < right ? -1 : left > right ? 1 : 0;
}

function representativeIn(group: readonly SubgraphNode[]): SubgraphNode {
  const named = group.filter((node) => node.keyForm === "account_domain_name");
  const pool = named.length > 0 ? named : group;
  return pool.reduce((least, node) =>
    compareIds(node.id, least.id) < 0 ? node : least,
  );
}

/**
 * 切り替えが真のとき、読み込んだ応答と背景の応答をまとめる。要求を取り直さない。
 * 切り替えが偽のときは、受け取った値をそのまま返す。
 */
export function useAccountMerge(
  state: FetchState<GraphResponse>,
  backdrop: GraphResponse | undefined,
  enabled: boolean,
): {
  merge: AccountMerge | undefined;
  shownState: FetchState<GraphResponse>;
  shownBackdrop: GraphResponse | undefined;
} {
  const merge = useMemo(
    () =>
      enabled && state.status === "loaded"
        ? mergeSameAccounts(state.value)
        : undefined,
    [enabled, state],
  );
  const shownState = useMemo<FetchState<GraphResponse>>(
    () =>
      merge === undefined ? state : { status: "loaded", value: merge.subgraph },
    [merge, state],
  );
  const shownBackdrop = useMemo(
    () =>
      enabled && backdrop !== undefined
        ? mergeSameAccounts(backdrop, merge?.representativeByKey).subgraph
        : backdrop,
    [enabled, backdrop, merge],
  );
  return { merge, shownState, shownBackdrop };
}

/** 選んだノードの id を、図と表に当てる id へ読み替える。 */
export function shownNodeId(
  merge: AccountMerge | undefined,
  id: NodeId | undefined,
): NodeId | undefined {
  return id === undefined ? undefined : (merge?.representativeOf.get(id) ?? id);
}

/**
 * 関係先を足す操作の起点を返す。まとめたノードは組の全員を起点にする。`nodes` は
 * まとめる前の応答のノードであり、表示名を探す。
 */
export function originsOf(
  merge: AccountMerge | undefined,
  node: NodeRef,
  nodes: readonly GraphNode[],
): NodeRef[] {
  const representative = merge?.representativeOf.get(node.id);
  const members =
    representative === undefined
      ? undefined
      : merge?.membersOf.get(representative);
  if (members === undefined) return [node];
  return members.map((id) => {
    if (id === node.id) return node;
    const found = nodes.find((candidate) => candidate.id === id);
    return {
      id,
      label: found?.label.rawText ?? found?.label.normalized ?? id,
      kind: found?.kind,
    };
  });
}

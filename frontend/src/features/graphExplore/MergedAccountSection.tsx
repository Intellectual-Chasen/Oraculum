import type {
  AccountNameWithheldReason,
  GraphEdge,
  GraphNode,
} from "@/shared/contracts/graph";
import { formatCount } from "@/shared/lib/format";
import { edgeKindLabels } from "@/shared/lib/graphLabels";
import { Fold } from "@/shared/ui/Fold";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import { ValueLink } from "@/shared/ui/ValueLink";
import { type AccountMerge, accountMergeKey } from "./accountMerge";
import { nodeKeyFormLabels } from "./labels";

/** アカウントのノードを同じアカウントとしてまとめない理由の表示。 */
export const accountNameWithheldLabels: Record<
  AccountNameWithheldReason,
  string
> = {
  no_name_recorded: "名前の記録なし",
  multiple_names: "名前が 2 つ以上",
  multiple_cases: "案件が 2 つ以上",
};

const accountNameWithheldDetails: Record<AccountNameWithheldReason, string> = {
  no_name_recorded: "SID と共にドメインとログイン名を記録したレコードが無い",
  multiple_names: "SID と共に記録したドメインとログイン名が 2 組以上",
  multiple_cases: "根拠のレコードが 2 つ以上の案件にある",
};

function labelOf(node: GraphNode | undefined, id: string): string {
  return node?.label.rawText ?? node?.label.normalized ?? id;
}

/**
 * 同じアカウントとしてまとめたノードの節。切り替えが真で、選んだノードがアカウントのときだけ出す。
 *
 * - まとめた組の全員への link と、組の間のエッジ (図と Edges から外したエッジ) の一覧。
 * - 同じ鍵を持ちながら応答に入っていないノードの数。
 * - まとめなかったノードでは、その理由。
 *
 * `nodes` はまとめる前の応答のノードである。
 */
export function MergedAccountSection({
  enabled,
  node,
  merge,
  nodes,
}: {
  enabled: boolean;
  node: GraphNode | undefined;
  merge: AccountMerge | undefined;
  nodes: readonly GraphNode[];
}) {
  if (!enabled || node?.kind !== "account") return null;
  const key = accountMergeKey(node);
  const representative = merge?.representativeOf.get(node.id);
  const members =
    representative === undefined
      ? []
      : (merge?.membersOf.get(representative) ?? []);
  const loops =
    representative === undefined
      ? []
      : (merge?.loopEdgesOf.get(representative) ?? []);
  const nodeOf = (id: string) => nodes.find((candidate) => candidate.id === id);
  const inResponse =
    key === undefined
      ? 0
      : nodes.filter((candidate) => accountMergeKey(candidate) === key).length;
  const outside =
    node.accountName === undefined
      ? 0
      : Math.max(0, node.accountName.nodeCount - inResponse);
  const withheld = node.accountNameWithheld;
  return (
    <section aria-labelledby="merged-account-heading">
      <h3 id="merged-account-heading">まとめたノード</h3>
      <KeyValueList
        pairs={[
          {
            name: "件数",
            value:
              members.length === 0 ? undefined : formatCount(members.length),
          },
          {
            name: "応答に無いノード",
            value:
              node.accountName === undefined ? undefined : (
                <Hint text="同じドメインとログイン名を持ち、グラフの応答に無いノード">
                  {formatCount(outside)}
                </Hint>
              ),
          },
          {
            name: "まとめない理由",
            value:
              withheld === undefined ? undefined : (
                <Hint text={accountNameWithheldDetails[withheld]}>
                  {accountNameWithheldLabels[withheld]}
                </Hint>
              ),
          },
        ]}
      />
      {members.length === 0 ? null : (
        <table>
          <caption className="sr-only">まとめたノード</caption>
          <thead>
            <tr>
              <th scope="col">ノード</th>
              <th scope="col">同一性の基準</th>
            </tr>
          </thead>
          <tbody>
            {members.map((id) => {
              const member = nodeOf(id);
              const label = labelOf(member, id);
              return (
                <tr key={id}>
                  <td>
                    <ValueLink
                      target={{ kind: "node", id, label }}
                      hover="Node Detail に表示"
                    >
                      <RawText text={label} />
                    </ValueLink>
                  </td>
                  <td>
                    {member === undefined
                      ? null
                      : nodeKeyFormLabels[member.keyForm]}
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      )}
      {loops.length === 0 ? null : (
        <Fold
          summary="まとめたノードの間のエッジ"
          summaryNote={`本数: ${formatCount(loops.length)}`}
          rowCount={loops.length}
        >
          <LoopEdgeTable edges={loops} nodeOf={nodeOf} />
        </Fold>
      )}
    </section>
  );
}

function LoopEdgeTable({
  edges,
  nodeOf,
}: {
  edges: readonly GraphEdge[];
  nodeOf: (id: string) => GraphNode | undefined;
}) {
  return (
    <table>
      <caption className="sr-only">まとめたノードの間のエッジ</caption>
      <thead>
        <tr>
          <th scope="col">エッジの種類</th>
          <th scope="col">始点</th>
          <th scope="col">終点</th>
          <th scope="col">根拠</th>
        </tr>
      </thead>
      <tbody>
        {edges.map((edge) => (
          <tr key={edge.id}>
            <td>
              <ValueLink
                target={{ kind: "edge", id: edge.id }}
                hover="Edge Detail に表示"
              >
                {edgeKindLabels[edge.kind]}
              </ValueLink>
            </td>
            <td>
              <RawText
                text={labelOf(nodeOf(edge.sourceNodeId), edge.sourceNodeId)}
              />
            </td>
            <td>
              <RawText
                text={labelOf(nodeOf(edge.targetNodeId), edge.targetNodeId)}
              />
            </td>
            <td className="text-right">{formatCount(edge.evidenceCount)}</td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

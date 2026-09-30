import { expect, test } from "vitest";
import { decodeGraphResponse, type GraphEdge } from "@/shared/contracts/graph";
import {
  accountMergeKey,
  mergeSameAccounts,
  originsOf,
  shownNodeId,
} from "./accountMerge";
import { buildSubgraphDrawing, withBackdrop } from "./subgraph";

// 識別子。名前のノード 2 つと SID のノード 2 つが同じ鍵を持つ。
const nameUpper = "n:account:b2";
const nameLower = "n:account:b1";
const oldSid = "n:account:a1";
const newSid = "n:account:a2";
const renamedSid = "n:account:c1";
const terminal = "n:terminal:t1";
const accountName = { name: "host-q\\user-q", nodeCount: 5 };

function account(
  id: string,
  keyForm: "account_sid" | "account_domain_name",
  extra: Record<string, unknown>,
  selection = "matched",
) {
  return {
    id,
    kind: "account",
    keyForm,
    identity:
      keyForm === "account_sid"
        ? [{ semantic: "account.sid", value: `S-1-5-21-1-${id}` }]
        : [
            { semantic: "account.domain", value: "HOST-Q" },
            { semantic: "account.name", value: id },
          ],
    label: { rawText: id, valueState: "present" },
    observation: "observed",
    creationRecord: "absent",
    selection,
    ...extra,
  };
}

function edge(
  id: string,
  kind: GraphEdge["kind"],
  source: string,
  target: string,
): GraphEdge {
  return {
    id,
    kind,
    state: "observed",
    sourceNodeId: source,
    targetNodeId: target,
    evidenceCount: 3,
  };
}

function response() {
  const edges = [
    edge("e:1", "account_identity_match", nameLower, oldSid),
    edge("e:2", "account_identity_match", nameUpper, newSid),
    edge("e:3", "terminal_account", terminal, oldSid),
    edge("e:4", "terminal_account", terminal, newSid),
    edge("e:5", "terminal_account", terminal, renamedSid),
  ];
  return decodeGraphResponse(
    {
      nodes: [
        {
          id: terminal,
          kind: "terminal",
          keyForm: "terminal_id",
          identity: [{ semantic: "terminal.id", value: "T1" }],
          label: { rawText: "HOST-Q", valueState: "present" },
          observation: "observed",
          creationRecord: "item_absent",
          selection: "matched",
        },
        account(newSid, "account_sid", { accountName }),
        account(nameUpper, "account_domain_name", { accountName }),
        account(oldSid, "account_sid", { accountName }, "edge_endpoint"),
        account(nameLower, "account_domain_name", { accountName }),
        account(renamedSid, "account_sid", {
          accountNameWithheld: "multiple_names",
        }),
      ],
      nodeCount: 5,
      matchedKinds: [
        { kind: "account", count: 4 },
        { kind: "terminal", count: 1 },
      ],
      subgraphNodeCount: 6,
      edges,
      edgeCount: edges.length,
      depth: 1,
    },
    "$",
  );
}

test("名前のノードのうち id が最小のものを代表にし、組の全員を代表へ読み替える", () => {
  const merge = mergeSameAccounts(response());
  expect(merge.membersOf.get(nameLower)).toEqual([
    oldSid,
    newSid,
    nameLower,
    nameUpper,
  ]);
  for (const id of [oldSid, newSid, nameLower, nameUpper]) {
    expect(shownNodeId(merge, id)).toBe(nameLower);
  }
  // 名前を変えた SID と端末はまとめない。
  expect(shownNodeId(merge, renamedSid)).toBe(renamedSid);
  expect(shownNodeId(merge, terminal)).toBe(terminal);
  expect(merge.subgraph.nodes.map((node) => node.id)).toEqual([
    terminal,
    nameLower,
    renamedSid,
  ]);
});

test("名前のノードが無い組は、SID のノードのうち id が最小のものを代表にする", () => {
  const base = response();
  const merge = mergeSameAccounts({
    ...base,
    nodes: base.nodes.filter((node) => node.keyForm !== "account_domain_name"),
  });
  expect(merge.membersOf.get(oldSid)).toEqual([oldSid, newSid]);
  expect(shownNodeId(merge, newSid)).toBe(oldSid);
});

test("エッジの id の集合をまとめる前と後で保ち、組の間のエッジを外して代表の一覧に置く", () => {
  const before = response();
  const merge = mergeSameAccounts(before);
  const loops = merge.loopEdgesOf.get(nameLower) ?? [];
  expect(loops.map((item) => item.id)).toEqual(["e:1", "e:2"]);
  const after = [...merge.subgraph.edges, ...loops].map((item) => item.id);
  expect(new Set(after)).toEqual(new Set(before.edges.map((item) => item.id)));
  expect(after).toHaveLength(before.edges.length);
  // 端点を付け替えるだけで、1 本にまとめず、根拠の件数と状態を保つ。
  const shown = merge.subgraph.edges.filter(
    (item) => item.targetNodeId === nameLower,
  );
  expect(shown.map((item) => item.id)).toEqual(["e:3", "e:4"]);
  for (const item of shown) {
    expect(item.sourceNodeId).toBe(terminal);
    expect(item.evidenceCount).toBe(3);
    expect(item.state).toBe("observed");
  }
  expect(merge.subgraph.edgeCount).toBe(merge.subgraph.edges.length);
  // まとめる前の応答を書き換えない。
  expect(before.edges[2]?.targetNodeId).toBe(oldSid);
});

test("エッジの端のノードをまとめても、組に一致ノードがあれば一致ノードとして出す", () => {
  const merge = mergeSameAccounts(response());
  expect(
    merge.subgraph.nodes.find((node) => node.id === nameLower)?.selection,
  ).toBe("matched");
});

test("関係先を足す起点は、まとめた組の全員にする", () => {
  const before = response();
  const merge = mergeSameAccounts(before);
  const origins = originsOf(
    merge,
    { id: nameLower, label: nameLower },
    before.nodes,
  );
  expect(origins.map((origin) => origin.id)).toEqual([
    oldSid,
    newSid,
    nameLower,
    nameUpper,
  ]);
  expect(
    originsOf(merge, { id: renamedSid, label: "x" }, before.nodes),
  ).toEqual([{ id: renamedSid, label: "x" }]);
  expect(
    originsOf(undefined, { id: nameLower, label: "x" }, before.nodes),
  ).toHaveLength(1);
});

test("鍵を持つノードが 1 つも組にならない応答は、そのまま返す", () => {
  const base = response();
  const single = {
    ...base,
    nodes: base.nodes.filter((node) =>
      [terminal, oldSid, renamedSid].includes(node.id),
    ),
  };
  const merge = mergeSameAccounts(single);
  expect(merge.subgraph).toBe(single);
  expect(merge.representativeOf.size).toBe(0);
});

test("前景と背景で構成員が異なる組は、前景の代表 id で 1 つに描く", () => {
  const base = response();
  const focused = {
    ...base,
    nodes: base.nodes.filter((node) => node.id === oldSid),
    edges: [],
    edgeCount: 0,
  };
  const background = {
    ...base,
    nodes: base.nodes.filter((node) =>
      [terminal, nameLower, newSid].includes(node.id),
    ),
    edges: [edge("e:background", "terminal_account", terminal, nameLower)],
    edgeCount: 1,
  };
  const merge = mergeSameAccounts(focused);
  const shownBackdrop = mergeSameAccounts(
    background,
    merge.representativeByKey,
  ).subgraph;
  const focusedAccount = focused.nodes.find((node) => node.id === oldSid);

  expect(focusedAccount).toBeDefined();
  if (focusedAccount === undefined) return;
  const key = accountMergeKey(focusedAccount);
  expect(key).toBeDefined();
  if (key === undefined) return;
  expect(merge.representativeByKey.get(key)).toBe(oldSid);

  const drawing = withBackdrop(
    buildSubgraphDrawing(merge.subgraph),
    buildSubgraphDrawing(shownBackdrop),
  );
  expect(drawing.points.map((point) => point.id)).toEqual([oldSid, terminal]);
  expect(
    drawing.links.map((link) => [link.sourceNodeId, link.targetNodeId]),
  ).toEqual([[terminal, oldSid]]);
});

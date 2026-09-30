// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { GraphEdge, GraphResponse } from "@/shared/contracts/graph";
import { mergeSameAccounts } from "./accountMerge";
import { MergedAccountSection } from "./MergedAccountSection";

afterEach(cleanup);

const accountName = { name: "host-q\\user-q", nodeCount: 3 };

function account(
  id: string,
  keyForm: "account_sid" | "account_domain_name",
  extra: Partial<GraphResponse["nodes"][number]> = {},
): GraphResponse["nodes"][number] {
  return {
    id,
    kind: "account",
    keyForm,
    identity: [{ value: id }],
    label: { rawText: `label-${id}`, valueState: "present" },
    observation: "observed",
    creationRecord: "absent",
    selection: "matched",
    ...extra,
  };
}

const loop: GraphEdge = {
  id: "e:loop",
  kind: "account_identity_match",
  state: "candidate",
  sourceNodeId: "n:b",
  targetNodeId: "n:a",
  evidenceCount: 2,
};

function response(): GraphResponse {
  return {
    nodes: [
      account("n:a", "account_sid", { accountName }),
      account("n:b", "account_domain_name", { accountName }),
      account("n:c", "account_sid", { accountNameWithheld: "multiple_names" }),
    ],
    nodeCount: 3,
    matchedKinds: [{ kind: "account", count: 3 }],
    subgraphNodeCount: 3,
    edges: [loop],
    edgeCount: 1,
    nodeLimitExceeded: false,
  } as GraphResponse;
}

test("まとめたノードごとの link と、外した組の間のエッジと、応答に無いノードの数を出す", () => {
  const graph = response();
  const merge = mergeSameAccounts(graph);
  render(
    <MergedAccountSection
      enabled
      node={graph.nodes[0]}
      merge={merge}
      nodes={graph.nodes}
    />,
  );
  const section = screen.getByRole("region", { name: "まとめたノード" });
  const members = within(section).getByRole("table", {
    name: "まとめたノード",
  });
  expect(within(members).getByText("label-n:a")).toBeInTheDocument();
  expect(within(members).getByText("label-n:b")).toBeInTheDocument();
  const loops = within(section).getByRole("table", {
    name: "まとめたノードの間のエッジ",
  });
  expect(within(loops).getAllByRole("row")).toHaveLength(2);
  // 同じ鍵を持つ 3 つのノードのうち、応答には 2 つがある。
  expect(section).toHaveTextContent("応答に無いノード: 1");
});

test("まとめなかった SID に理由を出し、切り替えが偽のときは節を出さない", () => {
  const graph = response();
  const { rerender } = render(
    <MergedAccountSection
      enabled
      node={graph.nodes[2]}
      merge={mergeSameAccounts(graph)}
      nodes={graph.nodes}
    />,
  );
  expect(
    screen.getByRole("region", { name: "まとめたノード" }),
  ).toHaveTextContent("まとめない理由: 名前が 2 つ以上");
  rerender(
    <MergedAccountSection
      enabled={false}
      node={graph.nodes[2]}
      merge={undefined}
      nodes={graph.nodes}
    />,
  );
  expect(screen.queryByRole("region", { name: "まとめたノード" })).toBeNull();
});

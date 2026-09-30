// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, expect, test } from "vitest";
import type { Timestamp } from "@/shared/contracts/common";
import type { GraphEdge, SubgraphNode } from "@/shared/contracts/graph";
import {
  type GraphTimeFilter,
  searchHighlightOf,
} from "@/shared/lib/searchHighlight";
import { noSearchTerms, type SearchTerms } from "@/shared/lib/searchTerms";
import { SearchHighlightContext } from "@/shared/ui/Highlighted";
import { SubgraphEdgeList } from "./SubgraphEdgeList";
import { SubgraphNodeList } from "./SubgraphNodeList";

afterEach(cleanup);

function hostNode(
  name: string,
  selection: SubgraphNode["selection"],
): SubgraphNode {
  return {
    id: `n:host:${name}`,
    kind: "terminal",
    keyForm: "terminal_id",
    identity: [{ semantic: "terminal.id", value: `${name}-TMID` }],
    label: { rawText: `${name}.example.test`, valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
    selection,
    valueMatches: [],
  };
}

function at(normalized: string): Timestamp {
  return {
    rawText: normalized,
    normalized,
    normalizedForm: "rfc3339_absolute",
    precision: "second",
    offsetState: "in_value",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
}

function highlighted(
  terms: SearchTerms,
  timeFilter: GraphTimeFilter | undefined,
  children: ReactNode,
) {
  return render(
    <SearchHighlightContext.Provider
      value={searchHighlightOf(terms, timeFilter)}
    >
      {children}
    </SearchHighlightContext.Provider>,
  );
}

const nodeActions = {
  onOpenDetail: () => {},
  onAddNeighbours: () => {},
  onShowNeighbours: () => {},
  onTraceLineage: () => {},
  onNarrowToTerminal: () => {},
  onAddTerm: () => {},
};

function rowsOf(name: string) {
  return within(screen.getByRole("table", { name }))
    .getAllByRole("row")
    .slice(1);
}

test("Nodes は一致ノードの行に印を付け、表示名と同一性の値の一致を mark で包む", () => {
  highlighted(
    { ...noSearchTerms, contains: ["alpha"] },
    undefined,
    <SubgraphNodeList
      nodes={[hostNode("alpha", "matched"), hostNode("bravo", "edge_endpoint")]}
      selectedNodeId={undefined}
      originNodeIds={[]}
      onSelect={() => {}}
      onExpand={() => {}}
      menuActions={nodeActions}
    />,
  );
  const [alpha, bravo] = rowsOf("グラフのノードの一覧");
  expect(alpha).toHaveClass("search-matched-row");
  expect(bravo).not.toHaveClass("search-matched-row");
  expect(
    [...(alpha?.querySelectorAll("mark") ?? [])].map(
      (mark) => mark.textContent,
    ),
  ).toEqual(["alpha", "alpha"]);
});

test("Edges は適用期間が重なる行と検索語に一致する端点を強調する", () => {
  const nodes = [hostNode("alpha", "matched"), hostNode("bravo", "matched")];
  const edge = (id: string, from: string, to: string): GraphEdge => ({
    id,
    kind: "ran_on",
    state: "observed",
    sourceNodeId: "n:host:alpha",
    targetNodeId: "n:host:bravo",
    applicableRange: { from: at(from), to: at(to) },
    evidenceCount: 1,
  });
  highlighted(
    { ...noSearchTerms, fieldContains: ["terminal.id=TMID"] },
    {
      from: { text: "2031-10-08T10:00:00Z" },
      to: { text: "2031-10-08T11:00:00Z" },
      unit: "second",
    },
    <SubgraphEdgeList
      edges={[
        edge("e:inside", "2031-10-08T10:59:59Z", "2031-10-08T12:00:00Z"),
        edge("e:outside", "2031-10-08T11:00:01Z", "2031-10-08T12:00:00Z"),
      ]}
      nodes={nodes}
      selectedEdgeId={undefined}
      onSelect={() => {}}
      menuActions={{
        onSelectEdge: () => {},
        onOpenNodeDetail: () => {},
        onAddTerm: () => {},
      }}
    />,
  );
  const [inside, outside] = rowsOf("グラフのエッジの一覧");
  expect(inside).toHaveClass("search-matched-row");
  expect(outside).not.toHaveClass("search-matched-row");
  expect(
    [...(inside?.querySelectorAll("mark") ?? [])].map(
      (mark) => mark.textContent,
    ),
  ).toEqual(["TMID", "TMID", "2031-10-08T10:59:59Z"]);
});

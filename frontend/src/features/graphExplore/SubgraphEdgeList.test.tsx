// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { decodeGraphEdge } from "@/shared/contracts/graph";
import { SubgraphEdgeList } from "./SubgraphEdgeList";

afterEach(cleanup);

function timestampJson(text: string) {
  return {
    rawText: text,
    normalized: text,
    normalizedForm: "rfc3339_absolute",
    precision: "second",
    offsetState: "in_value",
    offsetText: "Z",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
}

/** エッジ 1 本を一覧に描き、その行を返す。 */
function renderedRow(overrides: Record<string, unknown>) {
  const edge = decodeGraphEdge(
    {
      id: "edge-assigned",
      kind: "terminal_address",
      state: "observed",
      sourceNodeId: "node-terminal",
      targetNodeId: "node-ip",
      applicableRange: {
        from: timestampJson("2001-02-03T04:05:06Z"),
        to: timestampJson("2001-02-03T05:06:07Z"),
      },
      evidenceCount: 0,
      ...overrides,
    },
    "edge",
  );
  render(
    <SubgraphEdgeList
      edges={[edge]}
      nodes={[]}
      selectedEdgeId={undefined}
      onSelect={() => {}}
      menuActions={{
        onSelectEdge: () => {},
        onOpenNodeDetail: () => {},
        onAddTerm: () => {},
      }}
    />,
  );
  return within(
    screen.getByRole("table", { name: "グラフのエッジの一覧" }),
  ).getAllByRole("row")[1];
}

/** 行の、列の見出しに対応するセル。 */
function cellOf(row: HTMLElement | undefined, header: string) {
  const table = row?.closest("table") as HTMLElement;
  const headers = within(table)
    .getAllByRole("columnheader")
    .map((th) => th.textContent);
  return row?.children[headers.indexOf(header)] as HTMLElement;
}

test("操作の列を表の左端に置き、scroll していない間はほかの列の値に重ねない", () => {
  const row = renderedRow({});
  const table = row?.closest("table") as HTMLElement;

  expect(within(table).getAllByRole("columnheader")[0]).toHaveTextContent(
    "操作",
  );
  expect(row?.children[0]).toHaveClass("row-actions");
  expect(
    within(row?.children[0] as HTMLElement).getByRole("button", {
      name: /^\S+ .+ → .+ の詳細を表示$/,
    }),
  ).toBeInTheDocument();
});

test("割当の由来と根拠のレコードを両方持つエッジは、期間を適用期間と書かない", () => {
  const row = renderedRow({
    assignmentOrigins: ["analyst_supplied"],
    evidenceCount: 2,
  });

  expect(cellOf(row, "根拠のレコード")).toHaveTextContent("2");
  expect(cellOf(row, "端末の割り当て")).toHaveTextContent("分析者");
  expect(row).not.toHaveTextContent("適用期間");
});

test("割当の由来が空の配列のエッジは、端末の割り当ての列を空にする", () => {
  const row = renderedRow({ assignmentOrigins: [], evidenceCount: 1 });

  expect(cellOf(row, "端末の割り当て")?.textContent).toBe("");
});

test("期間の両端を UTC の時刻で出し、原資料の文字列を添える", () => {
  const row = renderedRow({
    applicableRange: {
      from: {
        ...timestampJson("2001-02-03 04:05:06.123456"),
        normalized: "2001-02-03T04:05:06.123456Z",
        precision: "microsecond",
        offsetState: "format_defined",
      },
      to: {
        ...timestampJson("2001/02/03 13:06:07"),
        normalized: "2001-02-03T13:06:07",
        normalizedForm: "local_without_offset",
        offsetState: "item_absent",
        interpretation: { offset: "+09:00" },
      },
    },
  });

  expect(row).toHaveTextContent(
    "2001-02-03T04:05:06.123456Z – 2001-02-03T04:06:07Z",
  );
  // 原文は UTC の時刻の title に入れる。
  expect(
    within(row as HTMLElement).getByText("2001-02-03T04:06:07Z"),
  ).toHaveAttribute("title", "原文: 2001/02/03 13:06:07");
  // 終わりの端だけがタイムゾーンを与えて読んだ時刻であり、その端の組であることを「終わり:」で示す。
  expect(row).toHaveTextContent("終わり: 起動時の指定: UTC+09:00");
  expect(row).not.toHaveTextContent("始まり:");
  expect(row).not.toHaveTextContent("分析者の記録");
});

test("両端を異なるずれか出どころで読んだ期間は、始まりと終わりの注記を分けて出す", () => {
  const local = (
    text: string,
    normalized: string,
    interpretation: { offset: string; assertionId?: string },
  ) => ({
    ...timestampJson(text),
    normalized,
    normalizedForm: "local_without_offset",
    offsetState: "undetermined",
    interpretation,
  });
  const row = renderedRow({
    applicableRange: {
      from: local("2001/02/03 04:05:06", "2001-02-03T04:05:06", {
        offset: "+00:00",
      }),
      to: local("2001/02/03 14:06:07", "2001-02-03T14:06:07", {
        offset: "+09:00",
        assertionId: "as-1",
      }),
    },
  });
  expect(row).toHaveTextContent(
    "始まり: 起動時の指定: UTC+00:00終わり: 分析者の記録: UTC+09:00",
  );
});

test("両端を同じずれで読んだ期間は、ずれの出どころを 1 行だけ出す", () => {
  const local = (text: string, normalized: string) => ({
    ...timestampJson(text),
    normalized,
    normalizedForm: "local_without_offset",
    offsetState: "undetermined",
    interpretation: { offset: "+00:00", assertionId: "as-1" },
  });
  const row = renderedRow({
    applicableRange: {
      from: local("2001/02/03 04:05:06", "2001-02-03T04:05:06"),
      to: local("2001/02/03 05:06:07", "2001-02-03T05:06:07"),
    },
  });
  expect(
    within(row as HTMLElement).getAllByText("分析者の記録: UTC+00:00"),
  ).toHaveLength(1);
});

test("互いに区別できない候補のエッジは、区別できない候補の列に候補の数を出す", () => {
  const row = renderedRow({
    kind: "cross_source_connection_match",
    state: "candidate",
    evidenceCount: 2,
    indistinguishableCandidateCount: 2,
  });
  expect(cellOf(row, "作り方")).toHaveTextContent("推定");
  expect(cellOf(row, "区別できない候補")).toHaveTextContent("2");
  expect(() =>
    decodeGraphEdge(
      {
        id: "edge",
        kind: "cross_source_connection_match",
        state: "candidate",
        sourceNodeId: "a",
        targetNodeId: "b",
        evidenceCount: 1,
        indistinguishableCandidateCount: 1,
      },
      "edge",
    ),
  ).toThrow(/indistinguishableCandidateCount/);
});

test("ログオンの連鎖の候補は、候補の順位と順位を決めた条件を別の列に出す", () => {
  const row = renderedRow({
    kind: "logon_chain",
    state: "uncertain_chain",
    evidenceCount: 2,
    candidateTier: {
      tier: 3,
      conditions: ["session_account_match", "session_logon_network"],
    },
  });
  expect(cellOf(row, "候補の順位")).toHaveTextContent("3");
  expect(cellOf(row, "順位の条件")).toHaveTextContent(
    "アカウントが一致、ネットワークのログオン",
  );
  for (const candidateTier of [
    { tier: 0, conditions: ["session_account_match"] },
    { tier: 1, conditions: [] },
    { tier: 1, conditions: ["logon_id"] },
  ]) {
    expect(() =>
      decodeGraphEdge(
        {
          id: "edge",
          kind: "logon_chain",
          state: "candidate",
          sourceNodeId: "a",
          targetNodeId: "b",
          evidenceCount: 1,
          candidateTier,
        },
        "edge",
      ),
    ).toThrow(/candidateTier/);
  }
});

test("起動時に指定した割当から作ったエッジは、割当を示し、適用期間を期間に出す", () => {
  const row = renderedRow({ assignmentOrigins: ["import_specified"] });
  expect(cellOf(row, "端末の割り当て")).toHaveTextContent("起動時の指定");
  expect(cellOf(row, "期間")).toHaveTextContent(
    "適用期間: 2001-02-03T04:05:06Z – 2001-02-03T05:06:07Z",
  );
});

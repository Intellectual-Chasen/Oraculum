import { memo, useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import { fetchTerminals } from "@/shared/api/terminals";
import type { GraphNode } from "@/shared/contracts/graph";
import type {
  TerminalSummary,
  TerminalsResponse,
} from "@/shared/contracts/terminals";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { terminalLabel, terminalSourceOf } from "@/shared/lib/terminalSource";
import { DataTable } from "@/shared/ui/DataTable";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";

/** 端末の OS を読み取る元が無いことのラベル。OS は registry の SOFTWARE の収集元から読み取る。 */
export const operatingSystemAbsent = "OS 情報を読み取る registry の収集元なし";

/** IP アドレスの形の文字列か。IPv4 の 4 つの数と、`:` を含む IPv6 の形を見る。 */
function looksLikeAddress(name: string): boolean {
  return /^\d{1,3}(\.\d{1,3}){3}$/.test(name) || name.includes(":");
}

/**
 * 名前を比べる形。大文字と小文字をそろえる。原資料に書かれたホスト名で IP アドレスの形でない
 * ものだけは、FQDN を先頭のラベルにし、短い名前と同じ名前に数える。Oraculum が組み立てた
 * 表示名と IP アドレスは、全体で比べる。
 */
function comparableName(node: GraphNode): string {
  const label = terminalLabel(node).toLowerCase();
  if (node.label.valueState !== "present" || looksLikeAddress(label)) {
    return label;
  }
  return label.split(".")[0] ?? label;
}

/** 同じ名前を持つ 2 行以上の端末の組を、一覧の並びの順で返す。 */
function sameNameGroups(terminals: readonly TerminalSummary[]): string[][] {
  const groups = new Map<string, string[]>();
  for (const summary of terminals) {
    const label = terminalLabel(summary.node);
    const key = comparableName(summary.node);
    groups.set(key, [...(groups.get(key) ?? []), label]);
  }
  return [...groups.values()].filter((labels) => labels.length > 1);
}

/**
 * 同じ名前の端末が 2 行以上あるとき、その名前と行の数の表を出し、割り当てで 1 台に
 * まとめられることを「?」に置く。端末は収集元ごとに作るため、同じ端末の収集元が 2 つあると
 * 2 行になる。
 */
function SameNameNote({
  terminals,
}: {
  terminals: readonly TerminalSummary[];
}) {
  const groups = sameNameGroups(terminals);
  if (groups.length === 0) {
    return null;
  }
  return (
    <div>
      {/* 「?」を見出しの隣に置くため、見出しを表の外に出す。 */}
      <p className="flex items-center gap-1 font-semibold text-sm">
        同じ名前の端末
        <HelpPopover label="同じ名前の端末">
          <KeyValueList
            stacked
            pairs={[
              { name: "端末の単位", value: "収集元" },
              {
                name: "1 台へのまとめ方",
                value:
                  "Time & Host の端末の割り当てで各収集元に同じ端末の識別子を記録",
              },
            ]}
          />
        </HelpPopover>
      </p>
      {/* 組ごとに、一覧で最初の行の名前と行の数だけを出す。名前の全体は一覧の行が出す。 */}
      <DataTable
        label="同じ名前の端末"
        rows={groups}
        rowKey={(labels) => labels[0] ?? ""}
        columns={[
          {
            key: "name",
            header: "名前",
            cell: (labels) => <RawText text={labels[0] ?? ""} />,
          },
          {
            key: "rows",
            header: "行数",
            numeric: true,
            cell: (labels) => formatCount(labels.length),
          },
        ]}
      />
    </div>
  );
}

/** 表示名のほかに名乗った名前。 */
function earlierNamesOf(summary: TerminalSummary): string[] {
  const label = terminalLabel(summary.node);
  return summary.names.filter((name) => name !== label);
}

/**
 * 端末を並べ、名前・収集元・OS・レコードの件数を出す。行を選ぶと、その端末の詳細を開く。
 *
 * **memo にし、条件が変わらない限り取り直さない。**
 */
export const TerminalList = memo(function TerminalList({
  matchConditions,
  version,
  selectedTerminalId,
  onSelectTerminal,
  sourceFileNames,
}: {
  matchConditions: MatchConditionSelection;
  /** 端末の割当と時刻の解釈を記録した回数。変わるたびに取り直す。 */
  version: number;
  selectedTerminalId: string | undefined;
  onSelectTerminal: (node: GraphNode) => void;
  /** 収集元の内容の識別から file 名を探す表。探せない収集元は内容の識別を出す。 */
  sourceFileNames: ReadonlyMap<string, string>;
}) {
  const [state, setState] = useState<FetchState<TerminalsResponse>>({
    status: "loading",
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じ条件で取り直す。
  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchTerminals(matchConditions, { signal: controller.signal }).then(
      (result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        );
      },
      () => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure("unexpected", "端末の一覧の取得"),
        });
      },
    );
    return () => controller.abort();
  }, [matchConditions, version]);

  return (
    <FetchStateView state={state} loadingDescription="端末の一覧を読み込み中">
      {(response) =>
        response.terminals.length === 0 ? (
          <p role="status">端末なし</p>
        ) : (
          <>
            <div className="flex items-center gap-1">
              <KeyValueList
                pairs={[
                  {
                    name: "端末",
                    value: formatCount(response.terminals.length),
                  },
                ]}
              />
              <HelpPopover label="Hosts の端末">
                <KeyValueList
                  stacked
                  pairs={[
                    {
                      name: "表示する端末",
                      value:
                        "分類に入るレコードか registry の端末の情報を持つ端末",
                    },
                    { name: "図の端末のノードの数", value: "不一致の場合あり" },
                  ]}
                />
              </HelpPopover>
            </div>
            <SameNameNote terminals={response.terminals} />
            <DataTable
              className="node-summaries"
              label="端末"
              rows={response.terminals}
              rowKey={(summary) => summary.node.id}
              rowProps={(summary) => ({
                "aria-current":
                  summary.node.id === selectedTerminalId ? "true" : undefined,
              })}
              columns={[
                {
                  key: "name",
                  header: "名前",
                  sortValue: (summary) => terminalLabel(summary.node),
                  cell: (summary) => (
                    <button
                      type="button"
                      onClick={() => onSelectTerminal(summary.node)}
                    >
                      <RawText text={terminalLabel(summary.node)} />
                    </button>
                  ),
                },
                {
                  key: "earlier",
                  header: "以前の名前",
                  cell: (summary) => {
                    const earlier = earlierNamesOf(summary);
                    return earlier.length === 0 ? (
                      <MissingValue description="なし" />
                    ) : (
                      <RawText text={earlier.join(", ")} />
                    );
                  },
                },
                {
                  key: "source",
                  header: "収集元",
                  cell: (summary) => {
                    const source = terminalSourceOf(
                      summary.node,
                      sourceFileNames,
                    );
                    return "absence" in source ? (
                      <MissingValue description={source.absence} />
                    ) : source.title === undefined ? (
                      <RawText text={source.text} />
                    ) : (
                      <Hint text={source.title}>
                        <RawText text={source.text} />
                      </Hint>
                    );
                  },
                },
                {
                  key: "os",
                  header: "OS",
                  sortValue: (summary) => summary.operatingSystem,
                  cell: (summary) =>
                    summary.operatingSystem === undefined ? (
                      <MissingValue description={operatingSystemAbsent} />
                    ) : (
                      <RawText text={summary.operatingSystem} />
                    ),
                },
                {
                  key: "records",
                  header: "レコード数",
                  numeric: true,
                  sortValue: (summary) => summary.recordCount,
                  cell: (summary) => formatCount(summary.recordCount),
                },
                {
                  key: "categorized",
                  header: "分類したレコード数",
                  numeric: true,
                  sortValue: (summary) => summary.categorizedRecordCount,
                  cell: (summary) =>
                    formatCount(summary.categorizedRecordCount),
                },
              ]}
            />
          </>
        )
      }
    </FetchStateView>
  );
});

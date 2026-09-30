import { memo, useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  fetchNodeSummaries,
  type NodeSummariesRequest,
} from "@/shared/api/nodeSummaries";
import {
  offsetCarriesInstant,
  type Timestamp,
} from "@/shared/contracts/common";
import type {
  NodeSummariesResponse,
  NodeSummary,
} from "@/shared/contracts/nodeSummaries";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { timestampPrecisionLabels } from "@/shared/lib/recordLabels";
import { ipSortValue, timestampSortValue } from "@/shared/lib/sortValue";
import {
  distinctTerminalNames,
  terminalLabel,
} from "@/shared/lib/terminalSource";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Highlighted, PeriodMark } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import {
  interpretationOriginLabel,
  offsetUnknownDescription,
  offsetUnknownLabel,
} from "@/shared/ui/TimestampOffsetNote";
import { TimeValueLink, ValueLink } from "@/shared/ui/ValueLink";

const kindLabels = { terminal: "端末", ip: "IP アドレス" } as const;

function labelOf(summary: NodeSummary): string {
  return (
    summary.node.label.rawText ??
    summary.node.label.normalized ??
    summary.node.id
  );
}

/**
 * 最初と最後の時刻の原文の文字列を 1 行で出し、精度と、タイムゾーンの出どころを tooltip に出す。
 *
 * **タイムゾーンの出どころを添える。** 最初と最後は UTC 時刻 (分析者のタイムゾーンの指定を含む)
 * で選ぶ。原文の文字列だけを並べると、地方時の文字列と UTC の文字列が並んだときに、最初の時刻が
 * 最後の時刻より後に見える。UTC に直せない時刻は、その印を同じ行に出す。
 */
function TimeCell({ time }: { time: Timestamp | undefined }) {
  if (time === undefined) {
    return <MissingValue description="UTC 時刻を持つレコードなし" />;
  }
  const details = [
    `精度: ${timestampPrecisionLabels[time.precision]}`,
    ...(time.interpretation === undefined
      ? []
      : [
          `${interpretationOriginLabel(time.interpretation)}: UTC${time.interpretation.offset}`,
        ]),
  ];
  const offsetUnknown =
    time.interpretation === undefined &&
    !offsetCarriesInstant(time.offsetState);
  return (
    <span className="whitespace-nowrap">
      <TimeValueLink timestamp={time} details={details}>
        {time.rawText === undefined ? (
          <MissingValue description="原文の時刻なし" plain />
        ) : (
          <PeriodMark timestamp={time}>
            <RawText text={time.rawText} />
          </PeriodMark>
        )}
      </TimeValueLink>
      {offsetUnknown ? (
        <strong className="ml-1">
          <Hint text={offsetUnknownDescription}>{offsetUnknownLabel}</Hint>
        </strong>
      ) : null}
    </span>
  );
}

const noFileNames: ReadonlyMap<string, string> = new Map();

/**
 * 行のアドレスを範囲に持つ端末の名前を出す。端末の範囲を持たないアドレスと、範囲の端末の
 * ノードを応答が含まないアドレスは、その理由を出す。
 */
function TerminalCell({
  summary,
  names,
}: {
  summary: NodeSummary;
  names: ReadonlyMap<string, string>;
}) {
  if (summary.terminal === undefined) {
    return (
      <MissingValue
        description={
          summary.node.keyForm === "terminal_id_address"
            ? "範囲の端末のノードなし"
            : "端末の範囲なし"
        }
      />
    );
  }
  return (
    <RawText
      text={names.get(summary.terminal.id) ?? terminalLabel(summary.terminal)}
    />
  );
}

/**
 * 時刻の範囲に入らないレコードの件数を、時系列と同じくタイムゾーン未定と時刻なしに分けた
 * 「名前: 値」の組にする。どちらも 0 件の組は描かない。
 */
function OutsideRangeCounts({ summary }: { summary: NodeSummary }) {
  const count = (value: number) => (value > 0 ? formatCount(value) : undefined);
  return (
    <KeyValueList
      className="note"
      pairs={[
        {
          name: "範囲外・タイムゾーン未定",
          value: count(summary.localTimeRecordCount),
        },
        {
          name: "範囲外・時刻なし",
          value: count(summary.undatedRecordCount),
        },
      ]}
    />
  );
}

/**
 * 1 つの種類のノードを、接するレコードの件数の多い順に並べる。ノードの値を押すと、Node Detail に
 * 表示してグラフで選ぶ。フィルタの条件はグラフと時系列と同じものを上位の画面から受け取る。
 *
 * **memo にし、要求が変わらない限り取り直さない。** 上位の画面は時系列に関係しない状態も持つ。
 */
export const NodeSummaryList = memo(function NodeSummaryList({
  request,
  version,
  selectedNodeId,
  fileNamesByContent = noFileNames,
}: {
  /** 要求の組。上位の画面が useMemo で保つ。 */
  request: NodeSummariesRequest;
  /** 端末の割り当てとタイムゾーンを記録した回数。変わるたびに取り直す。 */
  version: number;
  selectedNodeId: string | undefined;
  /** 収集元の内容の識別から表示名を探す表。同じ表示名の端末を見分けるのに使う。 */
  fileNamesByContent?: ReadonlyMap<string, string>;
}) {
  const [state, setState] = useState<FetchState<NodeSummariesResponse>>({
    status: "loading",
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じ条件で取り直す。
  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchNodeSummaries(request, { signal: controller.signal }).then(
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
          failure: buildFetchFailure("unexpected", "ノードの一覧の取得"),
        });
      },
    );
    return () => controller.abort();
  }, [request, version]);

  const kind = kindLabels[request.nodeKind];
  // 端末の範囲で識別するアドレスは、端末ごとに別の行になる。行の端末を列に出して見分ける。
  const showsTerminal = request.nodeKind === "ip";
  const terminalNames = useMemo(
    () =>
      distinctTerminalNames(
        state.status === "loaded"
          ? state.value.nodes.flatMap((summary) =>
              summary.terminal === undefined ? [] : [summary.terminal],
            )
          : [],
        fileNamesByContent,
      ),
    [state, fileNamesByContent],
  );
  const columns: DataTableColumn<NodeSummary>[] = [
    {
      key: "node",
      header: kind,
      rowHeader: true,
      sortValue: (summary) =>
        request.nodeKind === "ip"
          ? ipSortValue(labelOf(summary))
          : labelOf(summary),
      cell: (summary) => (
        <ValueLink
          target={{
            kind: "node",
            id: summary.node.id,
            label: labelOf(summary),
          }}
          hover={
            <KeyValueList
              stacked
              pairs={[
                { name: "種類", value: kind },
                { name: "識別", value: summary.node.id },
              ]}
            />
          }
        >
          <Highlighted text={labelOf(summary)} />
        </ValueLink>
      ),
    },
    ...(showsTerminal
      ? [
          {
            key: "terminal",
            header: "端末",
            sortValue: (summary: NodeSummary) =>
              summary.terminal === undefined
                ? undefined
                : terminalLabel(summary.terminal),
            cell: (summary: NodeSummary) => (
              <TerminalCell summary={summary} names={terminalNames} />
            ),
          },
        ]
      : []),
    {
      key: "records",
      header: "レコード",
      numeric: true,
      sortValue: (summary) => summary.recordCount,
      cell: (summary) => (
        <>
          {formatCount(summary.recordCount)}
          <OutsideRangeCounts summary={summary} />
        </>
      ),
    },
    {
      key: "first",
      header: "最初の時刻",
      mono: true,
      sortValue: (summary) => timestampSortValue(summary.firstTime),
      cell: (summary) => <TimeCell time={summary.firstTime} />,
    },
    {
      key: "last",
      header: "最後の時刻",
      mono: true,
      sortValue: (summary) => timestampSortValue(summary.lastTime),
      cell: (summary) => <TimeCell time={summary.lastTime} />,
    },
  ];
  return (
    <FetchStateView
      state={state}
      loadingDescription={`${kind}の一覧の読み込み中`}
    >
      {(response) =>
        response.nodeCount === 0 ? (
          <KeyValueList pairs={[{ name: kind, value: "0" }]} />
        ) : (
          <>
            <KeyValueList
              pairs={[
                { name: kind, value: formatCount(response.nodeCount) },
                { name: "並び", value: "レコードの多い順" },
              ]}
            />
            <DataTable
              label={kind}
              className="node-summaries"
              columns={columns}
              rows={response.nodes}
              rowKey={(summary) => summary.node.id}
              rowProps={(summary) => ({
                "aria-current":
                  summary.node.id === selectedNodeId ? "true" : undefined,
              })}
            />
          </>
        )
      }
    </FetchStateView>
  );
});

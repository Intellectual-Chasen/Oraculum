import { memo, useEffect, useMemo, useState } from "react";
import type { GraphTimeFilter, NodeRef, SourceRef } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import type { RecordLocator, RequestedTime } from "@/shared/contracts/common";
import type {
  TimelineEntry,
  TimelineResponse,
} from "@/shared/contracts/timeline";
import { formatCount } from "@/shared/lib/format";
import { timestampPrecisionLabels } from "@/shared/lib/recordLabels";
import { periodUnjudgedParts, utcTextOf } from "@/shared/lib/timestampInstant";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TimestampText } from "@/shared/ui/TimestampText";
import { SourceCoverageList } from "./SourceCoverageList";
import { type FindQuery, TimelineFind } from "./TimelineFind";
import { TimelineList } from "./TimelineList";
import type { TimelineScaleKey } from "./timelineGrouping";
import type {
  TimelineRowActions,
  TimelineRowBookmark,
} from "./timelineRowMenu";
import { useTimeline } from "./useTimeline";

const noEntries: readonly TimelineEntry[] = [];

/** 期間の端の文字列。精度は tooltip に入れる。 */
function RequestedBound({ value }: { value: RequestedTime | undefined }) {
  if (value === undefined) return null;
  return (
    <Hint text={`精度: ${timestampPrecisionLabels[value.precision]}`}>
      <RawText text={value.requestText} />
    </Hint>
  );
}

/**
 * 応答に用いられた期間・案件・検索式と、表の件数を「名前: 値」の組で出す。
 *
 * **入力欄をここに置かない。** 期間の入力欄は Search に 1 つだけ置き、本節はその値を読むだけに
 * する。同じ条件を 2 か所から変えられる形にすると、どちらの値が応答に作用したかを分析者が読めない。
 *
 * **時刻を読み取れなかったレコードを表に混ぜず、件数で示す。** 既定の日付を与えると、分析者は
 * その行の位置を原文の事実として読む。表の外の件数には、期間の判定から外れたレコード
 * (`periodUnjudged`) を合わせ、その内訳を続けて出す。
 */
function TimelineSummary({ response }: { response: TimelineResponse }) {
  const unjudged = response.periodUnjudged;
  const outside =
    response.undatedRecordCount +
    (unjudged?.localRecordCount ?? 0) +
    (unjudged?.undatedRecordCount ?? 0);
  const period =
    response.timeFrom === undefined && response.timeTo === undefined ? (
      "指定なし"
    ) : (
      <span className="font-mono">
        <RequestedBound value={response.timeFrom} /> –{" "}
        <RequestedBound value={response.timeTo} />
      </span>
    );
  const pairs: KeyValuePair[] = [
    { name: "期間", value: period },
    { name: "案件", value: response.case },
    {
      name: "検索式",
      value:
        response.searchExpression === undefined ? undefined : (
          <RawText text={response.searchExpression} />
        ),
    },
    { name: "レコード", value: formatCount(response.entryCount) },
    ...(outside === 0
      ? []
      : [
          { name: "表の外", value: formatCount(outside) },
          ...periodUnjudgedParts(unjudged),
        ]),
  ];
  return <KeyValueList pairs={pairs} />;
}

type TimelineProps = {
  /** 候補を絞るのに用いる条件の選択。上位の画面が持つ。 */
  matchConditions: MatchConditionSelection;
  /** 根拠のレコードを絞る期間。上位の画面が持つ。 */
  timeFilter: GraphTimeFilter | undefined;
  /** 事象の分類の文字列。上位の画面が持つ。文字列の一致を完全一致で調べる。 */
  eventCategory: string | undefined;
  eventAction: string | undefined;
  /** 事象の動作を 10 進の数として比べる範囲の両端。上位の画面が持つ。 */
  eventActionFrom?: number | undefined;
  eventActionTo?: number | undefined;
  /** 根拠のレコードを絞る案件。上位の画面が持つ。出ない場合はすべての案件を読む。 */
  caseId: CaseId | undefined;
  /** 根拠のレコードを絞る端末のノードの識別子。上位の画面が持つ。 */
  terminal: string | undefined;
  /** 根拠のレコードを絞る収集元の sourceId。上位の画面が持つ。 */
  sources?: readonly string[] | undefined;
  /**
   * 根拠のレコードを絞る検索式。上位の画面が持ち、グラフの探索と同じ文字列を渡す。出ない場合は
   * 式で絞らない。
   */
  searchExpression?: string | undefined;
  /**
   * 起点のノードの識別子と、起点から辿る段数。上位の画面が持つ。起点が出ない場合は
   * ノードで絞らない。
   */
  nearNodeId?: string | undefined;
  accountNodeId?: string | undefined;
  onBefore?: (entry: TimelineEntry) => void;
  nearDepth?: number;
  /** 行を選んだときに、元レコードの表示へ渡す。 */
  onSelectRecord: (recordRef: RecordLocator) => void;
  /**
   * 探した文字列の一致へ移ったときに、前面のビューを変えずに元レコードの表示へ渡す。
   * 出ない場合は onSelectRecord を使う。
   */
  onPreviewRecord?: (recordRef: RecordLocator) => void;
  /**
   * 行のコンテキストメニューから、行の端末で根拠のレコードを絞る。絞る条件は上位の画面が持つ。
   * 出ない場合は項目を出さない。
   */
  onNarrowToTerminal?: (terminal: NodeRef) => void;
  /** 行のコンテキストメニューから、行の収集元だけに根拠のレコードを絞る。出ない場合は項目を出さない。 */
  onNarrowToSource?: (source: SourceRef) => void;
  /**
   * 行のコンテキストメニューから、行のレコードのブックマークを付け外しする。一覧は上位の画面が持つ。
   * 描画をまたいで同じ組を渡す。出ない場合は項目を出さない。
   */
  rowBookmark?: TimelineRowBookmark;
  /** 端末の割当を記録した回数。上位の画面が持ち、変わるたびに時系列を取り直す。 */
  dataVersion: number;
  /**
   * 移動先の時刻。UTC の RFC 3339 の文字列。一覧を読めたら、その時刻以後の最初の行へ移り、
   * onSeekDone で知らせる。状態の所有者は上位の画面である。
   */
  seek?: { utc: string } | undefined;
  onSeekDone?: () => void;
};

/**
 * 移動先の時刻 `utc` 以後の最初の行の位置。行は時刻の順に並ぶ。以後の行が無ければ、時刻を
 * 持つ最後の行の位置を返し、時刻を持つ行が無ければ undefined を返す。
 */
export function seekIndexOf(
  entries: readonly TimelineEntry[],
  utc: string,
): number | undefined {
  const target = Date.parse(utc);
  let last: number | undefined;
  for (const [index, entry] of entries.entries()) {
    const text =
      entry.eventTime === undefined ? undefined : utcTextOf(entry.eventTime);
    if (text === undefined) continue;
    if (Date.parse(text) >= target) return index;
    last = index;
  }
  return last;
}

/**
 * 収集元をまたいで根拠を時刻順に読み、収集元ごとの収録範囲を並べる。
 *
 * **絞り込みの条件は上位の画面が持つ。** 同じ期間でグラフと時系列の両方が動くため、
 * 条件を本機能に閉じると 2 つの画面が別の期間を出す。
 *
 * **memo にし、props が変わらない限り描き直さない。** 上位の画面は、開いたレコードや
 * 選んだ関係など、時系列に関係しない状態を持つ。その状態が変わるたびに時系列の全行を
 * 描き直すと、画面が操作に応じない時間が伸びる。
 */
export const Timeline = memo(function Timeline({
  matchConditions,
  timeFilter,
  eventCategory,
  eventAction,
  eventActionFrom,
  eventActionTo,
  caseId,
  terminal,
  sources,
  searchExpression,
  nearNodeId,
  accountNodeId,
  onBefore,
  nearDepth = 1,
  onSelectRecord,
  onPreviewRecord = onSelectRecord,
  onNarrowToTerminal,
  onNarrowToSource,
  rowBookmark,
  dataVersion,
  seek,
  onSeekDone,
}: TimelineProps) {
  // 行は memo であり、描画をまたいで同じ組を渡す。
  const rowActions = useMemo<TimelineRowActions>(
    () => ({
      onSelectRecord,
      onPreviewRecord,
      onNarrowToTerminal,
      onNarrowToSource,
      terminal,
      sources,
      bookmark: rowBookmark,
    }),
    [
      onSelectRecord,
      onPreviewRecord,
      onNarrowToTerminal,
      onNarrowToSource,
      terminal,
      sources,
      rowBookmark,
    ],
  );
  // 原文から探す文字列。送った文字列ごとに取り直し、応答が一致した行の位置を返す。
  const [findQuery, setFindQuery] = useState<FindQuery | undefined>(undefined);
  // **要求の組を useMemo で保つ。** useTimeline の effect の依存は組そのものである。
  // レンダーごとに新しい組を作ると、取得 → 状態の更新 → 再レンダー → 新しい組 →
  // 取得のやり直し、という繰り返しになる。
  const request = useMemo(
    () => ({
      matchConditions,
      timeFilter,
      eventCategory,
      eventAction,
      eventActionFrom,
      eventActionTo,
      caseId,
      terminal,
      sources,
      searchExpression,
      near:
        nearNodeId === undefined
          ? undefined
          : { nodeId: nearNodeId, depth: nearDepth },
      accountNodeId,
      find: findQuery,
    }),
    [
      matchConditions,
      timeFilter,
      eventCategory,
      eventAction,
      eventActionFrom,
      eventActionTo,
      caseId,
      terminal,
      sources,
      searchExpression,
      nearNodeId,
      accountNodeId,
      nearDepth,
      findQuery,
    ],
  );
  const state = useTimeline(request, dataVersion);
  // **分析者が選んだ段階を、取り直しの間も保つ。** 読み込み中は一覧を描かないため、段階を
  // 一覧の中に持つと、条件を変えるたびに既定の段階へ戻る。
  const [chosenScale, setChosenScale] = useState<TimelineScaleKey | undefined>(
    accountNodeId === undefined ? undefined : "record",
  );
  const loaded = state.status === "loaded" ? state.value : undefined;
  // **一致の位置は応答ごとに持つ。** 条件を変えて取り直すと、同じ位置が別の行を指す。
  const [focus, setFocus] = useState<
    { response: TimelineResponse; index: number } | undefined
  >(undefined);
  const focusedEntry =
    focus !== undefined && focus.response === loaded ? focus.index : undefined;
  // 移動先の時刻と時刻が一致しない行へ移ったときの、移った先の行。行が無ければ entry を持たない。
  const [seekMiss, setSeekMiss] = useState<
    { response: TimelineResponse; entry: TimelineEntry | undefined } | undefined
  >(undefined);
  // 移動先の時刻は、一覧を読めた後に 1 回だけ使う。前面のビューと開いたレコードは変えない。
  useEffect(() => {
    if (seek === undefined || loaded === undefined) return;
    const index = seekIndexOf(loaded.entries, seek.utc);
    const entry = index === undefined ? undefined : loaded.entries[index];
    if (index !== undefined) setFocus({ response: loaded, index });
    const entryUtc =
      entry?.eventTime === undefined ? undefined : utcTextOf(entry.eventTime);
    const exact =
      entryUtc !== undefined && Date.parse(entryUtc) === Date.parse(seek.utc);
    setSeekMiss(exact ? undefined : { response: loaded, entry });
    onSeekDone?.();
  }, [seek, loaded, onSeekDone]);
  const seekMissEntry =
    seekMiss !== undefined && seekMiss.response === loaded
      ? seekMiss
      : undefined;
  const focusEntry = (index: number) => {
    const entry = loaded?.entries[index];
    if (loaded === undefined || entry === undefined) return;
    setSeekMiss(undefined);
    setFocus({ response: loaded, index });
    onPreviewRecord(entry.recordRef);
  };
  return (
    <section aria-labelledby="timeline-heading">
      <h2 id="timeline-heading">時系列</h2>
      <TimelineFind
        entries={loaded?.entries ?? noEntries}
        matches={loaded?.findMatches}
        focused={focusedEntry}
        onSubmit={setFindQuery}
        onFocus={focusEntry}
      />
      {seekMissEntry === undefined ? null : (
        <div role="status" className="flex flex-wrap items-center gap-2">
          <StatusLabel status="idle" label="一致なし" />
          <KeyValueList
            pairs={[
              {
                name: "移動先",
                value:
                  seekMissEntry.entry?.eventTime === undefined ? undefined : (
                    <TimestampText timestamp={seekMissEntry.entry.eventTime} />
                  ),
              },
            ]}
          />
        </div>
      )}
      <FetchStateView state={state} loadingDescription="読み込み中">
        {(response) => (
          <>
            <TimelineSummary response={response} />
            <SourceCoverageList coverages={response.sourceCoverages} />
            <TimelineList
              entries={response.entries}
              sourceCoverages={response.sourceCoverages}
              chosenScale={chosenScale}
              onChooseScale={setChosenScale}
              onSelectRecord={onSelectRecord}
              rowActions={rowActions}
              focusedEntry={focusedEntry}
              onBefore={onBefore}
            />
          </>
        )}
      </FetchStateView>
    </section>
  );
});

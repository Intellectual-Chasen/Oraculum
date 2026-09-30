import { Link2, PanelRightOpen } from "lucide-react";
import { type CSSProperties, useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  edgeUrlFragmentsFailureSummary,
  fetchEdgeUrlFragments,
} from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import type { RecordLocator } from "@/shared/contracts/common";
import type {
  UrlFragment,
  UrlFragmentDecodeFailure,
  UrlFragmentEncoding,
  UrlFragmentJoin,
  UrlFragmentSegment,
  ZipIntegrity,
} from "@/shared/contracts/urlFragments";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import {
  describeRecordPosition,
  recordRefKey,
} from "@/shared/lib/recordPosition";
import { DataTable } from "@/shared/ui/DataTable";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Fold } from "@/shared/ui/Fold";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import { useVirtualRows } from "@/shared/ui/useVirtualRows";
import {
  SpacerBody,
  stickyHeaderCellStyle,
  virtualCellStyle,
  virtualTableStyle,
} from "@/shared/ui/virtualTable";

const urlFragmentEncodingLabels: Record<UrlFragmentEncoding, string> = {
  base64url: "Base64 URL",
  base64: "Base64",
};

/** 復号しなかった理由。 */
const urlFragmentDecodeFailureLabels: Record<UrlFragmentDecodeFailure, string> =
  {
    missing_numbers: "欠けた番号",
    conflicting_duplicates: "同じ番号の異なる行",
    alphabet_undetermined: "Base64 の変種が不明",
    invalid_encoding: "Base64 として復号できない",
  };

const zipIntegrityLabels: Record<ZipIntegrity, string> = {
  passed: "全 file の CRC が一致",
  failed: "展開できないか CRC が不一致の file あり",
  not_checked: "展開の上限で CRC が未確認",
  unsupported: "暗号化か未対応の圧縮方式で CRC が未確認",
};

/** 断片の連結の規則。見出しの説明に出す。 */
const joinRules = [
  { name: "対象", value: "根拠のレコードの URL の path の /番号/文字列" },
  { name: "連結", value: "番号の順に連結して Base64 として復号" },
  { name: "送信のまとまりの区切り", value: "番号の減少と収集元の変化" },
];

const estimatedFragmentRowHeight = 40;
const overscanPx = 400;
const scrollerStyle: CSSProperties = {
  maxHeight: "50vh",
  overflow: "auto",
  overflowAnchor: "none",
};
const fragmentTableStyle: CSSProperties = {
  ...virtualTableStyle,
  display: "table",
  minWidth: "32rem",
};
/** 断片の表の列。連結の結果の文字列が折り返さない幅を与える。 */
const fragmentColumns = [
  { label: "番号", width: "5rem" },
  { label: "連結", width: "12rem" },
  { label: "レコード", width: "auto" },
];
/** 詳細の列は狭い。まとまりの表の結果の文字列を 1 字ずつ折り返さない。 */
const noWrapStyle: CSSProperties = { whiteSpace: "nowrap" };

/** 断片のレコード 1 件。収集元と位置を Record に表示する値にする。 */
function FragmentRecordButton({
  recordRef,
  onSelectRecord,
}: {
  recordRef: RecordLocator;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  return (
    <button
      type="button"
      className="value-link"
      onClick={() => onSelectRecord(recordRef)}
    >
      <RawText text={recordRef.sourceFileName} />{" "}
      {describeRecordPosition(recordRef)}
    </button>
  );
}

/**
 * エッジの根拠のレコードが URL の path に `/<番号>/<文字列>` の形で書いた断片を、送信の
 * まとまりごとに番号の順に連結して復号した結果を出す。ボタンを押したときだけ取得する。
 * エッジを替えたら、呼び出し側が key を替えて結果を捨てる。
 */
export function UrlFragmentJoinView({
  edgeId,
  caseId,
  matchConditions,
  onSelectRecord,
}: {
  edgeId: string;
  caseId?: CaseId;
  matchConditions: MatchConditionSelection;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const [requested, setRequested] = useState(false);
  const state = useUrlFragments(
    requested ? edgeId : undefined,
    caseId,
    matchConditions,
  );
  return (
    <section aria-label="URL の断片の連結">
      <div className="flex items-center gap-1">
        <h3>URL の断片の連結</h3>
        <HelpPopover label="URL の断片の連結">
          <KeyValueList stacked pairs={joinRules} />
        </HelpPopover>
        <IconButton label="URL の断片を連結" onPress={() => setRequested(true)}>
          <Link2 size={14} aria-hidden="true" />
        </IconButton>
      </div>
      {state === undefined ? null : (
        <FetchStateView state={state} loadingDescription="URL の断片の連結中">
          {(join) => <JoinResult join={join} onSelectRecord={onSelectRecord} />}
        </FetchStateView>
      )}
    </section>
  );
}

function JoinResult({
  join,
  onSelectRecord,
}: {
  join: UrlFragmentJoin;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const [selected, setSelected] = useState(join.segments.length - 1);
  const segment = join.segments[selected];
  return (
    <>
      {join.unnumberedRecordCount > 0 ? (
        <KeyValueList
          pairs={[
            {
              name: "番号の無い要求",
              value: (
                <Hint text="連結なし: 順序が不明">
                  {formatCount(join.unnumberedRecordCount)}
                </Hint>
              ),
            },
          ]}
        />
      ) : null}
      {join.segments.length === 0 ? (
        <p role="status">番号付きの断片なし</p>
      ) : (
        <table>
          <caption>送信のまとまり</caption>
          <thead>
            <tr>
              <th scope="col">番号</th>
              <th scope="col">行数</th>
              <th scope="col">最後の番号</th>
              <th scope="col">再送した行</th>
              <th scope="col">異なる文字列の再送</th>
              <th scope="col">欠けた番号</th>
              <th scope="col">結果</th>
              <th scope="col">操作</th>
            </tr>
          </thead>
          <tbody>
            {join.segments.map((item, index) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: まとまりの並びは応答が決め、同じ応答の中で変わらない。
              <tr key={index} aria-selected={index === selected}>
                <td>{index + 1}</td>
                <td>{formatCount(item.fragmentCount)}</td>
                <td>{formatCount(item.lastNumber)}</td>
                <td>{formatCount(item.duplicateCount)}</td>
                <td>{formatCount(item.conflictingDuplicateCount)}</td>
                <td>{formatCount(item.missingNumberCount)}</td>
                <td style={noWrapStyle}>{segmentOutcome(item)}</td>
                <td style={noWrapStyle}>
                  <IconButton
                    label={`送信のまとまり ${index + 1} を表示`}
                    isDisabled={index === selected}
                    disabledReason={{
                      title: "表示中",
                      text: "別のまとまりの選択",
                    }}
                    onPress={() => setSelected(index)}
                  >
                    <PanelRightOpen size={14} aria-hidden="true" />
                  </IconButton>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {segment === undefined ? null : (
        <SegmentDetail
          key={selected}
          index={selected}
          segment={segment}
          onSelectRecord={onSelectRecord}
        />
      )}
    </>
  );
}

/**
 * 断片 1 行を連結したかと、連結しなかった理由。状態が成功でない行と途中で切れた行は、
 * 断片が届いた根拠にしない。
 */
function fragmentOutcome(fragment: UrlFragment): string {
  if (fragment.adopted) {
    return "済み";
  }
  if (fragment.truncated) {
    return "未: 途中で切れた行";
  }
  const status = fragment.httpStatusCode;
  if (status !== undefined && !status.startsWith("2")) {
    return `未: 状態 ${status}`;
  }
  return "未: 再送";
}

/** まとまりの表に出す短い結果。理由はまとまりの結果のセクションが出す。 */
function segmentOutcome(segment: UrlFragmentSegment): string {
  if (segment.decoded === undefined) {
    return "未復号";
  }
  const zip = segment.decoded.zip;
  if (zip === undefined) {
    return segment.decoded.contentType;
  }
  return zip.readable
    ? `ZIP · file ${formatCount(zip.entryCount)}`
    : "ZIP として解析できない";
}

function SegmentDetail({
  index,
  segment,
  onSelectRecord,
}: {
  index: number;
  segment: UrlFragmentSegment;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const decoded = segment.decoded;
  const zip = decoded?.zip;
  return (
    <section aria-label={`送信のまとまり ${index + 1}`}>
      <h4>{`送信のまとまり ${index + 1}`}</h4>
      <KeyValueList
        stacked
        pairs={[
          {
            name: "未復号の理由",
            value:
              decoded === undefined && segment.decodeFailure !== undefined
                ? urlFragmentDecodeFailureLabels[segment.decodeFailure]
                : undefined,
          },
          {
            name: "Base64 の変種",
            value:
              segment.encoding === undefined
                ? undefined
                : urlFragmentEncodingLabels[segment.encoding],
          },
          {
            name: "先頭の byte の形式",
            value: decoded?.contentType,
          },
          {
            name: "先頭の byte",
            value:
              decoded === undefined ? undefined : (
                <Hint text="16 進">
                  <code>{decoded.leadingBytesHex}</code>
                </Hint>
              ),
          },
          {
            name: "byte 数",
            value:
              decoded === undefined
                ? undefined
                : formatCount(decoded.byteCount),
          },
          {
            name: "SHA-256",
            value:
              decoded === undefined ? undefined : <code>{decoded.sha256}</code>,
          },
          {
            name: "ZIP",
            value:
              zip === undefined
                ? undefined
                : zip.integrity === undefined
                  ? "中央ディレクトリの読み込みに失敗"
                  : zipIntegrityLabels[zip.integrity],
          },
        ]}
      />
      {zip === undefined || !zip.readable ? null : (
        <>
          <ul aria-label="ZIP の中の file">
            {zip.entries.map((entry, entryIndex) => (
              // biome-ignore lint/suspicious/noArrayIndexKey: 名前は重複しうる。並びは応答の中で変わらない。
              <li key={entryIndex}>
                <RawText text={entry.name} />
                {entry.nameHex === undefined ? null : (
                  <Hint
                    text="UTF-8 として解釈できない名前"
                    className="ml-1.5 text-xs text-muted"
                  >
                    元の byte 列: <code>{entry.nameHex}</code>
                  </Hint>
                )}
              </li>
            ))}
          </ul>
          <KeyValueList
            pairs={[
              {
                name: "表示していない file",
                value:
                  zip.entryCount > zip.entries.length
                    ? formatCount(zip.entryCount - zip.entries.length)
                    : undefined,
              },
            ]}
          />
        </>
      )}
      <TruncatedFragments segment={segment} onSelectRecord={onSelectRecord} />
      <Fold
        summary="断片のレコード"
        summaryNote={`件数: ${formatCount(segment.fragmentCount)}`}
        rowCount={segment.fragments.length}
      >
        <FragmentTable segment={segment} onSelectRecord={onSelectRecord} />
      </Fold>
    </section>
  );
}

/** 回の中の途中で切れた行を、欠けた番号として位置と一緒に出す。無いときは何も出さない。 */
function TruncatedFragments({
  segment,
  onSelectRecord,
}: {
  segment: UrlFragmentSegment;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const truncated = segment.fragments.filter((fragment) => fragment.truncated);
  if (truncated.length === 0) {
    return null;
  }
  const adoptedNumbers = new Set(
    segment.fragments
      .filter((fragment) => fragment.adopted)
      .map((fragment) => fragment.number),
  );
  return (
    <DataTable
      label="途中で切れた行"
      showCaption
      rows={truncated}
      rowKey={(fragment) => recordRefKey(fragment.recordRef)}
      columns={[
        {
          key: "number",
          header: "番号",
          numeric: true,
          cell: (fragment) => fragment.number,
        },
        {
          key: "state",
          header: "番号の状態",
          cell: (fragment) =>
            adoptedNumbers.has(fragment.number)
              ? "同じ番号の別の行を連結"
              : "欠けた番号",
        },
        {
          key: "record",
          header: "レコード",
          cell: (fragment) => (
            <FragmentRecordButton
              recordRef={fragment.recordRef}
              onSelectRecord={onSelectRecord}
            />
          ),
        },
      ]}
    />
  );
}

function FragmentTable({
  segment,
  onSelectRecord,
}: {
  segment: UrlFragmentSegment;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const fragments = segment.fragments;
  const virtual = useVirtualRows(
    fragments.length,
    fragments,
    estimatedFragmentRowHeight,
    overscanPx,
  );
  return (
    <section
      ref={virtual.scrollerRef}
      aria-label="断片のレコードの表"
      // biome-ignore lint/a11y/noNoninteractiveTabindex: スクロールする領域をキーボードで動かせるようにする。
      tabIndex={0}
      onScroll={virtual.onScroll}
      style={scrollerStyle}
    >
      <table style={fragmentTableStyle} aria-rowcount={fragments.length + 1}>
        <colgroup>
          {fragmentColumns.map((column) => (
            <col key={column.label} style={{ width: column.width }} />
          ))}
        </colgroup>
        <thead>
          <tr aria-rowindex={1}>
            {fragmentColumns.map((column) => (
              <th key={column.label} scope="col" style={stickyHeaderCellStyle}>
                {column.label}
              </th>
            ))}
          </tr>
        </thead>
        <SpacerBody
          height={virtual.spaceBefore}
          columnCount={fragmentColumns.length}
        />
        <tbody ref={virtual.bodyRef}>
          {fragments
            .slice(virtual.start, virtual.end)
            .map((fragment, offset) => {
              const rowIndex = virtual.start + offset;
              return (
                // 断片の並びは応答が決め、同じ応答の中で変わらない。
                <tr
                  key={rowIndex}
                  data-row-index={rowIndex}
                  aria-rowindex={rowIndex + 2}
                >
                  <td style={virtualCellStyle}>{fragment.number}</td>
                  <td style={virtualCellStyle}>{fragmentOutcome(fragment)}</td>
                  <td style={virtualCellStyle}>
                    <FragmentRecordButton
                      recordRef={fragment.recordRef}
                      onSelectRecord={onSelectRecord}
                    />
                  </td>
                </tr>
              );
            })}
        </tbody>
        <SpacerBody
          height={virtual.spaceAfter}
          columnCount={fragmentColumns.length}
        />
      </table>
    </section>
  );
}

/** 関係の識別子を受け取ったときに取得する。古い要求は打ち切る。 */
function useUrlFragments(
  edgeId: string | undefined,
  caseId: CaseId | undefined,
  matchConditions: MatchConditionSelection,
): FetchState<UrlFragmentJoin> | undefined {
  const [state, setState] = useState<FetchState<UrlFragmentJoin> | undefined>(
    undefined,
  );
  useEffect(() => {
    if (edgeId === undefined) {
      setState(undefined);
      return;
    }
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchEdgeUrlFragments(
      { id: edgeId, caseId, matchConditions },
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        );
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure(
            "unexpected",
            edgeUrlFragmentsFailureSummary,
          ),
        });
      });
    return () => controller.abort();
  }, [edgeId, caseId, matchConditions]);
  return state;
}

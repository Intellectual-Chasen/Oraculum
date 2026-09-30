import { FileText, SquareArrowOutUpRight } from "lucide-react";
import { type CSSProperties, memo, useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchRawText } from "@/shared/api/sources";
import type { RecordLocator } from "@/shared/contracts/common";
import type { ImportFailure } from "@/shared/contracts/sources";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { diagnosisClassLabels } from "@/shared/lib/sourceLabels";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Fold } from "@/shared/ui/Fold";
import { HelpPopover } from "@/shared/ui/HelpPopover";
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
import { describeFailedByte, describeFailureRecord } from "./inventory";
import { failureStageLabels } from "./labels";

/** 描く前に見積もる行の高さ (px)。描いた行は測った高さへ直す。 */
const estimatedRowHeight = 72;
/** 見えている範囲の前後に、先に描いておく高さ (px)。 */
const overscanPx = 400;

const scrollerStyle: CSSProperties = {
  maxHeight: "50vh",
  overflow: "auto",
  overflowAnchor: "none",
  scrollbarGutter: "stable",
};

/** 開いた原文の枠。長い行を折り返し、枠の中でスクロールする。 */
const rawTextStyle: CSSProperties = {
  whiteSpace: "pre-wrap",
  overflowWrap: "anywhere",
  maxHeight: "40vh",
  overflow: "auto",
};

const columns = [
  { label: "レコードの範囲", width: "16%" },
  { label: "失敗した位置", width: "9%" },
  { label: "失敗した段階", width: "9%" },
  { label: "期待した内容", width: "17%" },
  { label: "読み取った結果", width: "20%" },
  { label: "原因の分類", width: "17%" },
  { label: "操作", width: "12%" },
];

const truncatedLabel = "途中で切れた行";

const truncatedNote = (
  <KeyValueList
    stacked
    pairs={[
      { name: "行の終わり", value: "書き出した側の長さの上限" },
      {
        name: "切れる前に読み取れたフィールド",
        value: "レコードとして取り込み済み",
      },
    ]}
  />
);

/**
 * 途中で切れた行の位置を、読めた欄のレコードを探す位置にする。
 *
 * **行番号で指す位置から byte 範囲を外す。** 取り込めなかった行の位置は失敗の表に出す
 * byte 範囲を足して返すが、取り込んだレコードは行番号だけで探せる。byte 位置を添えた
 * 要求は、読めた欄のレコードに一致しない。
 */
function readableRecordRef(recordRef: RecordLocator): RecordLocator {
  if (recordRef.positionKind !== "line_number") {
    return recordRef;
  }
  const { byteOffset: _offset, byteLength: _length, ...readable } = recordRef;
  return readable;
}

/** 取り込めなかったレコード 1 件の行。スクロールで行を入れ替えるたびに、残る行を描き直さない。 */
const FailureRow = memo(function FailureRow({
  failure,
  rowIndex,
  onOpenRawText,
  onSelectRecord,
}: {
  failure: ImportFailure;
  rowIndex: number;
  onOpenRawText: (rowIndex: number) => void;
  onSelectRecord?: (recordRef: RecordLocator) => void;
}) {
  const recordRef = failure.recordRef;
  return (
    <tr data-row-index={rowIndex} aria-rowindex={rowIndex + 2}>
      <td style={virtualCellStyle}>{describeFailureRecord(failure)}</td>
      <td style={virtualCellStyle}>{describeFailedByte(failure) ?? "―"}</td>
      <td style={virtualCellStyle}>{failureStageLabels[failure.stage]}</td>
      <td style={virtualCellStyle}>
        <RawText text={failure.expectedMeaning} />
      </td>
      <td style={virtualCellStyle}>
        {failure.recordTruncated ? (
          <span className="block">
            <strong>{truncatedLabel}</strong>
            <HelpPopover label={truncatedLabel}>{truncatedNote}</HelpPopover>
          </span>
        ) : null}
        <RawText text={failure.observedResult} />
      </td>
      <td style={virtualCellStyle}>
        {diagnosisClassLabels[failure.diagnosisClass]}
        {failure.unresolvedReason === undefined ? null : (
          <span className="block">
            <RawText text={failure.unresolvedReason} />
          </span>
        )}
      </td>
      <td style={virtualCellStyle}>
        {failure.rawTextRef === undefined ? null : (
          <IconButton
            label="原文を開く"
            onPress={() => onOpenRawText(rowIndex)}
          >
            <FileText size={14} aria-hidden="true" />
          </IconButton>
        )}
        {failure.recordTruncated &&
        recordRef !== undefined &&
        onSelectRecord !== undefined ? (
          <IconButton
            label="読み取れたフィールドを開く"
            onPress={() => onSelectRecord(readableRecordRef(recordRef))}
          >
            <SquareArrowOutUpRight size={14} aria-hidden="true" />
          </IconButton>
        ) : null}
      </td>
    </tr>
  );
});

function VirtualFailureTable({
  failures,
  onOpenRawText,
  onSelectRecord,
}: {
  failures: ImportFailure[];
  onOpenRawText: (rowIndex: number) => void;
  onSelectRecord?: (recordRef: RecordLocator) => void;
}) {
  const virtual = useVirtualRows(
    failures.length,
    failures,
    estimatedRowHeight,
    overscanPx,
  );
  return (
    <section
      ref={virtual.scrollerRef}
      aria-label="取り込めなかったレコードの表"
      // biome-ignore lint/a11y/noNoninteractiveTabindex: スクロールする領域をキーボードで動かせるようにする。
      tabIndex={0}
      onScroll={virtual.onScroll}
      style={scrollerStyle}
    >
      <table style={virtualTableStyle} aria-rowcount={failures.length + 1}>
        <caption className="sr-only">取り込めなかったレコード</caption>
        <colgroup>
          {columns.map((column) => (
            <col key={column.label} style={{ width: column.width }} />
          ))}
        </colgroup>
        <thead>
          <tr aria-rowindex={1}>
            {columns.map((column) => (
              <th key={column.label} scope="col" style={stickyHeaderCellStyle}>
                {column.label}
              </th>
            ))}
          </tr>
        </thead>
        <SpacerBody height={virtual.spaceBefore} columnCount={columns.length} />
        <tbody ref={virtual.bodyRef}>
          {failures.slice(virtual.start, virtual.end).map((failure, offset) => (
            <FailureRow
              // 失敗は同じ位置に複数ありうるため、応答の中の順番で指す。
              // biome-ignore lint/suspicious/noArrayIndexKey: 応答の並びは再取得まで変わらない。
              key={virtual.start + offset}
              failure={failure}
              rowIndex={virtual.start + offset}
              onOpenRawText={onOpenRawText}
              onSelectRecord={onSelectRecord}
            />
          ))}
        </tbody>
        <SpacerBody height={virtual.spaceAfter} columnCount={columns.length} />
      </table>
    </section>
  );
}

/** 原文への参照を受け取ったときに取得する。古い要求は打ち切る。 */
function useRawText(
  rawTextRef: string | undefined,
): FetchState<string> | undefined {
  const [state, setState] = useState<FetchState<string> | undefined>(undefined);
  useEffect(() => {
    if (rawTextRef === undefined) {
      setState(undefined);
      return;
    }
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchRawText(rawTextRef, { signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value.rawText }
            : { status: "failed", failure: result.failure },
        );
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure("unexpected", "原文の取得"),
        });
      });
    return () => controller.abort();
  }, [rawTextRef]);
  return state;
}

function OpenedRawText({ failure }: { failure: ImportFailure }) {
  const state = useRawText(failure.rawTextRef);
  return (
    <section aria-label="開いた原文">
      <h4>開いた原文: {describeFailureRecord(failure)}</h4>
      {state === undefined ? null : (
        <FetchStateView state={state} loadingDescription="原文の読み込み中">
          {(rawText) => (
            <pre style={rawTextStyle}>
              <RawText text={rawText} />
            </pre>
          )}
        </FetchStateView>
      )}
    </section>
  );
}

/**
 * 収集元 1 件の取り込めなかったレコードを、位置と理由の表で出す。0 件のときは何も出さない。
 * 件数の多い表は閉じて出し、開いたときも見えている行だけを描く。行から原文を開ける。
 */
export function ImportFailureTable({
  failures,
  onSelectRecord,
}: {
  failures: ImportFailure[];
  /** 途中で切れた行の読めた欄を開く。渡さないときは開く操作を出さない。 */
  onSelectRecord?: (recordRef: RecordLocator) => void;
}) {
  const [openedRow, setOpenedRow] = useState<number | undefined>(undefined);
  if (failures.length === 0) {
    return null;
  }
  const opened = openedRow === undefined ? undefined : failures[openedRow];
  return (
    <Fold
      summary={`取り込めなかったレコード: ${formatCount(failures.length)}`}
      rowCount={failures.length}
    >
      <VirtualFailureTable
        failures={failures}
        onOpenRawText={setOpenedRow}
        onSelectRecord={onSelectRecord}
      />
      {opened === undefined ? null : (
        <OpenedRawText key={openedRow} failure={opened} />
      )}
    </Fold>
  );
}

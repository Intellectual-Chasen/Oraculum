import { ArrowDownToLine, Play, Trash2 } from "lucide-react";
import {
  type CSSProperties,
  type Dispatch,
  type DragEventHandler,
  type FormEvent,
  memo,
  type ReactNode,
  type SetStateAction,
  useCallback,
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import { formatCount } from "@/shared/lib/format";
import { formatKeyLabel } from "@/shared/lib/sourceLabels";
import { Button } from "@/shared/ui/Button";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { RawText } from "@/shared/ui/RawText";
import { TextEditorPopover } from "@/shared/ui/TextEditorPopover";
import { useVirtualRows } from "@/shared/ui/useVirtualRows";
import {
  SpacerBody,
  stickyHeaderCellStyle,
  virtualTableStyle,
} from "@/shared/ui/virtualTable";
import {
  hasFormat,
  type LoadingFormField,
  type LoadingFormRow,
  requiresLogFormatSpec,
  withLogFormatSpec,
} from "./loadingRows";

type LoadingFormProps = {
  /** server が読める入力形式。入力形式の選択肢になる。 */
  formatKeys: string[];
  /** 読み込む収集元。状態の所有者は上位の画面である。読み込みの段階の失敗の後も入力を保つ。 */
  rows: LoadingFormRow[];
  onChangeRows: Dispatch<SetStateAction<LoadingFormRow[]>>;
  /** すべての収集元に付ける案件。空文字列は案件を付けないことを表す。 */
  caseId: string;
  onChangeCaseId: (caseId: string) => void;
  /** 読み込みの完了後に続けて処理を始めるか。 */
  thenProcess: boolean;
  onChangeThenProcess: (thenProcess: boolean) => void;
  /** 入力形式を選んだ収集元の読み込みを始める。 */
  onSubmit: (rows: LoadingFormRow[]) => void;
  /** 段階を始める要求か、段階の状態を取り直す要求の応答を待っているか。待つ間は送る操作を止める。 */
  isSubmitting: boolean;
  dragName?: string;
  onDragOver?: DragEventHandler<HTMLFormElement>;
  onDrop?: DragEventHandler<HTMLFormElement>;
  onUploadFiles?: (files: File[]) => void;
  uploadNotice?: ReactNode;
};

/** 詳しい指定の欄の名前と見出し。 */
const detailFields: { field: LoadingFormField; label: string }[] = [
  { field: "terminalId", label: "記録した端末の識別子" },
  { field: "terminalIp", label: "記録した端末の IP" },
];

/** 表の列の見出しと幅。 */
const columns: { label: string; width: string }[] = [
  { label: "収集元", width: "28%" },
  { label: "大きさ (byte)", width: "12%" },
  { label: "入力形式", width: "22%" },
  { label: "端末名", width: "16%" },
  { label: "詳しい指定", width: "14%" },
  { label: "外す", width: "8%" },
];

// 既知の制限: 選んだ収集元の表は、見えている行とその前後だけを描く。スクロールで描かなくなった
// 行は、入力欄の focus と「詳しい指定」の開閉を失い、ブラウザーのページ内検索に当たらない,
// 3000 件で測ると、全行を描く表は描画に 110 秒、1 文字の入力に 508 ms かかった,
// 1 行の高さが見積もりから大きく外れる表示を足したときに見直す
const estimatedRowHeight = 40;
const overscanPx = 400;

/** 表をスクロールする領域。表の外の操作 (開始の button) を画面の中に残す。 */
const scrollerStyle: CSSProperties = {
  maxHeight: "60vh",
  overflow: "auto",
  overflowAnchor: "none",
};

type RowProps = {
  row: LoadingFormRow;
  rowIndex: number;
  formatKeys: string[];
  onUpdate: (
    originPath: string,
    field: LoadingFormField,
    value: string,
  ) => void;
  onRemove: (originPath: string) => void;
  onUpdateSpec: (
    originPath: string,
    spec: string,
    applyToSquid: boolean,
  ) => void;
};

/**
 * 選んだ収集元 1 件の行。**行の値が変わった行だけを描き直す。** 1 行の入力で、ほかの行を
 * 描き直さない。
 */
const LoadingRow = memo(function LoadingRow({
  row,
  rowIndex,
  formatKeys,
  onUpdate,
  onRemove,
  onUpdateSpec,
}: RowProps) {
  const others = formatKeys.filter(
    (formatKey) => !row.formatCandidates.includes(formatKey),
  );
  return (
    <tr data-row-index={rowIndex} aria-rowindex={rowIndex + 2}>
      <th scope="row" data-label="収集元">
        <RawText text={row.originPath} />
      </th>
      <td className="text-right tabular-nums" data-label="大きさ (byte)">
        {row.sizeBytes === undefined ? null : formatCount(row.sizeBytes)}
      </td>
      <td data-label="入力形式">
        <select
          aria-label={`${row.originPath} の入力形式`}
          value={row.formatKey}
          onChange={(event) =>
            onUpdate(row.originPath, "formatKey", event.target.value)
          }
        >
          <option value="">未選択</option>
          {row.formatCandidates.length === 0 ? null : (
            <optgroup label="判定した候補">
              {row.formatCandidates.map((formatKey) => (
                <option key={formatKey} value={formatKey}>
                  {formatKeyLabel(formatKey)}
                </option>
              ))}
            </optgroup>
          )}
          <optgroup label="ほかの形式">
            {others.map((formatKey) => (
              <option key={formatKey} value={formatKey}>
                {formatKeyLabel(formatKey)}
              </option>
            ))}
          </optgroup>
        </select>
        {requiresLogFormatSpec(row.formatKey) ? (
          <div className="import-format-spec">
            <TextEditorPopover
              label={`${row.originPath} のログ書式を指定`}
              inputLabel="ログ書式"
              value={row.formatSpec}
              target={<RawText text={row.originPath} />}
              description="CLIの --logformat と同じ指定。書式文字列・組み込み名・squid.confのlogformat行を入力できます。"
              applyToGroupLabel="選択中のSquid資料に同じ書式を適用"
              onCommit={(spec, all) => onUpdateSpec(row.originPath, spec, all)}
            />
            <span>
              {row.formatSpec.trim() === ""
                ? "ログ書式が必要"
                : "ログ書式を指定済み"}
            </span>
          </div>
        ) : null}
      </td>
      <td data-label="記録した端末の表示名">
        <input
          type="text"
          aria-label={`${row.originPath} を記録した端末の表示名`}
          value={row.terminalHostname}
          onChange={(event) =>
            onUpdate(row.originPath, "terminalHostname", event.target.value)
          }
        />
      </td>
      <td data-label="詳しい指定">
        <details>
          <summary>開く</summary>
          {detailFields.map(({ field, label }) => (
            <label key={field} className="block">
              {label}
              <input
                type="text"
                aria-label={`${row.originPath} の${label}`}
                value={row[field]}
                onChange={(event) =>
                  onUpdate(row.originPath, field, event.target.value)
                }
              />
            </label>
          ))}
        </details>
      </td>
      <td data-label="外す">
        <IconButton
          label="収集元を外す"
          accessibleName={`${row.originPath} を外す`}
          onPress={() => onRemove(row.originPath)}
        >
          <Trash2 size={18} aria-hidden="true" />
        </IconButton>
      </td>
    </tr>
  );
});

/** 選んだ収集元の表。見えている行とその前後だけを描く。 */
function LoadingRowsTable({
  rows,
  formatKeys,
  onChangeRows,
}: {
  rows: LoadingFormRow[];
  formatKeys: string[];
  onChangeRows: Dispatch<SetStateAction<LoadingFormRow[]>>;
}) {
  // 行の並びが変わったときだけ、測った行の高さを捨てる。値の入力では捨てない。
  const rowsKey = useMemo(
    () => rows.map((row) => row.originPath).join("\n"),
    [rows],
  );
  const virtual = useVirtualRows(
    rows.length,
    rowsKey,
    estimatedRowHeight,
    overscanPx,
  );
  const onUpdate = useCallback(
    (originPath: string, field: LoadingFormField, value: string) =>
      onChangeRows((current) =>
        current.map((row) =>
          row.originPath === originPath ? { ...row, [field]: value } : row,
        ),
      ),
    [onChangeRows],
  );
  const onRemove = useCallback(
    (originPath: string) =>
      onChangeRows((current) =>
        current.filter((row) => row.originPath !== originPath),
      ),
    [onChangeRows],
  );
  const onUpdateSpec = useCallback(
    (originPath: string, spec: string, applyToSquid: boolean) =>
      onChangeRows((current) =>
        withLogFormatSpec(current, originPath, spec, applyToSquid),
      ),
    [onChangeRows],
  );
  // **行の並びが変わっても、画面の上端に見えていた行を上端に残す。** 行の並びが変わると測った高さを
  // 捨てるため、そのままでは見ていた位置から離れた行が出る。行を足すのと外すのは、この表と
  // directory の一覧のどちらからでも起きる。上端の行は、スクロールのたびにその path と位置で覚える。
  // 上端の行を外したときは、同じ位置の行を上端に出す。
  const rowsRef = useRef(rows);
  rowsRef.current = rows;
  const topRef = useRef<{ originPath: string; index: number } | undefined>(
    undefined,
  );
  const { firstVisibleRow, scrollToRow, onScroll } = virtual;
  const rememberTop = useCallback(() => {
    const index = firstVisibleRow();
    const row = rowsRef.current[index];
    topRef.current =
      row === undefined ? undefined : { originPath: row.originPath, index };
  }, [firstVisibleRow]);
  // scrollToRow は行の並びが変わるたびに作り直され、並びを変えた後の描画でこの effect が動く。
  useLayoutEffect(() => {
    const top = topRef.current;
    if (top === undefined) {
      return;
    }
    const found = rowsRef.current.findIndex(
      (row) => row.originPath === top.originPath,
    );
    scrollToRow(
      found >= 0 ? found : Math.min(top.index, rowsRef.current.length - 1),
    );
    rememberTop();
  }, [scrollToRow, rememberTop]);
  return (
    <section
      ref={virtual.scrollerRef}
      aria-label="選んだ収集元の表"
      // biome-ignore lint/a11y/noNoninteractiveTabindex: スクロールする領域をキーボードで動かせるようにする。
      tabIndex={0}
      onScroll={() => {
        onScroll();
        rememberTop();
      }}
      style={scrollerStyle}
      className="loading-rows-scroll"
    >
      <table style={virtualTableStyle} aria-rowcount={rows.length + 1}>
        <caption className="sr-only">選んだ収集元の入力形式と端末</caption>
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
          {rows.slice(virtual.start, virtual.end).map((row, offset) => (
            <LoadingRow
              key={row.originPath}
              row={row}
              rowIndex={virtual.start + offset}
              formatKeys={formatKeys}
              onUpdate={onUpdate}
              onRemove={onRemove}
              onUpdateSpec={onUpdateSpec}
            />
          ))}
        </tbody>
        <SpacerBody height={virtual.spaceAfter} columnCount={columns.length} />
      </table>
    </section>
  );
}

/**
 * 選んだ収集元の入力形式と端末を確かめ、読み込みを始める。入力形式を選んでいない収集元は
 * 読み込まない。
 */
export function LoadingForm({
  formatKeys,
  rows,
  onChangeRows,
  caseId,
  onChangeCaseId,
  thenProcess,
  onChangeThenProcess,
  onSubmit,
  isSubmitting,
  dragName,
  onDragOver,
  onDrop,
  onUploadFiles,
  uploadNotice,
}: LoadingFormProps) {
  const [isDropActive, setIsDropActive] = useState(false);
  const fileInputRef = useRef<HTMLInputElement>(null);
  const folderInputRef = useRef<HTMLInputElement>(null);
  const ready = rows.filter(hasFormat);
  const unselected = rows.length - ready.length;
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (isSubmitting || ready.length === 0) {
      return;
    }
    onSubmit(ready);
  };

  return (
    <form
      aria-label="読み込む収集元"
      className="stages-form import-panel"
      data-drop-active={isDropActive || undefined}
      onSubmit={submit}
      onDragOver={(event) => {
        onDragOver?.(event);
        if (event.dataTransfer.types.includes("Files")) {
          event.preventDefault();
        }
        setIsDropActive(event.defaultPrevented);
      }}
      onDragLeave={(event) => {
        if (
          !(event.relatedTarget instanceof Node) ||
          !event.currentTarget.contains(event.relatedTarget)
        ) {
          setIsDropActive(false);
        }
      }}
      onDrop={(event) => {
        onDrop?.(event);
        setIsDropActive(false);
      }}
    >
      <div className="import-panel-heading">
        <h4>読み込む収集元 ({formatCount(rows.length)} 件)</h4>
        <Button
          variant="ghost"
          size="sm"
          isDisabled={rows.length === 0 || isSubmitting}
          onPress={() => onChangeRows([])}
        >
          <Trash2 size={18} aria-hidden="true" />
          すべて外す
        </Button>
      </div>
      <div className="import-drop-target">
        <ArrowDownToLine size={20} aria-hidden="true" />
        <span>
          {dragName === undefined
            ? "ファイル・フォルダをドロップ"
            : `${dragName} を追加`}
        </span>
        <HelpPopover label="ドラッグで追加">
          一覧からドラッグして追加。PC上のファイル・フォルダもアップロード可能。チェックボックスと選択ボタンでも追加可能
        </HelpPopover>
      </div>
      {onUploadFiles === undefined ? null : (
        <div className="import-upload-actions">
          <input
            ref={fileInputRef}
            type="file"
            multiple
            hidden
            aria-label="アップロードするファイル"
            onChange={(event) => {
              onUploadFiles(Array.from(event.currentTarget.files ?? []));
              event.currentTarget.value = "";
            }}
          />
          <input
            ref={folderInputRef}
            type="file"
            multiple
            hidden
            aria-label="アップロードするフォルダ"
            {...{ webkitdirectory: "" }}
            onChange={(event) => {
              onUploadFiles(Array.from(event.currentTarget.files ?? []));
              event.currentTarget.value = "";
            }}
          />
          <Button
            variant="secondary"
            isDisabled={isSubmitting}
            onPress={() => fileInputRef.current?.click()}
          >
            PCからファイルを選択
          </Button>
          <Button
            variant="secondary"
            isDisabled={isSubmitting}
            onPress={() => folderInputRef.current?.click()}
          >
            PCからフォルダを選択
          </Button>
        </div>
      )}
      {uploadNotice}
      {rows.length === 0 ? (
        <p role="status" className="import-empty">
          未選択。ファイル一覧から追加
        </p>
      ) : (
        <LoadingRowsTable
          rows={rows}
          formatKeys={formatKeys}
          onChangeRows={onChangeRows}
        />
      )}
      {unselected > 0 ? (
        <p role="status">
          入力形式・ログ書式が未指定の {formatCount(unselected)}{" "}
          件は読み込まない
        </p>
      ) : null}
      <div className="import-settings">
        <div className="import-case">
          <label>
            案件
            <input
              type="text"
              value={caseId}
              onChange={(event) => onChangeCaseId(event.target.value)}
            />
          </label>
          <HelpPopover label="案件">
            すべての収集元に付ける案件の識別子。空のときは付けない
          </HelpPopover>
        </div>
        <label className="import-process-option">
          <input
            type="checkbox"
            checked={thenProcess}
            onChange={(event) => onChangeThenProcess(event.target.checked)}
          />
          読み込みの完了後に処理を始める
        </label>
      </div>
      <div className="import-submit-bar">
        <span className="import-ready-count">
          読み込み可能: {formatCount(ready.length)} 件
        </span>
        <Button
          type="submit"
          variant="primary"
          isDisabled={isSubmitting || ready.length === 0}
          disabledReason={
            ready.length === 0 && !isSubmitting
              ? { title: "収集元なし", text: "入力形式を選んだ収集元が必要" }
              : undefined
          }
        >
          <Play size={18} aria-hidden="true" />
          {formatCount(ready.length)} 件の読み込みを開始
        </Button>
      </div>
    </form>
  );
}

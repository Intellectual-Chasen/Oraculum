import type { SourceRef } from "@/shared/api/graph";
import type {
  DiagnosisClass,
  PublicationState,
  SourceIdentity,
} from "@/shared/contracts/sources";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import {
  diagnosisClassLabels,
  publicationStateLabels,
} from "@/shared/lib/sourceLabels";
import {
  type ContextMenuContent,
  type ContextMenuTriggers,
  RowMenuButton,
  useContextMenu,
} from "@/shared/ui/ContextMenu";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { type MenuEntry, menuSeparator } from "@/shared/ui/Menu";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import type { Status } from "@/shared/ui/StatusDot";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { useCopyText } from "@/shared/ui/useCopyText";
import {
  findDiagnosisCount,
  findImportCount,
  type SourceInventory,
  type SourceInventoryRow,
} from "./inventory";
import { withheldReasonLabels } from "./labels";

const countAbsent = "集計なし";
const recordCountAbsent = "未確定";
const caseAbsent = "案件の指定なし";

/** 一覧に件数の列を置く失敗原因の分類。 */
const countedDiagnoses = [
  "unsupported_format",
  "undetermined",
  "inconsistent_input_confirmed",
] as const satisfies readonly DiagnosisClass[];

/** 公開の状態ごとの状態の icon。 */
const publicationStatuses: Record<PublicationState, Status> = {
  published_full: "done",
  published_partial: "idle",
  withheld: "failed",
};

function CountCell({
  value,
  absence,
}: {
  value: number | undefined;
  absence: string;
}) {
  return (
    <td>
      {value === undefined ? (
        <MissingValue description={absence} />
      ) : (
        formatCount(value)
      )}
    </td>
  );
}

function PublicationCell({ row }: { row: SourceInventoryRow }) {
  const status = row.importStatus;
  const reason = status.withheldReason;
  return (
    <td>
      <StatusLabel
        status={publicationStatuses[status.publicationState]}
        label={publicationStateLabels[status.publicationState]}
        details={
          reason === undefined ? undefined : (
            <KeyValueList
              stacked
              pairs={[{ name: "理由", value: withheldReasonLabels[reason] }]}
            />
          )
        }
      />
    </td>
  );
}

/** 収集元を読み上げと見出しに出す名前。 */
function sourceMenuName(source: SourceIdentity): string {
  return `収集元 ${toVisibleRawText(source.fileName)} の操作`;
}

/** 収集元の一覧の行のコンテキストメニューが読む、今の選択と絞り込みと操作。 */
type SourceMenuContext = {
  selectedSourceId: string | undefined;
  narrowedSourceIds: readonly string[] | undefined;
  onSelect: (source: SourceIdentity) => void;
  onNarrowToSource: ((source: SourceRef) => void) | undefined;
  copy: (text: string, what: string) => void;
};

/**
 * 収集元 1 件のコンテキストメニューの項目を組む。選んでいる収集元では選ぶ項目を、今その収集元
 * だけで絞っているときは絞る項目を、残して使えなくする。絞る操作を持たない画面では絞る項目を
 * 出さない。
 */
function sourceMenuContent(
  source: SourceIdentity | undefined,
  context: SourceMenuContext,
): ContextMenuContent {
  if (source === undefined) {
    return { label: "収集元の操作", entries: [] };
  }
  const { narrowedSourceIds, onNarrowToSource } = context;
  const entries: MenuEntry[] = [
    {
      kind: "item",
      key: "select",
      label: "詳細を表示",
      disabled: source.sourceId === context.selectedSourceId,
      onSelect: () => context.onSelect(source),
    },
  ];
  if (onNarrowToSource !== undefined) {
    entries.push({
      kind: "item",
      key: "narrow",
      label: "この収集元のフィルタを適用",
      disabled:
        narrowedSourceIds?.length === 1 &&
        narrowedSourceIds[0] === source.sourceId,
      onSelect: () =>
        onNarrowToSource({ id: source.sourceId, label: source.fileName }),
    });
  }
  entries.push(menuSeparator("copy"), {
    kind: "item",
    key: "copy-sha256",
    label: "SHA-256 をコピー",
    onSelect: () => context.copy(source.contentSha256, "SHA-256"),
  });
  return { label: sourceMenuName(source), entries };
}

function SourceRow({
  row,
  isSelected,
  showsCase,
  onSelect,
  menu,
  isMenuOpen,
}: {
  row: SourceInventoryRow;
  isSelected: boolean;
  /** 案件の列を出すか。案件を持つ収集元が一覧に 1 件以上あるときに真である。 */
  showsCase: boolean;
  onSelect: (source: SourceIdentity) => void;
  /** 行の操作のメニューを開く操作。対象は収集元の sourceId である。 */
  menu: ContextMenuTriggers<string>;
  isMenuOpen: boolean;
}) {
  const { source, importStatus } = row;
  return (
    <tr
      onContextMenu={(event) => menu.onContextMenu(event, source.sourceId)}
      onKeyDown={(event) => menu.onKeyDown(event, source.sourceId)}
    >
      <td>
        <button
          type="button"
          aria-pressed={isSelected}
          onClick={() => onSelect(source)}
        >
          <RawText text={source.fileName} />
        </button>
      </td>
      {showsCase ? (
        <td>{source.caseId ?? <MissingValue description={caseAbsent} />}</td>
      ) : null}
      <td className="wrapping-cell">{source.contentSha256}</td>
      <CountCell value={source.recordCount} absence={recordCountAbsent} />
      <CountCell
        value={findImportCount(importStatus.counts, "read")}
        absence={countAbsent}
      />
      <CountCell
        value={findImportCount(importStatus.counts, "succeeded")}
        absence={countAbsent}
      />
      {countedDiagnoses.map((diagnosis) => (
        <CountCell
          key={diagnosis}
          value={findDiagnosisCount(importStatus.diagnosisCounts, diagnosis)}
          absence={countAbsent}
        />
      ))}
      <PublicationCell row={row} />
      <td>
        <RowMenuButton
          label={sourceMenuName(source)}
          expanded={isMenuOpen}
          onClick={(event) => menu.onButtonClick(event, source.sourceId)}
        />
      </td>
    </tr>
  );
}

type SourceInventoryTableProps = {
  inventory: SourceInventory;
  selectedSourceId: string | undefined;
  onSelect: (source: SourceIdentity) => void;
  /** 根拠のレコードを今絞っている収集元の sourceId。状態の所有者は上位の画面である。 */
  narrowedSourceIds?: readonly string[] | undefined;
  /** 行の収集元だけに根拠のレコードを絞る。出ない場合はメニューに項目を出さない。 */
  onNarrowToSource?: (source: SourceRef) => void;
};

/**
 * 収集元ごとのファイル名・SHA-256・件数・公開の状態を 1 行で出す。行の右クリックと Shift+F10 と
 * 「…」の button から、詳細の表示・フィルタの適用・SHA-256 のコピーのメニューを開く。
 */
export function SourceInventoryTable({
  inventory,
  selectedSourceId,
  onSelect,
  narrowedSourceIds,
  onNarrowToSource,
}: SourceInventoryTableProps) {
  const { copy, notice } = useCopyText();
  // 対象は sourceId で持ち、項目は描画ごとに今の一覧から組む。
  const { triggers, openTarget, menu } = useContextMenu((sourceId: string) =>
    sourceMenuContent(
      inventory.rows.find((row) => row.source.sourceId === sourceId)?.source,
      { selectedSourceId, narrowedSourceIds, onSelect, onNarrowToSource, copy },
    ),
  );
  const showsCase = inventory.rows.some(
    (row) => row.source.caseId !== undefined,
  );
  return (
    <>
      <table>
        <caption className="sr-only">収集元</caption>
        <thead>
          <tr>
            <th scope="col">収集元</th>
            {showsCase ? <th scope="col">案件</th> : null}
            <th scope="col">SHA-256</th>
            <th scope="col">レコード件数</th>
            <th scope="col">読み込み</th>
            <th scope="col">解析の成功</th>
            {countedDiagnoses.map((diagnosis) => (
              <th key={diagnosis} scope="col">
                {diagnosisClassLabels[diagnosis]}
              </th>
            ))}
            <th scope="col">公開の状態</th>
            <th scope="col">操作</th>
          </tr>
        </thead>
        <tbody>
          {inventory.rows.map((row) => (
            <SourceRow
              key={row.source.sourceId}
              row={row}
              isSelected={row.source.sourceId === selectedSourceId}
              showsCase={showsCase}
              onSelect={onSelect}
              menu={triggers}
              isMenuOpen={openTarget === row.source.sourceId}
            />
          ))}
        </tbody>
      </table>
      {notice}
      {menu}
    </>
  );
}

import type { SourceRef } from "@/shared/api/graph";
import type { RecordLocator } from "@/shared/contracts/common";
import type { SourceIdentity } from "@/shared/contracts/sources";
import type { TerminalAssignment } from "@/shared/contracts/terminalAssignments";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import type { SourceInventory as Inventory } from "./inventory";
import { SkippedFileTable } from "./SkippedFileTable";
import { SourceDetail } from "./SourceDetail";
import { SourceInventoryTable } from "./SourceInventoryTable";

type SourceInventoryProps = {
  /**
   * 収集元の一覧の取得の状態。グラフの探索も一覧から案件を読むため、上位の画面が
   * `useSourceInventory` で取得して持つ。
   */
  state: FetchState<Inventory>;
  /** 選んでいる収集元。状態の所有者は上位の画面である。 */
  selectedSource: SourceIdentity | undefined;
  onSelect: (source: SourceIdentity) => void;
  /**
   * 途中で切れた行の読めた欄と、説明を組めなかったレコードを開く。レコードを開く画面を
   * 持たない段階では渡さず、開く操作を出さない。
   */
  onSelectRecord?: (recordRef: RecordLocator) => void;
  /** 利用者が与えた端末の割当の取得の状態。起動で指定した端末を選んだ収集元の詳細に出す。 */
  terminalAssignments?: FetchState<readonly TerminalAssignment[]>;
  /** 根拠のレコードを今絞っている収集元の sourceId。状態の所有者は上位の画面である。 */
  narrowedSourceIds?: readonly string[] | undefined;
  /** 行のメニューから、その収集元だけに根拠のレコードを絞る。出ない場合は項目を出さない。 */
  onNarrowToSource?: (source: SourceRef) => void;
};

/** 収集元の一覧を表示し、選んだ収集元を上位へ渡す (操作 1)。 */
export function SourceInventory({
  state,
  selectedSource,
  onSelect,
  onSelectRecord,
  terminalAssignments,
  narrowedSourceIds,
  onNarrowToSource,
}: SourceInventoryProps) {
  return (
    <section aria-label="収集元の一覧">
      <FetchStateView
        state={state}
        loadingDescription="収集元の一覧の読み込み中"
      >
        {(inventory) => (
          <>
            <KeyValueList
              pairs={[
                { name: "収集元", value: formatCount(inventory.sourceCount) },
              ]}
            />
            <SourceInventoryTable
              inventory={inventory}
              selectedSourceId={selectedSource?.sourceId}
              onSelect={onSelect}
              narrowedSourceIds={narrowedSourceIds}
              onNarrowToSource={onNarrowToSource}
            />
            <SkippedFileTable files={inventory.skippedFiles} />
            <SourceDetail
              row={inventory.rows.find(
                (row) => row.source.sourceId === selectedSource?.sourceId,
              )}
              onSelectRecord={onSelectRecord}
              terminalAssignments={terminalAssignments}
            />
          </>
        )}
      </FetchStateView>
    </section>
  );
}

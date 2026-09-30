import type { NodeRef, SourceRef } from "@/shared/api/graph";
import type { RecordLocator } from "@/shared/contracts/common";
import type { TimelineEntry } from "@/shared/contracts/timeline";
import { toVisibleRawText } from "@/shared/lib/rawText";
import {
  describeRecordLocation,
  recordPositionValue,
} from "@/shared/lib/recordPosition";
import type { ContextMenuContent } from "@/shared/ui/ContextMenu";
import { type MenuEntry, menuSeparator } from "@/shared/ui/Menu";

/** 時系列の行のコンテキストメニューが呼ぶ操作と、項目を使えるかを決める今の絞り込み。 */
export type TimelineRowActions = {
  onSelectRecord: (recordRef: RecordLocator) => void;
  /** 前面のビューを変えずに元レコードの表示へ渡す。 */
  onPreviewRecord: (recordRef: RecordLocator) => void;
  /** 行の端末で根拠のレコードを絞る。出ない場合は項目を出さない。 */
  onNarrowToTerminal?: (terminal: NodeRef) => void;
  /** 行の収集元だけに根拠のレコードを絞る。出ない場合は項目を出さない。 */
  onNarrowToSource?: (source: SourceRef) => void;
  /** 今絞っている端末のノードの識別子。 */
  terminal: string | undefined;
  /** 今絞っている収集元の sourceId。 */
  sources: readonly string[] | undefined;
  /** 行のレコードのブックマークを付け外しする。出ない場合は項目を出さない。 */
  bookmark?: TimelineRowBookmark | undefined;
};

/** 行のブックマーク。メニューを開いたときに付けてあるかを読む。 */
export type TimelineRowBookmark = {
  isMarked: (entry: TimelineEntry) => boolean;
  onToggle: (entry: TimelineEntry) => void;
};

/**
 * 時系列の行 1 件のコンテキストメニューの項目を組む。
 *
 * **端末で絞る項目は、行が端末を持つときだけ出す。** 今の絞り込みと同じ端末・同じ収集元だけで
 * 絞っているときは、項目を残して使えなくする。
 */
export function timelineRowMenuContent(
  entry: TimelineEntry,
  actions: TimelineRowActions,
  copy: (text: string, what: string) => void,
): ContextMenuContent {
  const { recordRef, terminal } = entry;
  const entries: MenuEntry[] = [
    {
      kind: "item",
      key: "open",
      label: "Record に表示",
      onSelect: () => actions.onSelectRecord(recordRef),
    },
    {
      kind: "item",
      key: "preview",
      label: "Record を前面に出さずに表示",
      onSelect: () => actions.onPreviewRecord(recordRef),
    },
  ];
  const { bookmark } = actions;
  if (bookmark !== undefined) {
    entries.push({
      kind: "item",
      key: "bookmark",
      label: bookmark.isMarked(entry)
        ? "ブックマークから削除"
        : "ブックマークに追加",
      onSelect: () => bookmark.onToggle(entry),
    });
  }
  const narrowing: MenuEntry[] = [];
  const { onNarrowToTerminal, onNarrowToSource } = actions;
  if (terminal !== undefined && onNarrowToTerminal !== undefined) {
    narrowing.push({
      kind: "item",
      key: "terminal",
      label: "この端末でフィルタ",
      disabled: actions.terminal === terminal.id,
      onSelect: () =>
        onNarrowToTerminal({
          id: terminal.id,
          label:
            terminal.label.rawText ?? terminal.label.normalized ?? terminal.id,
          kind: terminal.kind,
        }),
    });
  }
  if (onNarrowToSource !== undefined) {
    narrowing.push({
      kind: "item",
      key: "source",
      label: "この収集元でフィルタ",
      disabled:
        actions.sources?.length === 1 &&
        actions.sources[0] === recordRef.sourceId,
      onSelect: () =>
        onNarrowToSource({
          id: recordRef.sourceId,
          label: recordRef.sourceFileName,
        }),
    });
  }
  if (narrowing.length > 0) {
    entries.push(menuSeparator("narrow"), ...narrowing);
  }
  // コピーは接頭辞の無い値を書く。位置の値を持たないレコードには項目を出さない。
  const position = recordPositionValue(recordRef);
  if (position !== undefined) {
    entries.push(menuSeparator("copy"), {
      kind: "item",
      key: "copy-location",
      label: "位置をコピー",
      onSelect: () => copy(position, "レコードの位置"),
    });
  }
  return {
    label: `${toVisibleRawText(describeRecordLocation(recordRef))} の操作`,
    entries,
  };
}

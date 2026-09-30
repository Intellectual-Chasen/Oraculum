import { Bookmark, BookmarkCheck } from "lucide-react";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { IconButton } from "@/shared/ui/IconButton";

/** 対象 1 件のブックマークを付け外しする button。tooltip と読み上げに対象の名前を出す。 */
export function BookmarkToggle({
  name,
  marked,
  onToggle,
}: {
  /** 対象の名前。原文の文字列を含んでよい。 */
  name: string;
  marked: boolean;
  onToggle: () => void;
}) {
  const Icon = marked ? BookmarkCheck : Bookmark;
  return (
    <IconButton
      label={
        marked
          ? `${toVisibleRawText(name)} をブックマークから削除`
          : `${toVisibleRawText(name)} をブックマークに追加`
      }
      onPress={onToggle}
    >
      <Icon className="size-3.5" aria-hidden="true" />
    </IconButton>
  );
}

import { Plus } from "lucide-react";
import { toVisibleRawText } from "../lib/rawText";
import { IconButton } from "./IconButton";

/**
 * 原文の値 1 つを、検索の「含む文字列」の条件に追加する icon の button。
 * 読み上げの名前と tooltip は「含む条件に追加: 値」とし、どの値を追加するかを示す。
 */
export function AddTermButton({
  text,
  onAdd,
}: {
  /** 追加する値。原文の文字列である。 */
  text: string;
  onAdd: (text: string) => void;
}) {
  return (
    <IconButton
      label={`含む条件に追加: ${toVisibleRawText(text)}`}
      onPress={() => onAdd(text)}
    >
      <Plus size={14} aria-hidden="true" />
    </IconButton>
  );
}

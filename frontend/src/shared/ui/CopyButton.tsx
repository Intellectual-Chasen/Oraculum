import { Copy } from "lucide-react";
import { toVisibleRawText } from "../lib/rawText";
import { IconButton } from "./IconButton";

/**
 * 値をクリップボードにコピーする icon の button。
 * 読み上げの名前と tooltip は「コピー: 値」とし、どの値をコピーするかを示す。
 */
export function CopyButton({
  text,
  onCopy,
  label = `コピー: ${toVisibleRawText(text)}`,
}: {
  /** コピーする値。 */
  text: string;
  onCopy: (text: string) => void;
  label?: string;
}) {
  return (
    <IconButton label={label} onPress={() => onCopy(text)}>
      <Copy size={14} aria-hidden="true" />
    </IconButton>
  );
}

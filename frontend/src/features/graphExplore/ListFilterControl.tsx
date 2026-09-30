import { X } from "lucide-react";
import { formatCount } from "@/shared/lib/format";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { TextField } from "@/shared/ui/TextField";

/** 取得済み一覧の種類と文字列を絞り、一致した件数を表示する。 */
export function ListFilter({
  name,
  kinds,
  kind,
  text,
  onKindChange,
  onTextChange,
  matched,
  total,
}: {
  name: string;
  kinds: readonly { value: string; label: string }[];
  kind: string;
  text: string;
  onKindChange: (value: string) => void;
  onTextChange: (value: string) => void;
  matched: number;
  total: number;
}) {
  return (
    <div className="mb-2 flex flex-wrap items-end gap-2">
      <label className="flex min-w-0 flex-col gap-1 text-sm font-medium">
        一覧の{name}の種類
        <select
          className="w-48 max-w-full"
          value={kind}
          onChange={(event) => onKindChange(event.target.value)}
        >
          <option value="">すべて</option>
          {kinds.map((option) => (
            <option key={option.value} value={option.value}>
              {option.label}
            </option>
          ))}
        </select>
      </label>
      <TextField
        label={`${name}の文字列`}
        type="search"
        value={text}
        onChange={onTextChange}
        className="min-w-40 flex-1"
      />
      <HelpPopover label={`${name}の文字列`}>
        <KeyValueList
          stacked
          pairs={[
            {
              name: "対象",
              value:
                name === "エッジ"
                  ? "種類・作り方・始点と終点の表示名・同一性の値・種類"
                  : "表示名・同一性の値・種類",
            },
            { name: "大文字と小文字", value: "区別なし" },
            { name: "範囲", value: "取得済みの一覧" },
          ]}
        />
      </HelpPopover>
      <IconButton
        label={`${name}の絞り込みを解除`}
        onPress={() => {
          onKindChange("");
          onTextChange("");
        }}
      >
        <X size={14} aria-hidden="true" />
      </IconButton>
      <div role="status" aria-label={`${name}の一致件数`}>
        <KeyValueList
          pairs={[
            {
              name: "一覧内の一致",
              value: `${formatCount(matched)} / ${formatCount(total)}`,
            },
          ]}
        />
      </div>
    </div>
  );
}

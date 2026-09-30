import { CalendarDays } from "lucide-react";
import { useRef } from "react";
import { IconButton } from "./IconButton";
import { TextField, type TextFieldProps } from "./TextField";

type TimestampFieldProps = Omit<TextFieldProps, "value" | "onChange"> & {
  value: string;
  onChange: (value: string) => void;
  /** 空の欄から日時を選ぶときの UTC オフセット。端末時刻には空文字列を渡す。 */
  defaultOffset: string;
};

/**
 * 時刻の入力欄と、ブラウザ標準の日時選択。日時選択は秒までを変え、入力済みの
 * 秒の小数部と UTC オフセットを保つ。細かい時刻とオフセットは文字列の欄で編集できる。
 */
export function TimestampField({
  value,
  onChange,
  defaultOffset,
  ...props
}: TimestampFieldProps) {
  const picker = useRef<HTMLInputElement>(null);
  const parts =
    /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(\.\d+)?(Z|[+-]\d{2}:\d{2})?$/.exec(
      value.trim(),
    );
  return (
    <div className="flex items-start gap-1">
      <TextField
        {...props}
        className="min-w-0 flex-1"
        value={value}
        onChange={onChange}
      />
      <div className="relative pt-6">
        <input
          ref={picker}
          type="datetime-local"
          step="1"
          aria-label={`${props.label}の日時選択`}
          className="sr-only"
          tabIndex={-1}
          value={parts?.[1] ?? ""}
          onChange={(event) => {
            const selected = event.target.value;
            if (selected === "") {
              onChange("");
              return;
            }
            const seconds =
              selected.length === 16 ? `${selected}:00` : selected;
            const fraction = parts?.[2] ?? "";
            const offset = parts === null ? defaultOffset : (parts[3] ?? "");
            onChange(`${seconds}${fraction}${offset}`);
          }}
        />
        <IconButton
          label={`${props.label}をカレンダーで選択`}
          onPress={() => picker.current?.showPicker()}
        >
          <CalendarDays size={16} aria-hidden="true" />
        </IconButton>
      </div>
    </div>
  );
}

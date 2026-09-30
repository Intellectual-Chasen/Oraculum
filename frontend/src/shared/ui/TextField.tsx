import { CircleAlert } from "lucide-react";
import {
  TextField as AriaTextField,
  type TextFieldProps as AriaTextFieldProps,
  FieldError,
  Input,
  Label,
  Text,
  TextArea,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { cn } from "./cn";

export const inputVariants = tv({
  base: [
    "h-(--control-height) w-full rounded-sm border border-line bg-surface px-2 text-base text-ink",
    "outline-none transition-colors placeholder:text-faint",
    "data-[hovered]:border-muted",
    "data-[focused]:border-accent data-[focused]:ring-2 data-[focused]:ring-accent/20",
    "data-[invalid]:border-danger data-[invalid]:data-[focused]:ring-danger/20",
    "data-[disabled]:bg-ground data-[disabled]:text-faint",
  ],
  variants: {
    mono: { true: "font-mono" },
    multiline: { true: "h-auto min-h-16 py-1" },
  },
});

/**
 * 入力欄を縦に並べる form と fieldset の見た目。欄の左端をそろえ、欄の幅を読める長さに抑える。
 * fieldset の枠は付けない。
 */
export const formFieldsClass =
  "m-0 grid min-w-0 max-w-xl justify-items-stretch gap-3 border-0 p-0";

export type TextFieldProps = Omit<AriaTextFieldProps, "className"> & {
  label: string;
  /** 何を入れるかの説明。label の下ではなく入力欄の下に出す。 */
  description?: string;
  /** 入力の誤り。渡すと欄を誤りの状態にし、欄の下に出す。 */
  errorMessage?: string;
  placeholder?: string;
  /** 識別子・数値・時刻のように、等幅で比べる値の欄。 */
  mono?: boolean;
  /** 複数行の文を書く欄。 */
  multiline?: boolean;
  className?: string;
};

/**
 * label・説明・誤りを持つ入力欄。説明と誤りは React Aria が入力欄の
 * aria-describedby に結び付ける。
 */
export function TextField({
  label,
  description,
  errorMessage,
  placeholder,
  mono,
  multiline,
  className,
  ...props
}: TextFieldProps) {
  const field = inputVariants({ mono, multiline });
  return (
    <AriaTextField
      {...props}
      isInvalid={errorMessage !== undefined || props.isInvalid}
      className={cn("flex flex-col gap-1", className)}
    >
      <Label className="text-sm font-medium text-ink">{label}</Label>
      {multiline ? (
        <TextArea placeholder={placeholder} className={field} />
      ) : (
        <Input placeholder={placeholder} className={field} />
      )}
      {description === undefined ? null : (
        <Text slot="description" className="text-xs text-muted">
          {description}
        </Text>
      )}
      <FieldError className="flex items-center gap-1 text-xs text-danger">
        <CircleAlert className="size-3 shrink-0" aria-hidden="true" />
        {errorMessage}
      </FieldError>
    </AriaTextField>
  );
}

import type { ReactNode } from "react";
import { Button, type ButtonProps } from "./Button";

/**
 * icon だけを描く button。`label` を hover・focus の tooltip に使い、`accessibleName` が無ければ
 * 読み上げの名前にも使う。押せないときは `disabledReason` の tooltip を出す。
 */
export function IconButton({
  label,
  accessibleName = label,
  children,
  variant = "ghost",
  ...props
}: Omit<ButtonProps, "children" | "aria-label" | "size" | "tooltip"> & {
  label: string;
  /**
   * 読み上げの名前。行ごとに同じ操作の button が並ぶときに、対象の名前と操作の組を渡す。
   * 例: 「ワークスペース 2 を削除」。
   */
  accessibleName?: string;
  /** lucide の icon。`aria-hidden` を付けて渡す。 */
  children: ReactNode;
}) {
  return (
    <Button
      {...props}
      variant={variant}
      size="iconSm"
      aria-label={accessibleName}
      tooltip={label}
    >
      {children}
    </Button>
  );
}

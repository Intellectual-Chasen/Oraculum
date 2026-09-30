import { tv, type VariantProps } from "tailwind-variants";
import { cn } from "./cn";

const dot = tv({
  base: "inline-block size-2 shrink-0 rounded-full",
  variants: {
    status: {
      // 処理中だけを脈動させ、目を向ける先を 1 つにする。動きを減らす設定では止める。
      running: "bg-running animate-pulse motion-reduce:animate-none",
      done: "bg-done",
      failed: "bg-failed",
      pending: "bg-pending",
      idle: "bg-idle",
    },
  },
});

export type Status = NonNullable<VariantProps<typeof dot>["status"]>;

/**
 * 処理の状態を示す点と文。点は飾りとして読み上げから外し、状態は文で伝える。
 * 色だけで状態を伝えないため、文を必ず渡す。
 */
export function StatusDot({
  status,
  children,
  className,
}: {
  status: Status;
  children: string;
  className?: string;
}) {
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 text-sm text-muted",
        className,
      )}
    >
      <span className={dot({ status })} aria-hidden="true" />
      {children}
    </span>
  );
}

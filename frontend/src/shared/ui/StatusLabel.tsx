import {
  CircleCheck,
  CircleMinus,
  CircleX,
  Clock,
  LoaderCircle,
  type LucideIcon,
} from "lucide-react";
import { type ReactNode, useId } from "react";
import { Focusable } from "react-aria-components";
import { tv } from "tailwind-variants";
import { cn } from "./cn";
import type { Status } from "./StatusDot";
import { Tooltip, TooltipTrigger } from "./Tooltip";

const icons: Record<Status, LucideIcon> = {
  running: LoaderCircle,
  done: CircleCheck,
  failed: CircleX,
  pending: Clock,
  idle: CircleMinus,
};

const icon = tv({
  base: "shrink-0",
  variants: {
    status: {
      // 処理中だけを回し、目を向ける先を 1 つにする。動きを減らす設定では止める。
      running: "text-running animate-spin motion-reduce:animate-none",
      done: "text-done",
      failed: "text-failed",
      pending: "text-pending",
      idle: "text-idle",
    },
  },
});

/**
 * 状態の icon と短いラベル。色だけで状態を伝えないため、ラベルを必ず渡す。
 *
 * `details` を渡すと、ラベルは focus を受け、hover と focus で `details` を tooltip に出す。
 * `details` は tooltip が閉じていても読み上げに届くよう、画面に出さない文としても結び付ける。
 */
export function StatusLabel({
  status,
  label,
  details,
  className,
}: {
  status: Status;
  /** 状態の短いラベル。文にしない。 */
  label: string;
  /** 状態の詳細。`KeyValueList` の値の組を渡す。 */
  details?: ReactNode;
  className?: string;
}) {
  const detailsId = useId();
  const Icon = icons[status];
  const bodyClass = cn("inline-flex items-center gap-1.5 text-sm", className);
  const content = (
    <>
      <Icon size={14} aria-hidden="true" className={icon({ status })} />
      {label}
    </>
  );
  if (details === undefined) {
    return <span className={bodyClass}>{content}</span>;
  }
  return (
    <>
      <TooltipTrigger delay={300}>
        <Focusable>
          {/* 詳細を持つラベルは keyboard で tooltip を開けるよう focus を受ける。focus を受ける
              要素は読み上げが説明を読む role を持つ必要があるため、img の role で名前を渡す。 */}
          <span
            className={bodyClass}
            // biome-ignore lint/a11y/noNoninteractiveTabindex: 詳細の tooltip を keyboard で開くため focus を受ける。
            tabIndex={0}
            role="img"
            aria-label={label}
            aria-describedby={detailsId}
          >
            {content}
          </span>
        </Focusable>
        <Tooltip>{details}</Tooltip>
      </TooltipTrigger>
      <span id={detailsId} className="sr-only">
        {details}
      </span>
    </>
  );
}

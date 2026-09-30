import { createContext, type ReactNode, useContext } from "react";
import { Button as AriaButton } from "react-aria-components";
import type { RecordLocator, Timestamp } from "../contracts/common";
import { localTextOf, utcTextOf } from "../lib/timestampInstant";
import type { ValueCondition } from "../lib/valueCondition";
import { cn } from "./cn";
import { useDisplayOffset } from "./DisplayOffset";
import { Hint } from "./Hint";
import { Tooltip, TooltipTrigger } from "./Tooltip";

/** 表の値が指すもの。click で、その値を扱うビューに表示する。 */
export type ValueTarget =
  | { kind: "node"; id: string; label: string }
  | { kind: "edge"; id: string }
  | { kind: "record"; ref: RecordLocator }
  /** UTC の RFC 3339 の文字列。 */
  | { kind: "time"; utc: string };

/**
 * 表の値の操作。実装は画面を組み立てる側が provider で渡す。
 *
 * - `open`: 値を、その値を扱うビューに表示する。
 * - `addCondition`: 値から作った条件 (文字列・フィールドの値・イベントの種類・期間) を、検索の条件に
 *   追加する。
 */
export type ValueActions = {
  open: (target: ValueTarget) => void;
  addCondition: (condition: ValueCondition) => void;
};

/** 表の値の操作。provider が無い画面では、値を操作できない文字列として出す。 */
export const ValueActionsContext = createContext<ValueActions | undefined>(
  undefined,
);

/** 表の値の操作を返す。provider が無い画面では undefined を返す。 */
export function useValueActions(): ValueActions | undefined {
  return useContext(ValueActionsContext);
}

/**
 * 操作できる表の値。click でその値を扱うビューに表示し、hover と focus で `hover` を出す。
 * `hover` には、行が持つ値だけを渡す。`label` を渡すと、読み上げの名前を値の文字列から替える。
 */
export function ValueLink({
  target,
  hover,
  label,
  className,
  children,
}: {
  target: ValueTarget;
  hover: ReactNode;
  label?: string;
  className?: string;
  children: ReactNode;
}) {
  const actions = useValueActions();
  if (actions === undefined) {
    return <>{children}</>;
  }
  return (
    <TooltipTrigger delay={300}>
      <AriaButton
        className={cn("value-link", className)}
        aria-label={label}
        onPress={() => actions.open(target)}
      >
        {children}
      </AriaButton>
      <Tooltip>{hover}</Tooltip>
    </TooltipTrigger>
  );
}

/**
 * 時刻の値。click で Timeline のその時刻へ移動し、hover で UTC 時刻と表示のタイムゾーンの時刻を
 * 出す。時点が定まらない時刻は操作できない文字列として出す。
 *
 * `details` は「名前: 値」の補足の行。押せる時刻は同じ tooltip に足し、読み上げにも渡す。押せない
 * 時刻は `Hint` の tooltip に出す。
 */
export function TimeValueLink({
  timestamp,
  details = [],
  children,
}: {
  timestamp: Timestamp;
  details?: readonly string[];
  children: ReactNode;
}) {
  const offset = useDisplayOffset();
  const actions = useValueActions();
  const utc = utcTextOf(timestamp);
  const local = utc === undefined ? undefined : localTextOf(utc, offset);
  const lines = [
    ...(utc === undefined ? [] : [`UTC: ${utc}`]),
    ...(local === undefined ? [] : [`UTC${offset}: ${local}`]),
    ...details,
  ];
  if (utc === undefined || actions === undefined) {
    return details.length === 0 ? (
      children
    ) : (
      <Hint text={lines.join("\n")}>{children}</Hint>
    );
  }
  return (
    <ValueLink
      target={{ kind: "time", utc }}
      hover={lines.map((line) => (
        <span key={line} className="block">
          {line}
        </span>
      ))}
    >
      {children}
      {details.length === 0 ? null : (
        <span className="sr-only">{details.join("\n")}</span>
      )}
    </ValueLink>
  );
}

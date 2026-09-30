import type { ReactNode } from "react";
import {
  Tooltip as AriaTooltip,
  type TooltipProps as AriaTooltipProps,
  TooltipTrigger,
} from "react-aria-components";
import { tv } from "tailwind-variants";
import { usePortalContainer } from "./portalContainer";

// 開いたままの tooltip が、下にある tab や button の click を受け止めないようにする。
// 中の「名前: 値」の組の名前は、tooltip の地の上で読める色にする。
const tooltip = tv({
  base: "pointer-events-none max-w-72 rounded-md bg-ink px-2.5 py-2 text-xs text-surface shadow-float [&_.pair-name]:text-current [&_.pair-name]:opacity-75",
});

export type TooltipProps = Omit<AriaTooltipProps, "children" | "className"> & {
  /** tooltip の 1 行目に太く出す見出し。 */
  title?: string;
  children: ReactNode;
};

/** trigger の上か下に出す短い説明。hover と keyboard の focus で開き、Escape で閉じる。 */
export function Tooltip({
  title,
  children,
  offset = 6,
  ...props
}: TooltipProps) {
  // 既知の制限: UNSTABLE_portalContainer は react-aria-components が非推奨にした prop である。
  // 推奨の UNSAFE_PortalProvider を react-aria-components が公開していないため、これを使う。
  const portalContainer = usePortalContainer();
  return (
    <AriaTooltip
      {...props}
      offset={offset}
      UNSTABLE_portalContainer={portalContainer}
      className={tooltip()}
    >
      {title === undefined ? null : <p className="font-medium">{title}</p>}
      <div className={title === undefined ? undefined : "mt-0.5 opacity-80"}>
        {children}
      </div>
    </AriaTooltip>
  );
}

export { TooltipTrigger };

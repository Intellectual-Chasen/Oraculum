import { type ReactNode, useLayoutEffect, useRef, useState } from "react";
import { TooltipTriggerStateContext } from "react-aria-components";
import { cn } from "./cn";
import { Tooltip } from "./Tooltip";

/** keyboard で focus したかを返す。`:focus-visible` を解釈できない環境では focus を keyboard とみなす。 */
function isKeyboardFocus(element: Element): boolean {
  try {
    return element.matches(":focus-visible");
  } catch {
    return true;
  }
}

/** 表のセルと見出しの中にあるかを返す。 */
function isInTable(element: Element): boolean {
  return (
    element.closest(
      "td, th, [role=cell], [role=gridcell], [role=rowheader], [role=columnheader]",
    ) !== null
  );
}

/**
 * 補足を持つ値。マウスを重ねるとブラウザーの title の tooltip を出す。
 *
 * 表の外では focus を受け、keyboard の focus で同じ文字列の tooltip を出す。表のセルと見出しの
 * 中では focus を受けず、Tab を行の操作だけに止め、補足を sr-only の文字列で読み上げに渡す。
 */
export function Hint({
  text,
  children,
  className,
  mark = false,
}: {
  /** 補足の短い文字列。 */
  text: string;
  children: ReactNode;
  className?: string;
  /** 子が画面の印だけのとき、表の外でも補足を sr-only の文字列で読み上げに渡す。 */
  mark?: boolean;
}) {
  const ref = useRef<HTMLSpanElement>(null);
  const [open, setOpen] = useState(false);
  const [inTable, setInTable] = useState(false);
  useLayoutEffect(() => {
    if (ref.current !== null) setInTable(isInTable(ref.current));
  }, []);
  return (
    <>
      {/* biome-ignore lint/a11y/noStaticElementInteractions: 補足の値は押す操作を持たない。focus と Escape で tooltip だけを開閉する。 */}
      <span
        ref={ref}
        // 表の外の補足は、tooltip を keyboard で開くため focus を受ける。
        tabIndex={inTable ? undefined : 0}
        title={text}
        className={cn(
          "rounded-sm outline-none focus-visible:outline-2 focus-visible:outline-accent",
          className,
        )}
        onFocus={(event) => setOpen(isKeyboardFocus(event.currentTarget))}
        onBlur={() => setOpen(false)}
        onKeyDown={(event) => {
          if (event.key === "Escape") setOpen(false);
        }}
      >
        {children}
        {inTable || mark ? <span className="sr-only">{text}</span> : null}
      </span>
      {/* Tooltip は TooltipTrigger の外では状態を context から読むため、開閉の状態を渡す。 */}
      <TooltipTriggerStateContext
        value={{
          isOpen: open,
          shouldSkipAnimation: true,
          open: () => setOpen(true),
          close: () => setOpen(false),
        }}
      >
        <Tooltip triggerRef={ref}>
          {/* title と同じく、改行で区切った「名前: 値」の組を行ごとに出す。 */}
          <span className="whitespace-pre-line">{text}</span>
        </Tooltip>
      </TooltipTriggerStateContext>
    </>
  );
}

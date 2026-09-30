import {
  Fragment,
  type KeyboardEvent,
  useEffect,
  useId,
  useRef,
  useState,
} from "react";
import { Menu, type MenuEntry } from "./Menu";
import {
  indexByInitial,
  isMenuBarKey,
  typeaheadCharacter,
  wrapIndex,
} from "./menuKeys";
import { anchorBelow, type MenuAnchor } from "./menuPlacement";

/** メニューバーの見出し 1 つと、その見出しが開くメニューの項目。 */
export type MenuBarMenu = {
  /** 並びの中で見出しを見分ける値。表示に出さない。 */
  key: string;
  label: string;
  entries: readonly MenuEntry[];
};

type OpenMenu = {
  index: number;
  anchor: MenuAnchor;
  initialFocus: "first" | "last";
};

/**
 * 画面の上に置くメニューバー。WAI-ARIA の menubar の型に合わせてキーボードで操作する。
 *
 * - F10 で最初の見出しへ focus を移す。見出しは 1 つだけが Tab で止まる (roving tabindex)。
 * - 左右の矢印キーで見出しを移り、端では反対の端へ回る。メニューを開いている間は、隣の見出しの
 *   メニューを開き直す。
 * - 下の矢印キーと Enter と Space でメニューを開き、先頭の項目に focus を置く。上の矢印キーは
 *   末尾の項目に置く。見出しを押すとメニューを開き閉じする。
 */
export function MenuBar({
  label,
  menus,
}: {
  /** 読み上げに渡すメニューバーの名前。 */
  label: string;
  menus: readonly MenuBarMenu[];
}) {
  const barRef = useRef<HTMLDivElement>(null);
  const headingRefs = useRef<(HTMLButtonElement | null)[]>([]);
  const [current, setCurrent] = useState(0);
  const [open, setOpen] = useState<OpenMenu | undefined>(undefined);
  const baseId = useId();

  const focusHeading = (index: number) => {
    setCurrent(index);
    headingRefs.current[index]?.focus();
  };
  const openMenu = (index: number, initialFocus: "first" | "last") => {
    const heading = headingRefs.current[index];
    if (heading === null || heading === undefined) return;
    setCurrent(index);
    setOpen({ index, anchor: anchorBelow(heading), initialFocus });
  };

  useEffect(() => {
    const ownerDocument = barRef.current?.ownerDocument;
    if (ownerDocument === undefined) return;
    const focusFirst = (event: globalThis.KeyboardEvent) => {
      if (!isMenuBarKey(event)) return;
      event.preventDefault();
      setOpen(undefined);
      setCurrent(0);
      headingRefs.current[0]?.focus();
    };
    ownerDocument.addEventListener("keydown", focusFirst);
    return () => ownerDocument.removeEventListener("keydown", focusFirst);
  }, []);

  const onHeadingKeyDown = (
    event: KeyboardEvent<HTMLButtonElement>,
    index: number,
  ) => {
    switch (event.key) {
      case "ArrowLeft":
      case "ArrowRight": {
        event.preventDefault();
        const next = wrapIndex(
          index,
          event.key === "ArrowRight" ? 1 : -1,
          menus.length,
        );
        if (open === undefined) focusHeading(next);
        else openMenu(next, "first");
        return;
      }
      case "Home":
        event.preventDefault();
        focusHeading(0);
        return;
      case "End":
        event.preventDefault();
        focusHeading(menus.length - 1);
        return;
      case "ArrowDown":
      case "Enter":
      case " ":
        event.preventDefault();
        openMenu(index, "first");
        return;
      case "ArrowUp":
        event.preventDefault();
        openMenu(index, "last");
        return;
      case "Escape":
        setOpen(undefined);
        return;
      default: {
        const character = typeaheadCharacter(event);
        if (character === undefined) return;
        const next = indexByInitial(
          menus.map((menu) => menu.label),
          index,
          character,
        );
        if (next !== undefined) {
          event.preventDefault();
          focusHeading(next);
        }
      }
    }
  };

  return (
    <div ref={barRef} role="menubar" aria-label={label} className="menubar">
      {menus.map((menu, index) => {
        const headingId = `${baseId}-heading-${index}`;
        const menuId = `${baseId}-menu-${index}`;
        const opened = open?.index === index ? open : undefined;
        return (
          <Fragment key={menu.key}>
            <button
              ref={(element) => {
                headingRefs.current[index] = element;
              }}
              id={headingId}
              type="button"
              role="menuitem"
              aria-haspopup="menu"
              aria-expanded={opened !== undefined}
              aria-controls={opened === undefined ? undefined : menuId}
              tabIndex={index === current ? 0 : -1}
              onFocus={() => setCurrent(index)}
              onKeyDown={(event) => onHeadingKeyDown(event, index)}
              onClick={() => {
                if (opened === undefined) openMenu(index, "first");
                else setOpen(undefined);
              }}
              onPointerEnter={() => {
                // 開いている間は、指した見出しのメニューへ替える。
                if (open !== undefined && open.index !== index) {
                  openMenu(index, "first");
                }
              }}
            >
              {menu.label}
            </button>
            {opened === undefined ? null : (
              <Menu
                id={menuId}
                labelledBy={headingId}
                entries={menu.entries}
                anchor={opened.anchor}
                initialFocus={opened.initialFocus}
                returnFocus={headingRefs.current[index] ?? null}
                ignoreOutside={barRef.current}
                // 隣のメニューへ移った後に、前のメニューの閉じる通知で閉じない。
                onClose={() =>
                  setOpen((now) => (now?.index === index ? undefined : now))
                }
                onMoveSideways={(step) =>
                  openMenu(wrapIndex(index, step, menus.length), "first")
                }
              />
            )}
          </Fragment>
        );
      })}
    </div>
  );
}

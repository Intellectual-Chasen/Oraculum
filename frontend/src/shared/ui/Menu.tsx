import {
  type FocusEvent,
  type KeyboardEvent,
  type PointerEvent,
  useEffect,
  useLayoutEffect,
  useRef,
  useState,
} from "react";
import { createPortal } from "react-dom";
import { indexByInitial, typeaheadCharacter, wrapIndex } from "./menuKeys";
import { type MenuAnchor, placeMenu } from "./menuPlacement";

/** メニューの項目 1 つ。 */
export type MenuItem = {
  kind: "item";
  /** 並びの中で項目を見分ける値。表示に出さない。 */
  key: string;
  label: string;
  onSelect: () => void;
  /** 真のとき、項目を残したまま実行しない。 */
  disabled?: boolean;
};

/** 項目の間の区切り。 */
export type MenuSeparator = { kind: "separator"; key: string };

/** メニューに並べる項目と区切り。 */
export type MenuEntry = MenuItem | MenuSeparator;

/** 区切りを 1 つ作る。 */
export function menuSeparator(key: string): MenuSeparator {
  return { kind: "separator", key };
}

/**
 * 要素が focus を受けられる HTML の要素なら返す。別ウィンドウに出したビューの要素は、その
 * ウィンドウの HTMLElement で判定する。
 */
export function focusableElementOf(
  element: Element | null,
): HTMLElement | null {
  if (element === null) return null;
  const Html = element.ownerDocument.defaultView?.HTMLElement ?? HTMLElement;
  return element instanceof Html ? element : null;
}

type MenuProps = {
  /** 読み上げに渡すメニューの名前。labelledBy を与えたときは与えない。 */
  label?: string;
  /** メニューの名前を持つ要素の id。メニューバーの見出しを指す。 */
  labelledBy?: string;
  id?: string;
  entries: readonly MenuEntry[];
  anchor: MenuAnchor;
  /**
   * キーボードで閉じたときと、項目を実行したときに focus を戻す要素。メニューはこの要素の
   * document に描く。出ない場合は focus を戻さない。
   */
  returnFocus: HTMLElement | null;
  /** 開いたときに focus を置く項目。 */
  initialFocus?: "first" | "last";
  /** 閉じる。外側の操作、Escape、Tab、項目の実行で呼ぶ。2 回呼ぶことがある。 */
  onClose: () => void;
  /** 左右の矢印キーを受けたときに呼ぶ。メニューバーが隣のメニューへ移る。 */
  onMoveSideways?: (step: -1 | 1) => void;
  /**
   * 外側の操作とみなさない要素。メニューを開いた button やメニューバーを渡すと、その要素の
   * click がメニューを開き閉じする。
   */
  ignoreOutside?: Element | null;
};

/**
 * 項目を縦に並べたメニュー。WAI-ARIA の menu の型に合わせてキーボードで操作する。
 *
 * - 開くと最初の使える項目に focus を置く。
 * - 上下の矢印キーと Home と End で項目を移り、端では反対の端へ回る。文字を入れると、その文字で
 *   始まる次の項目へ移る。
 * - Enter と Space で項目を実行して閉じる。使えない項目は実行しない。
 * - Escape と Tab で閉じ、開いた元の要素へ focus を戻す。外側を押すと閉じる。
 *
 * **画面の端で切れない位置に出す。** 大きさを測ってから、画面の中に収まる位置へ動かす。
 * 要素は開いた元の要素の document の body に描き、ビューの枠の外まで出せるようにする。
 */
export function Menu({
  label,
  labelledBy,
  id,
  entries,
  anchor,
  returnFocus,
  initialFocus = "first",
  onClose,
  onMoveSideways,
  ignoreOutside = null,
}: MenuProps) {
  const rootRef = useRef<HTMLDivElement>(null);
  const itemRefs = useRef(new Map<string, HTMLButtonElement>());
  // 最初は左上に置いて大きさを測る。基準の位置に置くと、右端に近い基準では幅が縮んで測れる。
  const [placement, setPlacement] = useState({ left: 0, top: 0 });
  const ownerDocument = returnFocus?.ownerDocument ?? document;
  const items = entries.filter(
    (entry): entry is MenuItem => entry.kind === "item",
  );

  useLayoutEffect(() => {
    const root = rootRef.current;
    const view = root?.ownerDocument.defaultView;
    if (root === null || view === null || view === undefined) return;
    setPlacement(
      placeMenu(anchor, root.getBoundingClientRect(), {
        width: view.innerWidth,
        height: view.innerHeight,
      }),
    );
  }, [anchor]);

  // biome-ignore lint/correctness/useExhaustiveDependencies: 開いたときだけ focus を置く。項目が変わっても focus を動かさない。
  useEffect(() => {
    const usable = items.filter((item) => !item.disabled);
    const chosen =
      initialFocus === "last"
        ? (usable.at(-1) ?? items.at(-1))
        : (usable[0] ?? items[0]);
    const target =
      chosen === undefined ? rootRef.current : itemRefs.current.get(chosen.key);
    target?.focus({ preventScroll: true });
  }, []);

  useEffect(() => {
    const closeOnOutside = (event: Event) => {
      const path = event.composedPath();
      const root = rootRef.current;
      if (root !== null && path.includes(root)) return;
      if (ignoreOutside !== null && path.includes(ignoreOutside)) return;
      onClose();
    };
    ownerDocument.addEventListener("pointerdown", closeOnOutside, true);
    return () =>
      ownerDocument.removeEventListener("pointerdown", closeOnOutside, true);
  }, [ownerDocument, ignoreOutside, onClose]);

  const focusAt = (index: number) => {
    const item = items[index];
    if (item !== undefined) {
      itemRefs.current.get(item.key)?.focus({ preventScroll: true });
    }
  };
  const closeAndReturnFocus = () => {
    if (returnFocus?.isConnected) returnFocus.focus({ preventScroll: true });
    onClose();
  };
  const activate = (item: MenuItem) => {
    if (item.disabled) return;
    closeAndReturnFocus();
    item.onSelect();
  };

  const onKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    // 開いた元の要素の操作 (Shift+F10 で開く、メニューバーの左右の移動) へキーを渡さない。
    event.stopPropagation();
    const current = items.findIndex(
      (item) => itemRefs.current.get(item.key) === ownerDocument.activeElement,
    );
    switch (event.key) {
      case "ArrowDown":
      case "ArrowUp":
        event.preventDefault();
        if (items.length === 0) return;
        focusAt(
          current < 0
            ? event.key === "ArrowDown"
              ? 0
              : items.length - 1
            : wrapIndex(
                current,
                event.key === "ArrowDown" ? 1 : -1,
                items.length,
              ),
        );
        return;
      case "Home":
        event.preventDefault();
        focusAt(0);
        return;
      case "End":
        event.preventDefault();
        focusAt(items.length - 1);
        return;
      case "Enter":
      case " ": {
        event.preventDefault();
        const item = items[current];
        if (item !== undefined) activate(item);
        return;
      }
      case "Escape":
      case "Tab":
        event.preventDefault();
        closeAndReturnFocus();
        return;
      case "ArrowLeft":
      case "ArrowRight":
        if (onMoveSideways !== undefined) {
          event.preventDefault();
          onMoveSideways(event.key === "ArrowRight" ? 1 : -1);
        }
        return;
      default: {
        const character = typeaheadCharacter(event);
        if (character === undefined) return;
        const index = indexByInitial(
          items.map((item) => item.label),
          current,
          character,
        );
        if (index !== undefined) {
          event.preventDefault();
          focusAt(index);
        }
      }
    }
  };

  // **focus がメニューと ignoreOutside の外へ出たら閉じる。** F10 でメニューバーへ移ったときと、
  // 別のウィンドウへ移ったときに、開いたままのメニューを残さない。
  const onBlur = (event: FocusEvent<HTMLDivElement>) => {
    const next = event.relatedTarget;
    if (
      next !== null &&
      (rootRef.current?.contains(next) || ignoreOutside?.contains(next))
    ) {
      return;
    }
    onClose();
  };

  const focusUnderPointer = (event: PointerEvent<HTMLButtonElement>) => {
    if (ownerDocument.activeElement !== event.currentTarget) {
      event.currentTarget.focus({ preventScroll: true });
    }
  };

  return createPortal(
    <div
      ref={rootRef}
      id={id}
      role="menu"
      aria-label={labelledBy === undefined ? label : undefined}
      aria-labelledby={labelledBy}
      aria-orientation="vertical"
      tabIndex={-1}
      className="menu"
      style={{ left: placement.left, top: placement.top }}
      onKeyDown={onKeyDown}
      onBlur={onBlur}
      // 押しただけで focus を項目から外さない。項目の実行は click が受ける。
      onMouseDown={(event) => event.preventDefault()}
      onContextMenu={(event) => {
        event.preventDefault();
        event.stopPropagation();
      }}
    >
      {entries.map((entry) =>
        entry.kind === "separator" ? (
          <hr key={entry.key} className="menu-separator" />
        ) : (
          <button
            key={entry.key}
            ref={(element) => {
              if (element === null) itemRefs.current.delete(entry.key);
              else itemRefs.current.set(entry.key, element);
            }}
            type="button"
            role="menuitem"
            tabIndex={-1}
            aria-disabled={entry.disabled === true ? true : undefined}
            onClick={(event) => {
              event.stopPropagation();
              activate(entry);
            }}
            onPointerMove={focusUnderPointer}
          >
            {entry.label}
          </button>
        ),
      )}
    </div>,
    ownerDocument.body,
  );
}

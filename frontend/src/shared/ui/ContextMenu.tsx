import { Ellipsis } from "lucide-react";
import {
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
  useMemo,
  useRef,
  useState,
} from "react";
import { focusableElementOf, Menu, type MenuEntry } from "./Menu";
import { isContextMenuKey } from "./menuKeys";
import { anchorAtPoint, anchorBelow, type MenuAnchor } from "./menuPlacement";

/** コンテキストメニューに出す名前と項目。 */
export type ContextMenuContent = {
  /** 読み上げに渡すメニューの名前。何の操作かを示す。 */
  label: string;
  entries: readonly MenuEntry[];
};

/** コンテキストメニューを開く操作。描画をまたいで同じ関数を保つ。 */
export type ContextMenuTriggers<T> = {
  /** 右クリックした点に開く。ブラウザーのメニューを出さない。 */
  onContextMenu: (event: MouseEvent<Element>, target: T) => void;
  /** Shift+F10 と ContextMenu キーで、focus のある要素の下に開く。他のキーでは何もしない。 */
  onKeyDown: (event: KeyboardEvent<Element>, target: T) => void;
  /** button の下に開く。同じ button から開いているときは閉じる。 */
  onButtonClick: (event: MouseEvent<HTMLElement>, target: T) => void;
  /**
   * 基準の位置に開く。図のように React の event を持たない呼び出し元が使う。returnFocus は
   * キーボードで閉じたときに focus を戻す要素である。
   */
  openAt: (
    target: T,
    anchor: MenuAnchor,
    returnFocus: HTMLElement | null,
  ) => void;
};

type OpenMenu<T> = {
  /** 開き直すたびに増やす値。メニューを作り直し、開いたときの focus を置き直す。 */
  serial: number;
  target: T;
  anchor: MenuAnchor;
  returnFocus: HTMLElement | null;
  /** メニューを開いた button。押し直すと閉じる。 */
  button: HTMLElement | null;
};

/**
 * 対象ごとにコンテキストメニューを開く。1 つの一覧が 1 つ持ち、行は triggers を受け取る。
 *
 * **項目は開いている間の描画ごとに build で組む。** 開いている間に状態が変わっても、使えるか
 * どうかを今の状態で出す。
 *
 * 返す openTarget は開いているメニューの対象である。「…」の button の aria-expanded に使う。
 */
export function useContextMenu<T>(build: (target: T) => ContextMenuContent): {
  triggers: ContextMenuTriggers<T>;
  openTarget: T | undefined;
  menu: ReactNode;
} {
  const [open, setOpen] = useState<OpenMenu<T> | undefined>(undefined);
  const openRef = useRef(open);
  openRef.current = open;
  const serialRef = useRef(0);

  const triggers = useMemo<ContextMenuTriggers<T>>(() => {
    const show = (
      target: T,
      anchor: MenuAnchor,
      returnFocus: HTMLElement | null,
      button: HTMLElement | null,
    ) => {
      serialRef.current += 1;
      setOpen({
        serial: serialRef.current,
        target,
        anchor,
        returnFocus,
        button,
      });
    };
    return {
      onContextMenu: (event, target) => {
        event.preventDefault();
        // キーボードで開いた直後に、ブラウザーが同じキーから contextmenu を送ることがある。
        // 開いているメニューを右クリックの位置へ置き換えない。
        if (
          openRef.current !== undefined &&
          openRef.current.target === target
        ) {
          return;
        }
        show(
          target,
          anchorAtPoint(event.clientX, event.clientY),
          focusableElementOf(event.currentTarget.ownerDocument.activeElement),
          null,
        );
      },
      onKeyDown: (event, target) => {
        if (!isContextMenuKey(event)) return;
        event.preventDefault();
        event.stopPropagation();
        const focused = focusableElementOf(
          event.currentTarget.ownerDocument.activeElement,
        );
        show(
          target,
          anchorBelow(focused ?? event.currentTarget),
          focused,
          null,
        );
      },
      onButtonClick: (event, target) => {
        const button = event.currentTarget;
        if (openRef.current?.button === button) {
          setOpen(undefined);
          return;
        }
        show(target, anchorBelow(button), button, button);
      },
      openAt: (target, anchor, returnFocus) =>
        show(target, anchor, returnFocus, null),
    };
  }, []);

  const content = open === undefined ? undefined : build(open.target);
  const menu =
    open === undefined || content === undefined ? null : (
      <Menu
        key={open.serial}
        label={content.label}
        entries={content.entries}
        anchor={open.anchor}
        returnFocus={open.returnFocus}
        ignoreOutside={open.button}
        // 閉じた後に開き直したメニューを、前のメニューの閉じる通知で閉じない。
        onClose={() =>
          setOpen((current) =>
            current?.serial === open.serial ? undefined : current,
          )
        }
      />
    );
  return { triggers, openTarget: open?.target, menu };
}

/**
 * 行の操作のメニューを開く「…」の button。右クリックと Shift+F10 と同じメニューを開く。
 * 読み上げでは、どの行の操作かを label で示す。
 */
export function RowMenuButton({
  label,
  expanded,
  onClick,
}: {
  label: string;
  /** この button から開いたメニューが開いているか。 */
  expanded: boolean;
  onClick: (event: MouseEvent<HTMLButtonElement>) => void;
}) {
  return (
    <button
      type="button"
      className="row-menu-button"
      aria-label={label}
      aria-haspopup="menu"
      aria-expanded={expanded}
      title={label}
      onClick={onClick}
    >
      <Ellipsis size={14} aria-hidden="true" />
    </button>
  );
}

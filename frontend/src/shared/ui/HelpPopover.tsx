import { CircleHelp } from "lucide-react";
import { type CSSProperties, type ReactNode, useId, useState } from "react";
import { createPortal } from "react-dom";

/** 吹き出しの最大の幅 (px)。layout.css の `.help-popover-content` と揃える。 */
const contentMaxWidth = 360;

/**
 * 吹き出しを「?」のボタンの下に置く位置。ボタンが画面の下の側にあるときは上に置く。
 * 区画の scroll に切られないよう、画面に対する位置で置く。
 */
function placementOf(anchor: Anchor): CSSProperties {
  const { rect, view } = anchor;
  const left = Math.max(
    8,
    Math.min(rect.left, view.innerWidth - contentMaxWidth - 8),
  );
  return rect.bottom > view.innerHeight * 0.6
    ? { left, bottom: view.innerHeight - rect.top + 4 }
    : { left, top: rect.bottom + 4 };
}

/**
 * 開いた時点のボタンの位置と、ボタンを持つ文書。区画を別ウィンドウに出したときは、
 * そのウィンドウの文書に吹き出しを置く。
 */
type Anchor = { rect: DOMRect; view: Window; body: HTMLElement };

function anchorOf(trigger: HTMLElement): Anchor {
  const document = trigger.ownerDocument;
  return {
    rect: trigger.getBoundingClientRect(),
    view: document.defaultView ?? window,
    body: document.body,
  };
}

/**
 * 要素の右に置く「?」のボタンと、その説明の吹き出し。
 *
 * ポインタを重ねるか focus すると開き、離れると閉じる。クリック・Enter・Space で開いたままに
 * し、Escape で閉じる。タッチの端末と keyboard の利用者もクリックと Enter で読める。
 * 閉じている間は吹き出しを描かない。
 */
export function HelpPopover({
  label,
  children,
}: {
  /** ボタンの accessible name に使う、説明する対象の名前。 */
  label: string;
  children: ReactNode;
}) {
  const contentId = useId();
  const [hovered, setHovered] = useState(false);
  const [pinned, setPinned] = useState(false);
  // 既知の制限: 開いた時点の位置で置く。開いたまま区画を scroll すると吹き出しはボタンから離れる。
  const [anchor, setAnchor] = useState<Anchor>();
  const open = hovered || pinned;
  const close = () => {
    setHovered(false);
    setPinned(false);
  };
  const hover = (trigger: HTMLElement) => {
    setAnchor(anchorOf(trigger));
    setHovered(true);
  };
  return (
    <span className="help-popover">
      <button
        type="button"
        className="help-popover-trigger"
        aria-label={`${label} の説明`}
        aria-expanded={open}
        aria-controls={open ? contentId : undefined}
        aria-describedby={open ? contentId : undefined}
        onMouseEnter={(event) => hover(event.currentTarget)}
        onMouseLeave={() => setHovered(false)}
        onKeyDown={(event) => {
          if (event.key === "Escape" && open) {
            event.stopPropagation();
            close();
          }
        }}
        onFocus={(event) => hover(event.currentTarget)}
        onBlur={close}
        onClick={(event) => {
          if (pinned) {
            close();
            return;
          }
          setAnchor(anchorOf(event.currentTarget));
          setPinned(true);
        }}
      >
        <CircleHelp size={14} aria-hidden="true" />
      </button>
      {open &&
        anchor !== undefined &&
        createPortal(
          <span
            id={contentId}
            role="tooltip"
            className="help-popover-content"
            style={placementOf(anchor)}
          >
            {children}
          </span>,
          anchor.body,
        )}
    </span>
  );
}

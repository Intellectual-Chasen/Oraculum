/** メニューを出す基準の位置。値は viewport の座標 (px) である。 */
export type MenuAnchor = {
  /** メニューの左端を合わせる x。 */
  x: number;
  /** 基準の上端。下に収まらないとき、メニューの下端をここに合わせる。 */
  top: number;
  /** 基準の下端。メニューの上端をここに合わせる。 */
  bottom: number;
};

/** 幅と高さ (px)。 */
export type BoxSize = { width: number; height: number };

/** 画面の端とメニューの間に空ける幅 (px)。 */
export const menuViewportMargin = 4;

/** 右クリックした点を基準にする。 */
export function anchorAtPoint(x: number, y: number): MenuAnchor {
  return { x, top: y, bottom: y };
}

/** 要素の左下を基準にする。キーボードと button から開くメニューが使う。 */
export function anchorBelow(element: Element): MenuAnchor {
  const rect = element.getBoundingClientRect();
  return { x: rect.left, top: rect.top, bottom: rect.bottom };
}

/**
 * メニューの左上の位置を、画面の中に収まるように決める。
 *
 * 縦は基準の下に収まれば下に出し、収まらず上に収まれば上に出す。どちらにも収まらなければ
 * 画面の下端に寄せる。横は基準の x から右へ出し、右端を越えるときは左へ寄せる。どちらの向きも
 * 画面の上端と左端より外へ出さない。
 */
export function placeMenu(
  anchor: MenuAnchor,
  size: BoxSize,
  viewport: BoxSize,
  margin: number = menuViewportMargin,
): { left: number; top: number } {
  const left = Math.max(
    margin,
    Math.min(anchor.x, viewport.width - margin - size.width),
  );
  const fitsBelow = anchor.bottom + size.height <= viewport.height - margin;
  const fitsAbove = anchor.top - size.height >= margin;
  const top = fitsBelow
    ? anchor.bottom
    : fitsAbove
      ? anchor.top - size.height
      : viewport.height - margin - size.height;
  return { left, top: Math.max(margin, top) };
}

import { Hint } from "./Hint";

/**
 * 値が無いセルを表示する。値の位置には短い印「—」だけを出し、`description` のラベルは
 * `Hint` の読み上げ (sr-only) とマウスを重ねたときの tooltip に渡す。
 *
 * `description` に、そのセルで何が無いかを短いラベル (「フィールドなし」「値なし」) で書く。
 * 「フィールドそのものが無い」と「値が無い」を別のラベルで渡し、1 つにまとめない。
 *
 * 押せる要素の中に置くときは `plain` を渡し、ラベルをそのまま出す。押せる要素の中に focus を
 * 受ける要素を入れない。
 */
export function MissingValue({
  description,
  plain = false,
}: {
  description: string;
  /** 押せる要素の中に置くとき、印を作らずにラベルをそのまま出す。 */
  plain?: boolean;
}) {
  if (plain) {
    return <span>{description}</span>;
  }
  return (
    <Hint text={description} className="text-faint" mark>
      <span aria-hidden="true">—</span>
    </Hint>
  );
}

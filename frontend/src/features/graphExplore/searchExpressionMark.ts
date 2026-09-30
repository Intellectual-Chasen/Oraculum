/** 検索式の文字列を、誤りの範囲の前・範囲・後の 3 つに分けた組。3 つをつなぐと元の文字列になる。 */
export type MarkedSearchExpression = {
  before: string;
  /**
   * 誤りの範囲の文字列。長さ 0 の誤りでは空の文字列であり、`before` と `after` の境が
   * 誤りの位置である。
   */
  marked: string;
  after: string;
  /**
   * 誤りの位置。範囲は「N–M 文字目」、1 文字は「N 文字目」、長さ 0 は「N 文字目の前」、式の
   * 終わりの長さ 0 は「式の末尾」と書く。N は 1 から数える。
   */
  position: string;
};

/** 誤りの範囲を位置の値にする。値は応答の範囲のまま書く。 */
function positionOf(
  error: { offset: number; length: number },
  characterCount: number,
): string {
  const first = error.offset + 1;
  if (error.length === 0) {
    return error.offset >= characterCount ? "式の末尾" : `${first} 文字目の前`;
  }
  const last = error.offset + error.length;
  return last === first ? `${first} 文字目` : `${first}–${last} 文字目`;
}

/**
 * 検索式の文字列を、誤りの範囲で 3 つに分ける。
 * 範囲は Unicode の code point で数え、UTF-16 の surrogate pair を 1 文字として扱う。
 *
 * **範囲を式の中へ寄せる。** 応答は送った式の上で範囲を数えるため、範囲は式の中にある。
 * 式の終わりを越える範囲は、式の終わりで切って描く。
 */
export function markSearchExpression(
  text: string,
  error: { offset: number; length: number },
): MarkedSearchExpression {
  const characters = Array.from(text);
  const start = Math.min(error.offset, characters.length);
  const end = Math.min(start + error.length, characters.length);
  return {
    before: characters.slice(0, start).join(""),
    marked: characters.slice(start, end).join(""),
    after: characters.slice(end).join(""),
    position: positionOf(error, characters.length),
  };
}

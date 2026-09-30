/** Unicode の Cc (制御) と Cf (書式。bidi 制御と zero width を含む) の 1 文字。 */
const invisibleCharacter = /[\p{Cc}\p{Cf}]/u;

function toVisibleCode(codePoint: number): string {
  return `U+${codePoint.toString(16).toUpperCase().padStart(4, "0")}`;
}

/** 原文の文字をそのまま持つ部分 (`text`) か、1 つの code point を置き換えた `U+XXXX` の符号 (`code`)。 */
export type RawTextPart = { kind: "text" | "code"; text: string };

/**
 * 原資料由来の文字列を、原文のままの部分と、Cc と Cf の code point を置き換えた符号の部分に区切る。
 * 原文のままの連続した文字を 1 つの部分にまとめ、置き換えた code point は 1 つずつ別の部分にする。
 * 部分の `text` をつなぐと `toVisibleRawText` の結果になる。
 */
export function splitRawText(text: string): RawTextPart[] {
  const parts: RawTextPart[] = [];
  let plain = "";
  for (const character of text) {
    const codePoint = character.codePointAt(0);
    if (codePoint === undefined || !invisibleCharacter.test(character)) {
      plain += character;
      continue;
    }
    if (plain !== "") {
      parts.push({ kind: "text", text: plain });
      plain = "";
    }
    parts.push({ kind: "code", text: toVisibleCode(codePoint) });
  }
  if (plain !== "") {
    parts.push({ kind: "text", text: plain });
  }
  return parts;
}

/**
 * 原資料由来の文字列を、画面に描く形へ変換する。
 * Cc と Cf の code point を `U+XXXX` の可視の符号に置き換え、他の文字をそのまま残す。
 * 文字を 1 つも除かないため、描いた文字列は原文の全 code point を表す。
 * 呼び出し元は描画の直前に使い、state と props が持つ値を原文のまま保つ。
 */
export function toVisibleRawText(text: string): string {
  return splitRawText(text)
    .map((part) => part.text)
    .join("");
}

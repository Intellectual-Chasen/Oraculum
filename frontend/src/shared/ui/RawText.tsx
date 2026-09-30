import { Fragment } from "react";
import { splitRawText } from "../lib/rawText";

/** 描画で詰められうる空白。連続した空白と、部分の先頭か末尾の空白である。 */
const collapsibleSpace = / {2}|^ | $/;

/**
 * 原資料由来の文字列を描く。
 * Unicode の Cc (制御) と Cf (書式。bidi 制御と zero width を含む) の code point を
 * `U+XXXX` の可視の符号にし、枠の付いた `raw-control-code` の span で出す。
 * 他の文字はテキストノードで出すため、原文が持つ `U+XXXX` の文字列と置き換えた符号を見分けられる。
 * LF は符号の後で改行し、複数行の原文を行ごとに読めるようにする。
 * 置き換えは描画の処理だけで行い、state と props と応答の型が持つ値を原文のまま保つ。
 *
 * **空白を詰めない。** 空白が 2 つ以上続く部分と、先頭か末尾に空白を持つ部分は `raw-text` の
 * 枠に入れ、空白を原文のまま描く。`net  use` と `net use` を見分けられる。tab と改行は符号に
 * 置き換えるため、部分の中には残らない。
 */
export function RawText({ text }: { text: string }) {
  return (
    <>
      {splitRawText(text).map((part, index) => {
        if (part.kind === "text") {
          return collapsibleSpace.test(part.text) ? (
            // biome-ignore lint/suspicious/noArrayIndexKey: 部分の並びは text から一意に決まる。
            <span key={index} className="raw-text">
              {part.text}
            </span>
          ) : (
            part.text
          );
        }
        const label = `制御文字 ${part.text}`;
        return (
          // biome-ignore lint/suspicious/noArrayIndexKey: 部分の並びは text から一意に決まる。
          <Fragment key={index}>
            <span
              className="raw-control-code"
              role="img"
              aria-label={label}
              title={label}
            >
              {part.text}
            </span>
            {/* LF の符号を残したまま、原文の行の区切りで改行する。 */}
            {part.text === "U+000A" && <br />}
          </Fragment>
        );
      })}
    </>
  );
}

import { Info } from "lucide-react";
import { Hint } from "./Hint";

/**
 * 取り込みが英語で書いた導き方の文字列と、画面に出す日本語の導き方の対応。定義元は
 * `backend/adapters` の各入力形式の導き方の定数である。表に無い文字列は受け取ったまま出す。
 */
const derivationTexts: Readonly<Record<string, string>> = {
  "IPv4-mapped IPv6 address written as dotted decimal IPv4":
    "IPv6 の形で書いた IPv4 のアドレスをドット 10 進の IPv4 に直した値",
  "address without the port": "port を付けたアドレスから port を外した値",
  "hexadecimal process id with the 0x prefix written in decimal":
    "0x を付けた 16 進のプロセス番号を 10 進に直した値",
  "decimal digits written without leading zeros":
    "10 進の文字列から先頭の 0 を外した値",
  "task name without the NT TASK prefix":
    "タスクの名前から NT TASK の接頭辞を外した値",
  "user principal name without the realm":
    "ユーザープリンシパル名から realm を外した値",
  "common name of the first relative distinguished name": "DN の先頭の CN",
};

/** Oraculum が作った表示名であることと、作り方の「名前: 値」の組。 */
export function derivedLabelText(derivation: string | undefined): string {
  return derivation === undefined
    ? "Oraculum が作った表示名"
    : `Oraculum が作った表示名\n作り方: ${derivationTexts[derivation] ?? derivation}`;
}

/**
 * 導いた値であることを、表示名の横の小さな印 (Info の icon) で知らせ、導き方の文を読み上げの
 * 文 (sr-only) と、マウスを重ねたときの tooltip に渡す。原文に書かれた
 * 文字列と、Oraculum が組み立てた表示名を読み分けられるようにする。表の列が長い文で折り返し、
 * 値を読めなくなることを避ける。
 *
 * 表の外の印は focus を受けるため、押せる要素の中には置かない。
 *
 * 値がある列の印であり、値が無い列を表す `MissingValue` と別の component にする。
 */
export function DerivedLabelNote({
  derivation,
}: {
  derivation: string | undefined;
}) {
  const text = derivedLabelText(derivation);
  return (
    <Hint
      text={text}
      className="derived-note inline-flex items-center px-0.5 align-middle text-muted"
      mark
    >
      <Info className="size-3.5" aria-hidden="true" />
    </Hint>
  );
}

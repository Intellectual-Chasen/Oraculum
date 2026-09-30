import { toVisibleRawText } from "@/shared/lib/rawText";
import { DerivedLabelNote } from "@/shared/ui/DerivedLabelNote";
import { Highlighted } from "@/shared/ui/Highlighted";
import { MissingValue } from "@/shared/ui/MissingValue";
import type { NodeLabel } from "./subgraph";

/**
 * ノードの表示名の値だけを出す。
 * 値を持たない表示名は、値が無い理由を文のまま出す。
 *
 * **button の名前にはこちらを使う。** 導き方の印を button の中に置くと、押せる要素が入れ子に
 * なり、読み上げも button を辿るたびに同じ一文を読む。
 */
export function NodeLabelValue({ label }: { label: NodeLabel }) {
  return "text" in label.value ? (
    <Highlighted text={label.value.text} />
  ) : (
    <MissingValue description={label.value.absence} plain />
  );
}

/**
 * ノードの表示名の値を、`NodeLabelValue` が描くのと同じ文字列で返す。button の名前に使う。
 */
export function nodeLabelName(label: NodeLabel): string {
  return "text" in label.value
    ? toVisibleRawText(label.value.text)
    : `（${label.value.absence}）`;
}

/**
 * ノードの表示名を出す。
 * 原資料の文字列をそのまま出し、導いた値には導いたことと導き方を添える。
 */
export function NodeLabelView({ label }: { label: NodeLabel }) {
  if (!("text" in label.value)) {
    return <MissingValue description={label.value.absence} />;
  }
  return (
    <>
      <NodeLabelValue label={label} />
      {label.valueState === "derived" ? (
        <>
          {" "}
          <DerivedLabelNote derivation={label.derivation} />
        </>
      ) : null}
    </>
  );
}

/**
 * ノードの表示名を、図に描く 1 つの文字列にする。
 * 原資料由来の文字列を可視の符号へ置き換えてから返す。
 *
 * **path の形の表示名は、区切りの後の最後の要素だけを描く。** 図の上の長い path は隣の
 * ラベルと重なり、図の端で切れる。全体はノードの一覧と詳細が原資料の文字列のまま出す。
 *
 * **導いた値であることは図に描かない。** 図のラベルを短く保つ。導いたことと導き方は
 * ノードの一覧と詳細が出す。
 */
export function drawnNodeLabel(label: NodeLabel): string {
  if (!("text" in label.value)) {
    return `（${label.value.absence}）`;
  }
  return toVisibleRawText(lastPathElement(label.value.text));
}

/**
 * `\` または `/` で区切った文字列の最後の要素を返す。区切りを持たない文字列はそのまま返す。
 * 両端の引用符は外す。
 */
export function lastPathElement(text: string): string {
  const unquoted = /^"(.*)"$/s.exec(text)?.[1] ?? text;
  const cut = Math.max(unquoted.lastIndexOf("\\"), unquoted.lastIndexOf("/"));
  if (cut < 0 || cut === unquoted.length - 1) {
    return text;
  }
  return unquoted.slice(cut + 1);
}

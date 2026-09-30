import type { ReactNode } from "react";
import type { SearchExpressionFailure } from "@/shared/lib/fetchState";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import { markSearchExpression } from "./searchExpressionMark";

/**
 * 適用している検索式の誤りを、理由と位置と、範囲に印を付けた式で出す。
 * 式は分析者の入力であり、制御文字を可視の符号にして描く (`RawText`)。
 */
export function SearchExpressionErrorView({
  expression,
  error,
}: {
  expression: string;
  error: SearchExpressionFailure;
}) {
  const marked = markSearchExpression(expression, error);
  return (
    <section aria-label="検索式の誤り" className="search-expression-error">
      <KeyValueList
        stacked
        pairs={[
          { name: "検索式の誤り", value: error.description },
          { name: "誤りの位置", value: marked.position },
        ]}
      />
      <p className="search-expression-echo">
        <RawText text={marked.before} />
        {marked.marked === "" ? (
          <mark className="search-expression-point">
            <span className="visually-hidden">誤りの位置</span>
          </mark>
        ) : (
          <mark>
            <RawText text={marked.marked} />
          </mark>
        )}
        <RawText text={marked.after} />
      </p>
    </section>
  );
}

/** 検索式の書き方の組。値の中の記法は等幅で描く。 */
const expressionSyntax: { name: string; value: ReactNode }[] = [
  { name: "比較", value: <code>フィールド 演算子 値</code> },
  {
    name: "演算子",
    value: <code>{"== != > >= < <= contains"}</code>,
  },
  { name: "フィールドの無い値", value: "どれかのフィールドに含まれる値" },
  {
    name: "組み合わせ",
    value: <code>{"and && · or || · not ! · ( )"}</code>,
  },
  { name: "優先の順", value: <code>not · and · or</code> },
  { name: "空白で並べた条件", value: <code>and</code> },
  {
    name: '「"」で囲む値',
    value: "空白・括弧・引用符・= ! < > & | を含む値と and・or・not",
  },
  { name: '囲んだ中の「"」', value: <code>{'\\"'}</code> },
  { name: "囲んだ中の「\\」", value: <code>{"\\\\"}</code> },
  { name: "判定の単位", value: "1 件のレコードのフィールド" },
  {
    name: "大文字と小文字の区別",
    value: (
      <>
        なし: <code>==</code> と <code>contains</code>
      </>
    ),
  },
  {
    name: "!= に一致するレコード",
    value: "フィールドを持ち等しい値の無いレコード",
  },
  {
    name: "フィールドの無いレコードを含める式",
    value: <code>not フィールド == 値</code>,
  },
  {
    name: "> >= < <= の値",
    value: "10 進の数と UTC オフセットを持つ RFC 3339 の時刻",
  },
  {
    name: "例",
    value: (
      <code>
        TargetUserName contains admin and not (LogonType == 3 or LogonType ==
        10)
      </code>
    ),
  },
];

/** 検索式の書き方の名前と値の組。条件の入力の説明の中に出す。 */
export function SearchExpressionSyntax() {
  return <KeyValueList stacked pairs={expressionSyntax} />;
}

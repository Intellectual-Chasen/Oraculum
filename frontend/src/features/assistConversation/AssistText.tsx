import { FileCode2, FileText } from "lucide-react";
import { Fragment, useState } from "react";
import Markdown, { type Components } from "react-markdown";
import remarkGfm from "remark-gfm";
import { splitRawText } from "@/shared/lib/rawText";
import { IconButton } from "@/shared/ui/IconButton";
import { RawText } from "@/shared/ui/RawText";

/**
 * 会話の文字列を描く。改行で行を分け、行の中の制御文字と書式文字を可視の符号にする。
 * **文字列としてだけ描く。** 画像を読み込まず、URL をリンクにしない。LLM の応答は証拠の
 * 文字列を引用しうるため、bidi 制御の文字で表示の並びを変えさせない。
 */
export function AssistText({ text }: { text: string }) {
  const lines = text.split("\n");
  return (
    <p className="assist-text">
      {lines.map((line, index) => (
        // biome-ignore lint/suspicious/noArrayIndexKey: 行の並びは text から一意に決まる。
        <Fragment key={index}>
          {index === 0 ? null : "\n"}
          <RawText text={line.endsWith("\r") ? line.slice(0, -1) : line} />
        </Fragment>
      ))}
    </p>
  );
}

/** mdast と hast のノードのうち、plugin が読み書きする項目。 */
type TreeNode = {
  type: string;
  value?: string;
  tagName?: string;
  properties?: Record<string, unknown>;
  children?: TreeNode[];
};

function visit(node: TreeNode, action: (node: TreeNode) => void) {
  action(node);
  for (const child of node.children ?? []) visit(child, action);
}

/** mdast の HTML のノードを文字列のノードにする。HTML に見える文字列も原文のまま描く。 */
function htmlAsText() {
  return (tree: TreeNode) =>
    visit(tree, (node) => {
      if (node.type === "html") node.type = "text";
    });
}

/** hast の文字列を、制御文字と書式文字を `raw-control-code` の符号にしたノードの並びにする。 */
function visibleCodes(value: string): TreeNode[] {
  // LF と TAB は Markdown の区切りとコードブロックの字下げのため、符号にしない。
  return value.split(/([\n\t])/).flatMap((chunk) =>
    /^[\n\t]$/.test(chunk)
      ? [{ type: "text", value: chunk }]
      : splitRawText(chunk).map((part) =>
          part.kind === "text"
            ? { type: "text", value: part.text }
            : {
                type: "element",
                tagName: "span",
                properties: {
                  className: ["raw-control-code"],
                  role: "img",
                  ariaLabel: `制御文字 ${part.text}`,
                  title: `制御文字 ${part.text}`,
                },
                children: [{ type: "text", value: part.text }],
              },
        ),
  );
}

/** hast の文字列の制御文字と書式文字を可視の符号にする。 */
function controlCodes() {
  return (tree: TreeNode) =>
    visit(tree, (node) => {
      if (node.children === undefined) return;
      node.children = node.children.flatMap((child) =>
        child.type === "text" ? visibleCodes(child.value ?? "") : [child],
      );
    });
}

/** リンクを文字列にし、画像を読み込まずに代わりの文字列を出す。 */
const components: Components = {
  a: ({ children }) => <span>{children}</span>,
  img: ({ alt }) => <span>{alt ? <RawText text={alt} /> : null}</span>,
};

/**
 * AI の応答の文を Markdown として描く。分析者は文ごとに原文の表示へ切り替えられる。
 * **HTML を描かず、画像を読み込まず、URL をリンクにしない。** 制御文字と書式文字は
 * `RawText` と同じ符号にし、bidi 制御の文字で表示の並びを変えさせない。
 */
export function AssistMarkdown({ text }: { text: string }) {
  const [raw, setRaw] = useState(false);
  return (
    <div className="assist-markdown-frame">
      <IconButton
        label={raw ? "Markdown で表示" : "原文で表示"}
        className="assist-markdown-toggle"
        onPress={() => setRaw((current) => !current)}
      >
        {raw ? (
          <FileCode2 size={14} aria-hidden="true" />
        ) : (
          <FileText size={14} aria-hidden="true" />
        )}
      </IconButton>
      {raw ? (
        <AssistText text={text} />
      ) : (
        <div className="assist-markdown">
          <Markdown
            remarkPlugins={[remarkGfm, htmlAsText]}
            rehypePlugins={[controlCodes]}
            components={components}
          >
            {text}
          </Markdown>
        </div>
      )}
    </div>
  );
}

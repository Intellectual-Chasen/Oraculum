import type { ReactNode } from "react";
import { cn } from "./cn";

/** 「名前: 値」の組 1 つ。`value` が undefined の組は描かない。 */
export type KeyValuePair = {
  name: string;
  value: ReactNode;
  /** 同じ `name` の組を並べるときに区別する key。省略すると `name` を使う。 */
  key?: string;
};

/**
 * 「名前: 値」の組を並べる。既定は横に並べ、`stacked` を渡すと 1 行に 1 組を置き、値を
 * 折り返す。tooltip の中の詳細には `stacked` を使う。`inline` を渡すと、`<summary>` の中に
 * 置けるよう、一覧の要素を使わずに `<span>` だけで描く。
 */
export function KeyValueList({
  pairs,
  stacked = false,
  inline = false,
  className,
}: {
  pairs: KeyValuePair[];
  stacked?: boolean;
  inline?: boolean;
  className?: string;
}) {
  const shown = pairs.filter((pair) => pair.value !== undefined);
  if (shown.length === 0) {
    return null;
  }
  const List = inline ? "span" : "ul";
  const Item = inline ? "span" : "li";
  return (
    <List
      className={cn(
        "value-pairs",
        stacked &&
          "flex-col items-start [&>li]:whitespace-normal [&>li]:[overflow-wrap:anywhere]",
        inline && "inline-flex [&>span]:whitespace-nowrap",
        className,
      )}
    >
      {shown.map((pair) => (
        <Item key={pair.key ?? pair.name}>
          <span className="pair-name">{pair.name}:</span> {pair.value}
        </Item>
      ))}
    </List>
  );
}

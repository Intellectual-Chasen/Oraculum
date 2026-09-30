import type { SubgraphNode } from "@/shared/contracts/graph";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { nodeKindLabelOf } from "./labels";

/** ノードの表示名、同一性の項目と値、種類を一覧の文字列検索へ渡す。 */
export function nodeSearchValues(node: SubgraphNode): string[] {
  return [
    node.label.rawText ?? "",
    node.label.normalized ?? "",
    node.kind,
    nodeKindLabelOf(node),
    ...node.identity.flatMap((item) => [item.semantic ?? "", item.value]),
  ];
}

/** 表示する文字列のどれかが、大文字と小文字を区別せず検索文字列を含むかを返す。 */
export function includesListText(
  values: readonly string[],
  text: string,
): boolean {
  const query = text.toLowerCase();
  return values.some((value) =>
    toVisibleRawText(value).toLowerCase().includes(query),
  );
}

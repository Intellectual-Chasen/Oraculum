import type { GraphNode } from "../contracts/graph";

/** 端末のノードの表示名。名前を記録していない端末は id で出す。 */
export function terminalLabel(node: GraphNode): string {
  return node.label.rawText ?? node.label.normalized ?? node.id;
}

/**
 * 端末のノードの識別子から、画面に出す名前を探す表を組む。
 *
 * **同じ表示名の端末が 2 つ以上あるときは、端末を記録した収集元の表示名を足す。**
 * 収集元ごとに組んだ端末は、ホスト名が同じでも別のノードである。収集元の表示名は
 * `fileNamesByContent` (内容の sha256 から表示名) で探し、探せない収集元は足さない。
 */
export function distinctTerminalNames(
  nodes: readonly GraphNode[],
  fileNamesByContent: ReadonlyMap<string, string>,
): Map<string, string> {
  // 同じ端末が一覧に 2 回現れても 1 台と数える。
  const idsByLabel = new Map<string, Set<string>>();
  for (const node of nodes) {
    const label = terminalLabel(node);
    idsByLabel.set(label, (idsByLabel.get(label) ?? new Set()).add(node.id));
  }
  const counts = new Map(
    [...idsByLabel].map(([label, ids]) => [label, ids.size]),
  );
  return new Map(
    nodes.map((node) => {
      const label = terminalLabel(node);
      // 収集元ごとに組んだ端末の鍵は、先頭に収集元の内容の sha256 を持つ。
      const content = node.keyForm.startsWith("recording_source")
        ? node.identity[0]?.value
        : undefined;
      const source =
        content === undefined ? undefined : fileNamesByContent.get(content);
      return [
        node.id,
        (counts.get(label) ?? 0) > 1 && source !== undefined
          ? `${label}、収集元: ${source}`
          : label,
      ];
    }),
  );
}

/** 端末を識別した単位。文字列か、文字列を出さない理由のどちらかを持つ。 */
export type TerminalSource =
  | { text: string; title?: string }
  | { absence: string };

/**
 * 端末を識別した単位を、識別鍵の形ごとに書く。収集元で識別した端末は収集元の file 名を返し、
 * 端末の識別子で識別した端末は出さない理由を返す。
 */
export function terminalSourceOf(
  node: GraphNode,
  sourceFileNames: ReadonlyMap<string, string>,
): TerminalSource {
  const first = node.identity[0]?.value;
  switch (node.keyForm) {
    case "recording_source_content_sha256":
    case "recording_source_content_sha256_hostname":
      // 識別鍵の先頭の値が、端末を作った収集元の内容の識別である。
      return first === undefined
        ? { absence: "収集元なし" }
        : { text: sourceFileNames.get(first) ?? first, title: first };
    case "collection_content_sha256":
      // 識別鍵の値は、収集の directory の file の内容から求めた値である。
      return first === undefined
        ? { absence: "収集の単位なし" }
        : {
            text: `収集の directory: ${first.slice(0, 12)}…`,
            title: first,
          };
    case "terminal_id":
      return { absence: "端末 ID で識別" };
    default:
      return { absence: "収集元で識別していない端末" };
  }
}

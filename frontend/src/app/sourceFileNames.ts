/**
 * 収集元の内容の sha256 から表示名を探す表を組む。
 *
 * **同じ内容を 2 件以上の収集元として取り込んだときは、表示名を取り込んだ順に「、」でつなぐ。**
 * 内容の sha256 で組むノード (収集元ごとの端末など) は、同じ内容の収集元のすべてを表す。
 */
export function fileNamesByContentOf(
  sources: readonly { contentSha256: string; fileName: string }[],
): ReadonlyMap<string, string> {
  const names = new Map<string, string[]>();
  for (const source of sources) {
    const known = names.get(source.contentSha256);
    if (known === undefined) {
      names.set(source.contentSha256, [source.fileName]);
    } else if (!known.includes(source.fileName)) {
      known.push(source.fileName);
    }
  }
  return new Map(
    [...names].map(([content, fileNames]) => [content, fileNames.join("、")]),
  );
}

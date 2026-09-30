/** キーの入力のうち、メニューの操作が読む項目。React の event と DOM の event の両方が持つ。 */
export type MenuKeyInput = {
  key: string;
  shiftKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  metaKey: boolean;
};

/** コンテキストメニューを開くキー (Shift+F10 と ContextMenu キー) か。 */
export function isContextMenuKey(input: MenuKeyInput): boolean {
  if (input.key === "ContextMenu") return true;
  return (
    input.key === "F10" &&
    input.shiftKey &&
    !input.ctrlKey &&
    !input.altKey &&
    !input.metaKey
  );
}

/** メニューバーへ focus を移すキー (修飾キーを伴わない F10) か。 */
export function isMenuBarKey(input: MenuKeyInput): boolean {
  return (
    input.key === "F10" &&
    !input.shiftKey &&
    !input.ctrlKey &&
    !input.altKey &&
    !input.metaKey
  );
}

/**
 * 頭文字で項目へ移る入力の文字を返す。印字できる 1 文字で、修飾キーを伴わないときだけ返す。
 * 空白は項目の実行に使うため返さない。
 */
export function typeaheadCharacter(input: MenuKeyInput): string | undefined {
  if (input.ctrlKey || input.altKey || input.metaKey) return undefined;
  if ([...input.key].length !== 1 || input.key.trim() === "") return undefined;
  return input.key;
}

/**
 * current の次から順に見て、文字列が character で始まる最初の位置を返す。末尾まで見たら先頭へ
 * 戻る。大文字と小文字を区別しない。該当する文字列が無ければ undefined を返す。
 */
export function indexByInitial(
  labels: readonly string[],
  current: number,
  character: string,
): number | undefined {
  const folded = character.toLocaleLowerCase();
  for (let step = 1; step <= labels.length; step++) {
    const index = (current + step + labels.length) % labels.length;
    if (labels[index]?.toLocaleLowerCase().startsWith(folded)) return index;
  }
  return undefined;
}

/** 位置を step だけ動かし、端を越えたら反対の端へ回す。 */
export function wrapIndex(index: number, step: number, length: number): number {
  return (((index + step) % length) + length) % length;
}

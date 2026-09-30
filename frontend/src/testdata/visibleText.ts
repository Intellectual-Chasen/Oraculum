/** 画面に出る文字列を返す。読み上げだけに渡す sr-only の文字列を除く。 */
export function visibleText(element: Element | null | undefined): string {
  if (element === null || element === undefined) return "";
  const clone = element.cloneNode(true) as Element;
  for (const hidden of clone.querySelectorAll(".sr-only")) hidden.remove();
  return clone.textContent ?? "";
}

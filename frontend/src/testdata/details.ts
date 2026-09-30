import { act, fireEvent, screen } from "@testing-library/react";

/**
 * 見出しの文字列が name に一致する `<details>` を開く。jsdom は summary の click で開閉しないため、
 * `open` を立てて toggle を送る。開いている `<details>` はそのままにする。
 */
export function openDetails(name: RegExp | string) {
  const summary = screen.getByText(name, { selector: "summary" });
  const details = summary.parentElement as HTMLDetailsElement;
  if (details.open) {
    return;
  }
  act(() => {
    details.open = true;
    fireEvent(details, new Event("toggle"));
  });
}

/** 見出しが現れるのを待ってから `<details>` を開く。 */
export async function findAndOpenDetails(name: RegExp | string) {
  await screen.findByText(name, { selector: "summary" });
  openDetails(name);
}

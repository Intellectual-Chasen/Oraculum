// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { DerivedLabelNote } from "./DerivedLabelNote";

afterEach(cleanup);

test("取り込みが英語で書いた作り方を、日本語の作り方の組で出す", () => {
  const { container } = render(
    <DerivedLabelNote derivation="IPv4-mapped IPv6 address written as dotted decimal IPv4" />,
  );
  expect(container.textContent).toBe(
    "Oraculum が作った表示名\n作り方: IPv6 の形で書いた IPv4 のアドレスをドット 10 進の IPv4 に直した値",
  );
});

test("画面には印の icon だけを出し、作り方は読み上げと、マウスを重ねたときと keyboard の focus の tooltip に出す", () => {
  const { container } = render(
    <DerivedLabelNote derivation="収集元の file 名" />,
  );
  const mark = container.querySelector(".derived-note");
  expect(mark?.querySelector("svg")).not.toBeNull();
  expect(mark?.getAttribute("title")).toBe(
    "Oraculum が作った表示名\n作り方: 収集元の file 名",
  );
  expect(screen.getByText(/作り方: 収集元の file 名/).className).toBe(
    "sr-only",
  );
  // keyboard の利用者も focus で tooltip を開ける。
  expect(mark?.getAttribute("tabindex")).toBe("0");
});

test("対応表に無い作り方は、受け取った文字列のまま出す", () => {
  const { container } = render(
    <DerivedLabelNote derivation="収集元の file 名" />,
  );
  expect(container.textContent).toContain("作り方: 収集元の file 名");
});

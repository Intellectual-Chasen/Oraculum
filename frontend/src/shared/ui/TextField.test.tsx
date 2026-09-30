// @vitest-environment jsdom
import { cleanup, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { inputVariants, TextField } from "./TextField";

afterEach(() => {
  cleanup();
});

test("1 行の入力欄は、高さを button と同じ --control-height から取る", () => {
  expect(inputVariants({})).toMatch(/(^| )h-\(--control-height\)( |$)/);
});

function describedTexts(input: HTMLElement): string[] {
  return (input.getAttribute("aria-describedby") ?? "")
    .split(" ")
    .filter((id) => id !== "")
    .map((id) => document.getElementById(id)?.textContent ?? "");
}

test("説明を入力欄に結び付ける", () => {
  render(
    <TextField label="下端" description="0 以上の整数を入れる。" value="" />,
  );
  const input = screen.getByRole("textbox", { name: "下端" });
  expect(input.getAttribute("aria-invalid")).toBeNull();
  expect(describedTexts(input)).toEqual(["0 以上の整数を入れる。"]);
});

test("誤りを渡すと、欄を誤りの状態にし、誤りを入力欄に結び付ける", () => {
  render(
    <TextField
      label="下端"
      description="0 以上の整数を入れる。"
      errorMessage="下端 4800 が上端 4624 より大きい。"
      value="4800"
    />,
  );
  const input = screen.getByRole("textbox", { name: "下端" });
  expect(input.getAttribute("aria-invalid")).toBe("true");
  expect(describedTexts(input)).toContain("下端 4800 が上端 4624 より大きい。");
});

// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { Button, buttonVariants } from "./Button";
import { PortalContainerContext } from "./portalContainer";

afterEach(() => {
  cleanup();
});

const reason = { title: "記録できない", text: "記録には編集者の役割が要る。" };

test("押せる button は押すと onPress を呼ぶ", () => {
  const onPress = vi.fn();
  render(<Button onPress={onPress}>足す</Button>);
  fireEvent.click(screen.getByRole("button", { name: "足す" }));
  expect(onPress).toHaveBeenCalledTimes(1);
});

test("理由を持つ押せない button は、focus を受け、押しても onPress を呼ばない", () => {
  const onPress = vi.fn();
  render(
    <Button isDisabled disabledReason={reason} onPress={onPress}>
      メモを記録する
    </Button>,
  );
  const button = screen.getByRole("button", { name: "メモを記録する" });
  expect(button.getAttribute("aria-disabled")).toBe("true");
  expect(button.hasAttribute("disabled")).toBe(false);
  fireEvent.click(button);
  expect(onPress).not.toHaveBeenCalled();
});

test("理由を持つ押せない button は、tooltip を開く前から理由を読み上げに結び付ける", () => {
  render(
    <Button isDisabled disabledReason={reason}>
      メモを記録する
    </Button>,
  );
  const button = screen.getByRole("button", { name: "メモを記録する" });
  const described = document.getElementById(
    button.getAttribute("aria-describedby") ?? "",
  );
  expect(described?.textContent).toBe(
    "記録できない: 記録には編集者の役割が要る。",
  );
});

test("理由を持つ押せない button は、呼び出し元が結び付けた説明も残す", () => {
  render(
    <>
      <p id="viewer-note">閲覧者の役割です。</p>
      <Button isDisabled disabledReason={reason} aria-describedby="viewer-note">
        メモを記録する
      </Button>
    </>,
  );
  const ids = screen
    .getByRole("button", { name: "メモを記録する" })
    .getAttribute("aria-describedby")
    ?.split(" ");
  expect(ids?.[0]).toBe("viewer-note");
  expect(ids).toHaveLength(2);
});

test("理由を持つ押せない button は、keyboard の focus で理由を出す", () => {
  render(
    <Button isDisabled disabledReason={reason}>
      メモを記録する
    </Button>,
  );
  const button = screen.getByRole("button", { name: "メモを記録する" });
  act(() => {
    fireEvent.keyDown(document.body, { key: "Tab" });
    button.focus();
  });
  const tooltip = screen.getByRole("tooltip");
  expect(tooltip.textContent).toContain("記録できない");
  expect(tooltip.textContent).toContain("記録には編集者の役割が要る。");
  // 開いたままの理由が、下にある区画の tab の click を受け止めない。
  expect(tooltip.className).toContain("pointer-events-none");
});

test("理由は、区画が指す描く先の要素に描く", () => {
  const container = document.createElement("div");
  document.body.append(container);
  render(
    <PortalContainerContext value={() => container}>
      <Button isDisabled disabledReason={reason}>
        メモを記録する
      </Button>
    </PortalContainerContext>,
  );
  act(() => {
    fireEvent.keyDown(document.body, { key: "Tab" });
    screen.getByRole("button", { name: "メモを記録する" }).focus();
  });
  expect(container.contains(screen.getByRole("tooltip"))).toBe(true);
  container.remove();
});

test("理由を持つ押せない送信 button は、押しても form を送らない", () => {
  const onSubmit = vi.fn((event: { preventDefault: () => void }) =>
    event.preventDefault(),
  );
  render(
    <form onSubmit={onSubmit}>
      <input aria-label="メモ" />
      <input aria-label="記録者" />
      <Button type="submit" isDisabled disabledReason={reason}>
        メモを記録する
      </Button>
    </form>,
  );
  const button = screen.getByRole("button", { name: "メモを記録する" });
  fireEvent.click(button);
  expect(onSubmit).not.toHaveBeenCalled();
  expect(button.getAttribute("type")).toBe("button");
});

test("どの大きさの button も、高さを入力欄と同じ --control-height から取る", () => {
  for (const size of ["md", "sm", "icon", "iconSm"] as const) {
    expect(buttonVariants({ size })).toMatch(
      /(^| )(h|size)-\(--control-height\)( |$)/,
    );
  }
});

test("理由を持たない押せない button は、focus を受けない", () => {
  render(<Button isDisabled>足す</Button>);
  const button = screen.getByRole("button", { name: "足す" });
  expect(button.hasAttribute("disabled")).toBe(true);
});

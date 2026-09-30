// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { defaultLayoutSettings } from "./cosmosLayout";
import {
  LayoutSettingsInputs,
  LayoutSettingsPopover,
} from "./LayoutSettingsInputs";

afterEach(cleanup);

test("範囲の中の値で設定を差し替え、範囲の外の値と数でない値は渡さない", () => {
  const changed = vi.fn();
  const { getByRole } = render(
    <LayoutSettingsInputs value={defaultLayoutSettings} onChange={changed} />,
  );
  const cluster = getByRole("spinbutton", {
    name: "互いに多く繋がったノードを寄せる強さ",
  });
  expect(getByRole("spinbutton", { name: "中心へ寄せる強さ" })).toBeTruthy();
  expect(getByRole("spinbutton", { name: "エッジの長さ" })).toBeTruthy();

  fireEvent.change(cluster, { target: { value: "0.3" } });
  expect(changed).toHaveBeenLastCalledWith({
    ...defaultLayoutSettings,
    simulationCluster: 0.3,
  });

  fireEvent.change(cluster, { target: { value: "2" } });
  fireEvent.change(cluster, { target: { value: "" } });
  expect(changed).toHaveBeenCalledTimes(1);
});

test("範囲の外の値を打って欄から離れると、欄の表示を使っている値に戻す", () => {
  const { getByRole } = render(
    <LayoutSettingsInputs value={defaultLayoutSettings} onChange={() => {}} />,
  );
  const gravity = getByRole("spinbutton", {
    name: "中心へ寄せる強さ",
  }) as HTMLInputElement;

  fireEvent.change(gravity, { target: { value: "9" } });
  fireEvent.blur(gravity);

  expect(gravity.value).toBe(String(defaultLayoutSettings.simulationGravity));
});

test("配置の設定は button で開くまで欄を出さず、開くと値の意味と範囲を添えた欄を出す", () => {
  render(
    <LayoutSettingsPopover value={defaultLayoutSettings} onChange={() => {}} />,
  );
  expect(screen.queryByRole("spinbutton")).toBeNull();

  fireEvent.click(screen.getByRole("button", { name: "配置の設定" }));

  const dialog = screen.getByRole("dialog", { name: "配置の設定" });
  expect(dialog.textContent).toContain("大きい値: 離れるノード");
  expect(dialog.textContent).toContain("範囲: 1–100");
  expect(screen.getAllByRole("spinbutton")).toHaveLength(3);
});

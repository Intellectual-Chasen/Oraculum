// @vitest-environment jsdom
import { afterEach, beforeEach, expect, test, vi } from "vitest";

// 判定の結果は module が 1 回だけ持つ。test ごとに module を読み直し、前の test の結果を使わない。
let canDrawWithGpu: () => boolean;
beforeEach(async () => {
  vi.resetModules();
  ({ canDrawWithGpu } = await import("./webglProbe"));
});

afterEach(() => {
  vi.restoreAllMocks();
});

test("context を作れないときは、描けないと返す", () => {
  const getContext = vi
    .spyOn(HTMLCanvasElement.prototype, "getContext")
    .mockReturnValue(null);
  expect(canDrawWithGpu()).toBe(false);
  // ソフトウェアで描く実装を断るよう求める。
  expect(getContext).toHaveBeenCalledWith("webgl2", {
    failIfMajorPerformanceCaveat: true,
  });
});

test("context を作る途中で例外が出たときは、描けないと返す", () => {
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockImplementation(() => {
    throw new Error("Device initialization failed");
  });
  expect(canDrawWithGpu()).toBe(false);
});

/** 描く実装の名前が renderer の context の代わり。 */
function contextOf(renderer: string, loseContext: () => void) {
  return {
    getExtension: (name: string) =>
      name === "WEBGL_debug_renderer_info"
        ? { UNMASKED_RENDERER_WEBGL: 0x9246 }
        : { loseContext },
    getParameter: () => renderer,
  } as unknown as WebGL2RenderingContext;
}

test("GPU で描く context を作れたときは描けると返し、確かめに作った context を手放す", () => {
  const loseContext = vi.fn();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
    contextOf("ANGLE (Example GPU Direct3D11)", loseContext),
  );
  expect(canDrawWithGpu()).toBe(true);
  expect(loseContext).toHaveBeenCalledTimes(1);
});

test.each([
  "ANGLE (Google, Vulkan 1.3.0 (SwiftShader Device))",
  "ANGLE (Microsoft, Microsoft Basic Render Driver Direct3D11 vs_5_0 ps_5_0)",
])("CPU で描く実装 %s の context は、描けないと返す", (renderer) => {
  const loseContext = vi.fn();
  vi.spyOn(HTMLCanvasElement.prototype, "getContext").mockReturnValue(
    contextOf(renderer, loseContext),
  );
  expect(canDrawWithGpu()).toBe(false);
  expect(loseContext).toHaveBeenCalledTimes(1);
});

test("2 回目からは context を作らず、1 回目の判定の結果を返す", () => {
  const getContext = vi
    .spyOn(HTMLCanvasElement.prototype, "getContext")
    .mockReturnValue(contextOf("ANGLE (Example GPU Direct3D11)", vi.fn()));
  expect(canDrawWithGpu()).toBe(true);
  expect(canDrawWithGpu()).toBe(true);
  expect(getContext).toHaveBeenCalledTimes(1);
});

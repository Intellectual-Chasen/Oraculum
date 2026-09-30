// jest-dom の matcher を vitest の expect に登録し、型を tsc へ渡す。
import "@testing-library/jest-dom/vitest";
import { configure } from "@testing-library/dom";

// 既知の制限: findBy と waitFor の待ちの上限を既定の 1 秒から 10 秒に延ばす, CPU を他の検査と共有する CI では描画の後の要素の出現が 1 秒を超えて待ちが失敗し、負荷と test を 1 core に固定した手元でも画面の test 5 件が時間切れになった, CI の runner が検査を並べて動かさなくなったときに見直す
configure({ asyncUtilTimeout: 10_000 });

// jsdom は ResizeObserver を持たない。ビューの作業場所は大きさの変化を監視するため、何も
// 通知しない代わりを置く。
globalThis.ResizeObserver ??= class {
  observe() {}
  unobserve() {}
  disconnect() {}
};

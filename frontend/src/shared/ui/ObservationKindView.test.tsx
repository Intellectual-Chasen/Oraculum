// @vitest-environment jsdom
import { cleanup, render } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { ObservationKind } from "../contracts/common";
import { ObservationKindView } from "./ObservationKindView";

afterEach(cleanup);

// 意味の状態の 3 値は `backend/core/observation_kind.go` の `ObservationKindStatus` が
// 定める。**3 値それぞれの表示を通す。** 1 つでも通らない値があると、その状態の
// レコードだけが画面で読めないまま残る。

function netKind(
  subEvt: string,
  status: ObservationKind["status"],
  meaning?: string,
): ObservationKind {
  return {
    raw: [
      {
        name: "evt",
        kind: "text",
        text: { rawText: "net", valueState: "present" },
      },
      {
        name: "subEvt",
        kind: "text",
        text: { rawText: subEvt, valueState: "present" },
      },
    ],
    status,
    ...(meaning === undefined ? {} : { meaning }),
  };
}

function drawn(observationKind: ObservationKind): string {
  const { container } = render(
    <ObservationKindView observationKind={observationKind} />,
  );
  return container.textContent ?? "";
}

test("仕様書が意味を定めた種別に、意味が確定であることを出す", () => {
  const text = drawn(netKind("con", "determined"));

  expect(text).toContain("意味: 確定");
  expect(text).not.toContain("意味: 推定");
  expect(text).not.toContain("意味: 不明");
});

test("推定した種別に、推定であることと推定した意味を出す", () => {
  const meaning = "TCP 接続が確立したときに出力される";
  const text = drawn(netKind("est", "inferred", meaning));

  expect(text).toContain("意味: 推定");
  expect(text).toContain(`推定した意味: ${meaning}`);
});

test("意味を確定できない種別に、不明であることを出す", () => {
  const text = drawn(netKind("exampleSubEvent", "undetermined"));

  expect(text).toContain("意味: 不明");
  expect(text).not.toContain("意味: 推定");
});

test("推定した意味の制御文字を可視の符号にして出す", () => {
  const text = drawn(netKind("est", "inferred", "TCP‮接続"));

  expect(text).toContain("TCPU+202E接続");
});

test("イベントの種類のフィールドを持たない入力形式に、フィールドが無いことを出す", () => {
  const text = drawn({ raw: [] });

  expect(text).toContain("イベントの種類のフィールドなし");
});

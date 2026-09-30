import { expect, test } from "vitest";
import type { NodeKind } from "@/shared/contracts/graph";
import { autoView, resolveView, toggledView } from "./searchView";

test("検索の条件が無い自動の選択は、端末と IP アドレスを出す", () => {
  expect(resolveView(autoView, { hasCondition: false })).toEqual({
    granularity: "object",
    nodeKinds: ["terminal", "ip"],
  });
});

test.each([
  ["ps", ["process"]],
  ["net", ["process", "ip"]],
])(
  "事象の分類 %s を持つ自動の選択は、%s の種別を出す",
  (eventCategory, nodeKinds) => {
    expect(
      resolveView(autoView, { hasCondition: true, eventCategory }),
    ).toEqual({ granularity: "object", nodeKinds });
  },
);

// 分類の対応に無い文字列と、分類を持たない条件は、すべての対象を出す。
test.each([["file"], [undefined]])(
  "事象の分類 %s の自動の選択は、すべての対象を出す",
  (eventCategory) => {
    expect(
      resolveView(autoView, { hasCondition: true, eventCategory }),
    ).toEqual({ granularity: "object" });
  },
);

// 条件の有無の判定は呼び出し元が持つため、分類だけを与えて条件が無いとした入力は端末と IP アドレスを出す。
test("条件が無いと言う入力では、事象の分類を読まない", () => {
  expect(
    resolveView(autoView, { hasCondition: false, eventCategory: "ps" })
      .nodeKinds,
  ).toEqual(["terminal", "ip"]);
});

test("手で選んだ種別は、検索の条件に依らずそのまま使い、選んだ種別のどれかを出す", () => {
  const chosen = { kind: "manual", nodeKinds: ["process", "file"] } as const;
  for (const input of [
    { hasCondition: false },
    { hasCondition: true, eventCategory: "net" },
  ]) {
    expect(resolveView(chosen, input)).toEqual({
      granularity: "object",
      nodeKinds: ["process", "file"],
    });
  }
});

test("手で選んだ種別にレコードが入るときはレコードの粒度で出し、入らないときは対象の粒度で出す", () => {
  expect(
    resolveView(
      { kind: "manual", nodeKinds: ["record", "process"] },
      { hasCondition: true },
    ).granularity,
  ).toBe("record");
  expect(
    resolveView(
      { kind: "manual", nodeKinds: ["process"] },
      { hasCondition: true },
    ).granularity,
  ).toBe("object");
});

test("種別を 1 つも選んでいない手の選択は、すべての対象を出す", () => {
  expect(
    resolveView({ kind: "manual", nodeKinds: [] }, { hasCondition: true }),
  ).toEqual({ granularity: "object" });
});

test("種別を押すと、今出している種別に足すか外した手の選択になり、並びは与えた順を保つ", () => {
  const order: NodeKind[] = ["terminal", "process", "ip", "file"];
  const processIp = resolveView(autoView, {
    hasCondition: true,
    eventCategory: "net",
  });
  expect(toggledView(processIp, "file", order)).toEqual({
    kind: "manual",
    nodeKinds: ["process", "ip", "file"],
  });
  expect(toggledView(processIp, "process", order)).toEqual({
    kind: "manual",
    nodeKinds: ["ip"],
  });
  // すべての対象を出しているときは、押した種別だけの選択になる。
  expect(
    toggledView(resolveView(autoView, { hasCondition: true }), "file", order),
  ).toEqual({ kind: "manual", nodeKinds: ["file"] });
});

test("同じ自動の結果に同じ値を返し、要求の組の identity を保つ", () => {
  expect(resolveView(autoView, { hasCondition: false })).toBe(
    resolveView(autoView, { hasCondition: false, eventCategory: "ps" }),
  );
});

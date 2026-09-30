import { expect, test } from "vitest";
import { maxOriginNodes, type NodeRef } from "@/shared/api/graph";
import { everyMatchCondition } from "@/shared/api/matchConditions";
import {
  type Exploration,
  explorationCriteria,
  withOrigin,
  withoutOrigin,
} from "./exploration";

test("レコードのノードを起点に含む関係先の要求は、レコードの粒度で読む", () => {
  const record: NodeRef = {
    id: "n:record:synthetic",
    label: "4624",
    kind: "record",
  };
  const address: NodeRef = { id: "n:ip:x", label: "192.0.2.1", kind: "ip" };
  const criteria = (origins: NodeRef[], view: "object" | "record" = "object") =>
    explorationCriteria(
      { kind: "neighbours", origins },
      everyMatchCondition(),
      200,
      [],
      view,
    );

  expect(criteria([record]).granularity).toBe("record");
  expect(criteria([address, record]).granularity).toBe("record");
  expect(criteria([address]).granularity).toBe("object");
  // グラフに出す対象にレコードがあれば、レコード以外の起点からもレコードの粒度で読む。
  expect(criteria([address], "record").granularity).toBe("record");
});

test("関係先の要求は、画面で選んでいる関係の種別で辿る", () => {
  const address: NodeRef = { id: "n:ip:x", label: "192.0.2.1", kind: "ip" };
  const neighbours: Exploration = { kind: "neighbours", origins: [address] };

  expect(
    explorationCriteria(
      neighbours,
      everyMatchCondition(),
      200,
      ["terminal_address"],
      "object",
    ).edgeKinds,
  ).toStrictEqual(["terminal_address"]);
  expect(
    explorationCriteria(neighbours, everyMatchCondition(), 200, [], "object")
      .edgeKinds,
  ).toBeUndefined();
});

function nodes(count: number): NodeRef[] {
  return Array.from({ length: count }, (_, index) => ({
    id: `n:ip:${index}`,
    label: `192.0.2.${index}`,
  }));
}

test("関係先を出していないときは、足したノードだけを起点にし、検索の結果から始めたときだけ検索の結果を保つ", () => {
  const [first] = nodes(1);
  if (first === undefined) {
    throw new Error("no node");
  }
  expect(withOrigin({ kind: "lineage", origin: first }, first)).toStrictEqual({
    kind: "neighbours",
    origins: [first],
    keepsSearch: false,
  });
  expect(withOrigin(undefined, first)).toStrictEqual({
    kind: "neighbours",
    origins: [first],
    keepsSearch: true,
  });
});

test("起点が上限より少ないときは足し、上限に達した後と、既にある起点は足さない", () => {
  const all = nodes(maxOriginNodes + 1);
  const belowLimit: Exploration = {
    kind: "neighbours",
    origins: all.slice(0, maxOriginNodes - 1),
  };
  const last = all[maxOriginNodes - 1];
  const beyond = all[maxOriginNodes];
  if (last === undefined || beyond === undefined) {
    throw new Error("no node");
  }

  const full = withOrigin(belowLimit, last);
  expect(full).toStrictEqual({
    kind: "neighbours",
    origins: all.slice(0, maxOriginNodes),
  });
  expect(withOrigin(full, beyond)).toBe(full);
  expect(withOrigin(belowLimit, all[0] as NodeRef)).toBe(belowLimit);
});

test("起点を外し、最後の起点を外すと探索を終える", () => {
  const [first, second] = nodes(2);
  if (first === undefined || second === undefined) {
    throw new Error("no node");
  }
  const two: Exploration = { kind: "neighbours", origins: [first, second] };

  expect(withoutOrigin(two, first.id)).toStrictEqual({
    kind: "neighbours",
    origins: [second],
  });
  expect(
    withoutOrigin({ kind: "neighbours", origins: [first] }, first.id),
  ).toBeUndefined();
});

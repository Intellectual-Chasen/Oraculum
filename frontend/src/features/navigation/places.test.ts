import { expect, test } from "vitest";
import type { RecordLocator } from "@/shared/contracts/common";
import {
  emptyHistory,
  move,
  type Place,
  placeLabel,
  relabelCurrent,
  visit,
} from "./places";

const recordRef: RecordLocator = {
  sourceId: "src-1",
  sourceContentSha256: "a".repeat(64),
  sourceFileName: "host-a.evtx",
  positionKind: "byte_range",
  byteOffset: 1024,
  byteLength: 512,
  recordRawTextRef: "/api/v0/records?byteOffset=1024",
};

test("レコードの場所の表示名は、収集元と位置を「名前: 値」の組で並べ、Event ID が分かるときは先頭に置く", () => {
  expect(placeLabel({ kind: "record", ref: recordRef })).toBe(
    "収集元: host-a.evtx、位置: 1024-1536",
  );
  expect(placeLabel({ kind: "record", ref: recordRef, eventId: "4624" })).toBe(
    "Event ID: 4624、収集元: host-a.evtx、位置: 1024-1536",
  );
});

test("読み込んだレコードの Event ID は、今の場所が同じレコードのときだけ表示名へ入れる", () => {
  const history = visit(emptyHistory, { kind: "record", ref: recordRef });
  const labeled = relabelCurrent(history, {
    kind: "record",
    ref: { ...recordRef },
    eventId: "4624",
  });
  expect(labeled.places).toEqual([
    { kind: "record", ref: { ...recordRef }, eventId: "4624" },
  ]);
  const moved = visit(history, node("x"));
  expect(
    relabelCurrent(moved, { kind: "record", ref: recordRef, eventId: "4624" }),
  ).toBe(moved);
});

const node = (id: string): Place => ({
  kind: "node",
  node: { id, label: id },
});

test("戻った後に別の場所を見ると進む先を捨て、今の場所と同じ場所は足さずに表示名を置き換える", () => {
  let history = visit(
    visit(visit(emptyHistory, node("a")), node("b")),
    node("c"),
  );
  history = move(move(history, -1), -1);
  expect(history.index).toBe(0);
  expect(move(history, -1)).toBe(history);

  const relabeled = visit(history, {
    kind: "node",
    node: { id: "a", label: "HOST-A" },
  });
  expect(relabeled.index).toBe(0);
  expect(relabeled.places).toHaveLength(3);
  expect(relabeled.places[0]).toEqual({
    kind: "node",
    node: { id: "a", label: "HOST-A" },
  });

  history = visit(history, node("d"));
  expect(
    history.places.map((place) => place.kind === "node" && place.node.id),
  ).toEqual(["a", "d"]);
  expect(move(history, 1)).toBe(history);
});

test("見た場所をすべて履歴に残し、最初の場所まで戻れる", () => {
  let history = emptyHistory;
  for (let index = 0; index < 250; index += 1) {
    history = visit(history, node(`n${index}`));
  }
  expect(history.places).toHaveLength(250);
  expect(history.index).toBe(249);
  for (let step = 0; step < 249; step += 1) history = move(history, -1);
  expect(history.places[history.index]).toEqual(node("n0"));
});

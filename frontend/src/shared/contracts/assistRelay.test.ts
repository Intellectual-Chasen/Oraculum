import { expect, test } from "vitest";
import { decodeAssistEvent } from "./assistRelay";
import { DecodeFailure } from "./decoding";

const card = {
  sequence: 2,
  turnId: "t1",
  kind: "search_query_card",
  searchQuery: { depth: 2, nodeIds: ["n:process:1", "n:record:2"] },
  origins: [
    { id: "n:process:1", kind: "process", label: "a.exe" },
    { id: "n:record:2", kind: "record", label: "" },
  ],
};

test("起点を持つ card は、nodeIds と同じ順の起点の種別と表示名を読む", () => {
  expect(decodeAssistEvent(card, "$").origins).toEqual(card.origins);
});

test.each([
  ["起点の無い nodeIds", { ...card, origins: undefined }],
  ["順の違う起点", { ...card, origins: [...card.origins].reverse() }],
  ["nodeIds の無い起点", { ...card, searchQuery: { depth: 2 } }],
  [
    "知らない種別の起点",
    {
      ...card,
      origins: [
        card.origins[0],
        { id: "n:record:2", kind: "planet", label: "" },
      ],
    },
  ],
])("%s を読めないものとする", (_name, event) => {
  expect(() => decodeAssistEvent(event, "$")).toThrow(DecodeFailure);
});

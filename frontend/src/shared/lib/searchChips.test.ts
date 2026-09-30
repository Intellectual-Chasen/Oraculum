import { expect, test } from "vitest";
import { graphChipsOf } from "./searchChips";

test("ノードID の chip はノードの表示名を値にし、表示名の無いノードは識別子を値にする", () => {
  const chips = graphChipsOf({
    depth: 1,
    origins: [
      { id: "n:process:1", label: "a.exe", kind: "process" },
      { id: "n:record:2", label: "", kind: "record" },
    ],
  }).filter((chip) => chip.kind === "origin");
  expect(chips.map((chip) => [chip.label, chip.value, chip.id])).toEqual([
    ["ノードID", "a.exe", "n:process:1"],
    ["ノードID", "n:record:2", "n:record:2"],
  ]);
});

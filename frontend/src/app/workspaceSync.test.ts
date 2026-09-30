import { expect, test } from "vitest";
import { changedFields, patchOf, withSelection } from "./workspaceSync";

test.each([
  { a: {}, b: {}, fields: [] },
  { a: { x: 1 }, b: { x: 1 }, fields: [] },
  { a: { x: 1 }, b: { x: 2 }, fields: ["x"] },
  { a: { x: 1 }, b: {}, fields: ["x"] },
  { a: {}, b: { y: [1] }, fields: ["y"] },
  { a: { x: { p: 1 }, y: 2 }, b: { x: { p: 2 }, y: 2 }, fields: ["x"] },
  { a: { x: undefined }, b: {}, fields: [] },
])("changedFields($a, $b) は $fields", ({ a, b, fields }) => {
  expect(changedFields(a, b)).toEqual(fields);
});

test.each([
  { state: { x: 1, y: 2 }, fields: ["x"], patch: { x: 1 } },
  { state: { x: 1 }, fields: ["y"], patch: { y: null } },
  { state: { x: undefined }, fields: ["x"], patch: { x: null } },
  { state: { x: 1 }, fields: [], patch: {} },
])("patchOf($state, $fields) は $patch", ({ state, fields, patch }) => {
  expect(patchOf(state, fields)).toEqual(patch);
});

test.each([
  {
    name: "選択を置き換え、graph の他の欄を保つ",
    state: { a: 1, selectedEdgeId: "e1", graph: { view: "v", selected: "n1" } },
    selection: { selectedEdgeId: "e2", graphSelected: "n2" },
    result: {
      a: 1,
      selectedEdgeId: "e2",
      graph: { view: "v", selected: "n2" },
    },
  },
  {
    name: "選択に無い欄を消す",
    state: {
      openedRecord: "r",
      evidenceGroupSelector: "g",
      graph: { selected: "n1" },
    },
    selection: {},
    result: { graph: {} },
  },
  {
    name: "null の選択は空の選択として扱う",
    state: { selectedEdgeId: "e1" },
    selection: null,
    result: {},
  },
  {
    name: "graph が無い状態に graph を足さない",
    state: {},
    selection: { graphSelected: "n1", openedRecord: "r" },
    result: { openedRecord: "r" },
  },
])("withSelection: $name", ({ state, selection, result }) => {
  expect(withSelection(state, selection)).toEqual(result);
});

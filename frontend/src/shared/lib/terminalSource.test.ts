import { expect, test } from "vitest";
import type { GraphNode } from "../contracts/graph";
import { distinctTerminalNames } from "./terminalSource";

function terminal(id: string, label: string, content?: string): GraphNode {
  return {
    id,
    kind: "terminal",
    keyForm:
      content === undefined
        ? "terminal_id"
        : "recording_source_content_sha256_hostname",
    identity:
      content === undefined
        ? [{ value: label }]
        : [{ value: content }, { value: label }],
    label: { rawText: label, valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
  };
}

test("同じ端末のノードが一覧に 2 回現れても、同じ表示名の別の端末と数えない", () => {
  const names = new Map([["a".repeat(64), "x.log"]]);
  const node = terminal("n:1", "host01", "a".repeat(64));
  expect([...distinctTerminalNames([node, node], names)]).toEqual([
    ["n:1", "host01"],
  ]);
});

test("同じ表示名の端末にだけ、記録した収集元の表示名を足す。探せない収集元は足さない", () => {
  const names = new Map([
    ["a".repeat(64), "x.log (host-a)"],
    ["b".repeat(64), "x.log (host-b)"],
  ]);
  expect([
    ...distinctTerminalNames(
      [
        terminal("n:1", "host01", "a".repeat(64)),
        terminal("n:2", "host01", "b".repeat(64)),
        terminal("n:3", "host01", "c".repeat(64)),
        terminal("n:4", "host02", "a".repeat(64)),
        terminal("n:5", "host03"),
      ],
      names,
    ),
  ]).toEqual([
    ["n:1", "host01、収集元: x.log (host-a)"],
    ["n:2", "host01、収集元: x.log (host-b)"],
    ["n:3", "host01"],
    ["n:4", "host02"],
    ["n:5", "host03"],
  ]);
});

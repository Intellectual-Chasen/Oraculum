import { expect, test } from "vitest";
import { graphResponseJson } from "@/testdata/graph/graphResponse";
import { DecodeFailure } from "./decoding";
import { decodeGraphResponse } from "./graph";

test("検索の条件と粒度と端末を、応答が用いた条件として読む", () => {
  const decoded = decodeGraphResponse(
    {
      ...graphResponseJson(),
      nodeKinds: ["process", "ip"],
      granularity: "object",
      terminal: "n:terminal:1f0c",
      valueContains: ["code", "tunnel"],
      valueExcludes: ["dcon"],
    },
    "graph",
  );

  expect(decoded.nodeKinds).toEqual(["process", "ip"]);
  expect(decoded.granularity).toBe("object");
  expect(decoded.terminal).toBe("n:terminal:1f0c");
  expect(decoded.valueContains).toEqual(["code", "tunnel"]);
  expect(decoded.valueExcludes).toEqual(["dcon"]);
  expect(decoded.matchedKinds).toEqual(graphResponseJson().matchedKinds);
});

test("種別ごとの件数の和が nodeCount と一致しない応答と、未知の種別と粒度を退ける", () => {
  const spoiled: Record<string, object> = {
    "a sum below nodeCount": { matchedKinds: [{ kind: "process", count: 1 }] },
    "no matched kinds": { matchedKinds: undefined },
    "an unknown node kind": { nodeKinds: ["session"] },
    "an unknown granularity": { granularity: "session" },
    "a token that is not a string": { valueContains: [1] },
  };
  for (const [name, change] of Object.entries(spoiled)) {
    expect(
      () => decodeGraphResponse({ ...graphResponseJson(), ...change }, "graph"),
      name,
    ).toThrow(DecodeFailure);
  }
});

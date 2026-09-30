import { expect, test } from "vitest";
import { isSquidCacheHit } from "./proxyStatus";

test.each([
  ["TCP_HIT:NONE", true],
  ["TCP_MEM_HIT:NONE", true],
  ["TCP_IMS_HIT", true],
  ["TCP_MISS:DIRECT", false],
  ["TCP_MISS:HIT_PARENT", false],
  ["TCP_HITLESS:DIRECT", false],
  ["tcp_hit:none", false],
  ["", false],
])("%s は cache からの応答が %s である", (status, want) => {
  expect(isSquidCacheHit(status)).toBe(want);
});

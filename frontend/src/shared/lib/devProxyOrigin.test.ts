import { expect, test } from "vitest";
import { shouldRewriteDevProxyOrigin } from "./devProxyOrigin";

test.each([
  [
    "同じ端末の開発 server の origin",
    "http://localhost:5173",
    "127.0.0.1",
    true,
  ],
  ["IPv6 の loopback からの要求", "http://localhost:5173", "::1", true],
  [
    "IPv4 を写した IPv6 の loopback からの要求",
    "http://localhost:5173",
    "::ffff:127.0.0.1",
    true,
  ],
  ["別の origin の page", "http://attacker.example", "127.0.0.1", false],
  ["別の port の page", "http://localhost:3000", "127.0.0.1", false],
  ["https の origin", "https://localhost:5173", "127.0.0.1", false],
  ["sandbox の frame", "null", "127.0.0.1", false],
  ["Origin を持たない要求", undefined, "127.0.0.1", false],
  ["別の端末からの要求", "http://localhost:5173", "192.0.2.10", false],
  ["接続元が分からない要求", "http://localhost:5173", undefined, false],
])("%s の Origin を書き換えるか", (_name, origin, remoteAddress, expected) => {
  expect(
    shouldRewriteDevProxyOrigin({
      origin,
      host: "localhost:5173",
      remoteAddress,
    }),
  ).toBe(expected);
});

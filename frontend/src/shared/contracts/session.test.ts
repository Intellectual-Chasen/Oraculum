import { expect, test } from "vitest";
import { accountSessionJson } from "@/testdata/session";
import { DecodeFailure } from "./decoding";
import { decodeSession } from "./session";

test("アカウントを持つ起動の応答から、ログイン名と表示名を読む", () => {
  expect(decodeSession(accountSessionJson(), "response")).toEqual({
    authentication: "account",
    login: "alice",
    displayName: "石橋",
  });
});

test.each([
  ["ログイン名を持つ none", { authentication: "none", login: "alice" }],
  ["表示名を欠く account", { authentication: "account", login: "alice" }],
  ["定義の外の authentication", { authentication: "token" }],
])("%s の応答を読まない", (_name, body) => {
  expect(() => decodeSession(body, "response")).toThrow(DecodeFailure);
});

import { expect, test } from "vitest";
import {
  noSearchTerms,
  type SearchTerms,
  withExpression,
  withField,
  withFieldTerm,
  withoutExpression,
  withoutField,
  withoutFieldTerm,
  withoutTerm,
  withTerm,
} from "./searchTerms";

const heldPair: SearchTerms = {
  contains: [],
  excludes: [],
  fieldContains: ["TargetUserName=user-a"],
};

test.each([
  [
    "前後の空白を外して足す",
    " LogonType ",
    " 3 ",
    ["TargetUserName=user-a", "LogonType=3"],
  ],
  ["空白だけの欄は足さない", "  ", "3", undefined],
  ["空白だけの文字列は足さない", "LogonType", "  ", undefined],
  ["= を含む欄は足さない", "Logon=Type", "3", undefined],
  ["同じ組は足さない", "TargetUserName", "user-a", undefined],
  // 文字列は = を含んでよい。handler は最初の = で分ける。
  [
    "= を含む文字列は足す",
    "CommandLine",
    "a=b",
    ["TargetUserName=user-a", "CommandLine=a=b"],
  ],
])("欄と文字列の組: %s", (_name, field, term, expected) => {
  const result = withFieldTerm(heldPair, field, term);
  if (expected === undefined) {
    // 足さないときは元の値をそのまま返し、要求の取り直しを起こさない。
    expect(result).toBe(heldPair);
  } else {
    expect(result.fieldContains).toEqual(expected);
  }
});

test.each([
  ["ある組を外す", "TargetUserName=user-a", []],
  ["無い組を外しても残りを変えない", "LogonType=3", ["TargetUserName=user-a"]],
])("欄と文字列の組を外す: %s", (_name, pair, expected) => {
  expect(withoutFieldTerm(heldPair, pair).fieldContains).toEqual(expected);
});

test("完全一致の組は部分一致の組と別に持ち、別に外す", () => {
  const whole = withFieldTerm(heldPair, "TargetUserName", "user-a", true);
  expect(whole.fieldContains).toEqual(["TargetUserName=user-a"]);
  expect(whole.fieldEquals).toEqual(["TargetUserName=user-a"]);
  expect(withFieldTerm(whole, "TargetUserName", "user-a", true)).toBe(whole);
  const removed = withoutFieldTerm(whole, "TargetUserName=user-a", true);
  expect(removed.fieldEquals).toEqual([]);
  expect(removed.fieldContains).toEqual(["TargetUserName=user-a"]);
});

test("欄の指定を外しても、欄と文字列の組は残る", () => {
  expect(
    withoutField({ ...heldPair, field: "CommandLine" }).fieldContains,
  ).toEqual(["TargetUserName=user-a"]);
});

test("欄の指定は前後の空白を外して足し、同じ欄では元の値を返し、空の文字列と外す操作で消す", () => {
  const terms = { contains: ["example"], excludes: [] };
  const withCommandLine = withField(terms, " CommandLine ");
  expect(withCommandLine).toEqual({ ...terms, field: "CommandLine" });
  expect(withField(withCommandLine, "CommandLine")).toBe(withCommandLine);
  expect(withField(withCommandLine, "  ")).toEqual(terms);
  expect(withoutField(withCommandLine)).toEqual(terms);
  expect(withoutField(terms)).toBe(terms);
});

test("文字列を前後の空白を外して、指定した向きの末尾に足す", () => {
  const terms = withTerm(
    withTerm(noSearchTerms, "contains", "  ssh.exe "),
    "contains",
    "id_rsa",
  );

  expect(terms).toEqual({ contains: ["ssh.exe", "id_rsa"], excludes: [] });
});

test("空の文字列と、同じ向きに既にある文字列を足さず、元の値を返す", () => {
  const terms = withTerm(noSearchTerms, "excludes", "dcon");

  expect(withTerm(terms, "excludes", "   ")).toBe(terms);
  expect(withTerm(terms, "excludes", "dcon")).toBe(terms);
});

test("反対の向きにある同じ文字列は、足すときに外す", () => {
  const contains = withTerm(noSearchTerms, "contains", "powershell");

  const excluded = withTerm(contains, "excludes", "powershell");

  expect(excluded).toEqual({ contains: [], excludes: ["powershell"] });
});

test("大文字と小文字だけが違う文字列を同じ文字列として扱う", () => {
  const contains = withTerm(noSearchTerms, "contains", "PowerShell");

  expect(withTerm(contains, "contains", "powershell")).toBe(contains);
  expect(withTerm(contains, "excludes", "POWERSHELL")).toEqual({
    contains: [],
    excludes: ["POWERSHELL"],
  });
});

test("検索式は前後の空白を外して適用し、他の条件を残す", () => {
  const terms: SearchTerms = { ...heldPair, contains: ["example"] };

  expect(withExpression(terms, "  LogonType == 3 ")).toEqual({
    ...terms,
    expression: "LogonType == 3",
  });
});

test("空白だけの検索式と、適用している式と同じ式は、元の値を返す", () => {
  const applied = withExpression(noSearchTerms, "LogonType == 3");

  expect(withExpression(noSearchTerms, " \t ")).toBe(noSearchTerms);
  expect(withExpression(applied, " LogonType == 3 ")).toBe(applied);
  expect(withExpression(applied, "  ")).toBe(applied);
});

test("別の検索式を適用すると、前の式を置き換える", () => {
  const applied = withExpression(noSearchTerms, "LogonType == 3");

  expect(withExpression(applied, "LogonType == 10").expression).toBe(
    "LogonType == 10",
  );
});

test("検索式を外すと他の条件を残し、式が無いときは元の値を返す", () => {
  const terms: SearchTerms = { ...heldPair, expression: "LogonType == 3" };

  const removed = withoutExpression(terms);

  expect(removed).toEqual(heldPair);
  expect("expression" in removed).toBe(false);
  expect(withoutExpression(heldPair)).toBe(heldPair);
});

test("文字列を外すと、同じ向きの他の文字列を残す", () => {
  const terms = withTerm(
    withTerm(withTerm(noSearchTerms, "contains", "code"), "contains", "tunnel"),
    "excludes",
    "code",
  );

  expect(terms).toEqual({ contains: ["tunnel"], excludes: ["code"] });
  expect(withoutTerm(terms, "contains", "tunnel")).toEqual({
    contains: [],
    excludes: ["code"],
  });
  expect(withoutTerm(terms, "excludes", "absent")).toEqual(terms);
});

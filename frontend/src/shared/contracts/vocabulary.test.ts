import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { expect, test } from "vitest";
import { conditionKeys } from "./candidates";
import { edgeKinds } from "./graph";
import {
  logonSessionRejectionReasons,
  relationDerivationBases,
  relationDerivationOutcomes,
} from "./graphDetail";

/**
 * backend の宣言から、語彙の値を宣言した順で読む。
 *
 * **定義元は Go の型宣言である。** 画面の語彙が定義元から外れると、`requireEnum` が応答を `DecodeFailure` で
 * 退け、その値を含む応答を読む画面が動作しない。値を足した変更でこの検査が失敗する形にする。
 */
function goEnumValues(relativePath: string, typeName: string): string[] {
  const source = readFileSync(
    fileURLToPath(
      new URL(`../../../../backend/${relativePath}`, import.meta.url),
    ),
    "utf8",
  );
  const declaration = new RegExp(`\\s${typeName}\\s*=\\s*"([^"]+)"`, "g");
  const values: string[] = [];
  for (const match of source.matchAll(declaration)) {
    values.push(match[1]);
  }
  if (values.length === 0) {
    throw new Error(`no value of ${typeName} was found in ${relativePath}`);
  }
  return values;
}

test("関係を導いた結果の分類が backend の宣言と揃う", () => {
  const declared = goEnumValues(
    "core/relation_derivation.go",
    "RelationDerivationOutcome",
  );
  expect([...relationDerivationOutcomes].sort()).toEqual([...declared].sort());
});

test("分類の区分が backend の宣言と揃う", () => {
  const declared = goEnumValues(
    "core/relation_derivation.go",
    "RelationDerivationBasis",
  );
  expect([...relationDerivationBases].sort()).toEqual([...declared].sort());
});

test("関係の種別が backend の宣言と揃う", () => {
  const declared = goEnumValues("core/graph.go", "EdgeKind");
  expect([...edgeKinds].sort()).toEqual([...declared].sort());
});

test("ログオンと操作を関係にしなかった理由が backend の宣言と揃う", () => {
  const declared = goEnumValues(
    "core/logon_session.go",
    "LogonSessionRejectionReason",
  );
  expect([...logonSessionRejectionReasons].sort()).toEqual(
    [...declared].sort(),
  );
});

test("関連付けの条件の種別が backend の宣言と揃う", () => {
  const declared = goEnumValues("core/match_condition.go", "ConditionKey");
  expect([...conditionKeys].sort()).toEqual([...declared].sort());
});

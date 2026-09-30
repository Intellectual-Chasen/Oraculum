import {
  DecodeFailure,
  type Decoder,
  optionalArray,
  readObject,
  requireCount,
  requireString,
} from "./decoding";

/**
 * 案件の識別子。取り込みを求める側が収集元に付けた区分である。
 * 定義元は `backend/core/case.go` の `ValidateCaseId` である。
 */
export type CaseId = string & { readonly __brand: "CaseId" };

/** 案件の識別子の文字列。英数字と `.`、`_`、`-` の 1 文字以上 64 文字以下である。 */
const caseIdPattern = /^[A-Za-z0-9._-]{1,64}$/;

/** 文字列が案件の識別子の規則に合うかを判定する。 */
export function isCaseId(value: string): value is CaseId {
  return caseIdPattern.test(value);
}

/** 案件の識別子を検証する。 */
export const decodeCaseId: Decoder<CaseId> = (input, path) => {
  if (typeof input !== "string" || !isCaseId(input)) {
    throw new DecodeFailure(
      path,
      "expected 1 to 64 letters, digits, '.', '_' or '-'",
    );
  }
  return input;
};

/**
 * 案件 1 つに属する根拠のレコードの件数。
 * 定義元は `backend/core/graph_response.go` の `CaseEvidenceCount` である。
 */
export type CaseEvidenceCount = {
  caseId: CaseId;
  /** その案件の収集元から来た根拠のレコードの件数。1 以上である。 */
  evidenceCount: number;
};

/** `CaseEvidenceCount` を検証する。 */
export const decodeCaseEvidenceCount: Decoder<CaseEvidenceCount> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const evidenceCount = requireCount(source, "evidenceCount", path);
  if (evidenceCount < 1) {
    throw new DecodeFailure(`${path}.evidenceCount`, "expected 1 or more");
  }
  return {
    caseId: decodeCaseId(
      requireString(source, "caseId", path),
      `${path}.caseId`,
    ),
    evidenceCount,
  };
};

/**
 * 根拠の件数を案件ごとに分けた `evidenceByCase` を読む。項目が無いときは `undefined` を返す。
 *
 * 並びは案件の識別子の昇順で、件数の和は全体の件数 `total` と等しい
 * (`backend/core/graph_response.go` の `validateEvidenceByCase`)。
 * **項目が無いことを 0 件の集合として読まない。** 項目が無い応答は、取り込みが案件を
 * 区別していない。
 */
export function optionalEvidenceByCase(
  source: Record<string, unknown>,
  path: string,
  total: number,
): CaseEvidenceCount[] | undefined {
  const counts = optionalArray(
    source,
    "evidenceByCase",
    path,
    decodeCaseEvidenceCount,
  );
  if (counts === undefined) {
    return undefined;
  }
  counts.forEach((count, index) => {
    const previous = counts[index - 1];
    if (previous !== undefined && previous.caseId >= count.caseId) {
      throw new DecodeFailure(
        `${path}.evidenceByCase[${index}].caseId`,
        "expected case ids in strictly ascending order",
      );
    }
  });
  const sum = counts.reduce(
    (running, count) => running + count.evidenceCount,
    0,
  );
  if (sum !== total) {
    throw new DecodeFailure(
      `${path}.evidenceByCase`,
      `expected the counts to add up to ${total}`,
    );
  }
  return counts;
}

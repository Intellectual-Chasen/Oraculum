/** 検索の文字列の条件。`contains` はどれも含む、`excludes` はどれも含まないことを求める。 */
export type SearchTerms = {
  contains: readonly string[];
  excludes: readonly string[];
  /**
   * 文字列を照合する欄。語彙の項目か原資料の key である。出ない場合は全欄を照合する。文字列が 1 つも
   * 無い間は要求に載せない。
   */
  field?: string;
  /**
   * 欄と文字列の組。`欄=文字列` の文字列で、どの組もその欄に文字列を含むことを求める。`field` の指定は
   * 組に適用しない (`backend/api/graph_request.go` の fieldContains)。
   */
  fieldContains?: readonly string[];
  /**
   * 完全一致の欄と文字列の組。`欄=文字列` の文字列で、どの組もその欄の値の全体が文字列と
   * 等しいことを求める (`backend/api/graph_request.go` の fieldEquals)。
   */
  fieldEquals?: readonly string[];
  /**
   * 適用した検索式。欄・演算子・論理・括弧で書いた文字列であり、グラフの探索と時系列の要求が
   * 同じ文字列を含む。出ない場合は式で絞らない。空白だけの式を持たない。
   */
  expression?: string;
};

/** 文字列の条件の向き。 */
export type SearchTermKind = "contains" | "excludes";

/** 文字列の条件を持たない状態。描画ごとに別の値を作らない。 */
export const noSearchTerms: SearchTerms = { contains: [], excludes: [] };

/**
 * 文字列を 1 つ条件に足した値を返す。
 *
 * **前後の空白を外し、空の文字列と同じ向きに既にある文字列は足さない。** handler は空の文字列を
 * 退け、同じ文字列を 2 回与えても結果は変わらない。足さないときは元の値をそのまま返し、
 * 要求の取り直しを起こさない。
 *
 * **反対の向きにある同じ文字列は外す。** 同じ文字列を含み、かつ含まない条件は必ず 0 件になる。
 *
 * handler は大文字と小文字を区別せずに比べるため、大小だけが違う文字列も同じ文字列として扱う。
 */
export function withTerm(
  terms: SearchTerms,
  kind: SearchTermKind,
  text: string,
): SearchTerms {
  const token = text.trim();
  const folded = token.toLowerCase();
  const isSame = (existing: string) => existing.toLowerCase() === folded;
  if (token === "" || terms[kind].some(isSame)) {
    return terms;
  }
  const opposite: SearchTermKind =
    kind === "contains" ? "excludes" : "contains";
  return {
    ...terms,
    [kind]: [...terms[kind], token],
    [opposite]: terms[opposite].filter((existing) => !isSame(existing)),
  };
}

/** 文字列を照合する欄を指定した値を返す。前後の空白を外し、空の文字列は欄の指定を外す。 */
export function withField(terms: SearchTerms, text: string): SearchTerms {
  const field = text.trim();
  if (field === "") {
    return withoutField(terms);
  }
  return field === terms.field ? terms : { ...terms, field };
}

/** 欄の指定を外した値を返す。指定が無いときは元の値をそのまま返す。 */
export function withoutField(terms: SearchTerms): SearchTerms {
  if (terms.field === undefined) {
    return terms;
  }
  const { field: _field, ...rest } = terms;
  return rest;
}

/**
 * 欄と文字列の組を 1 つ足した値を返す。前後の空白を外し、欄か文字列が空の組と、欄が `=` を含む組と、
 * 既にある組は足さない。足さないときは元の値をそのまま返す。handler は最初の `=` で欄と文字列を分ける。
 * `wholeValue` が真の組は完全一致の組 (`fieldEquals`) に足す。
 */
export function withFieldTerm(
  terms: SearchTerms,
  fieldText: string,
  termText: string,
  wholeValue = false,
): SearchTerms {
  const key = wholeValue ? "fieldEquals" : "fieldContains";
  const field = fieldText.trim();
  const term = termText.trim();
  const pair = `${field}=${term}`;
  const held = terms[key] ?? [];
  if (
    field === "" ||
    field.includes("=") ||
    term === "" ||
    held.includes(pair)
  ) {
    return terms;
  }
  return { ...terms, [key]: [...held, pair] };
}

/** 欄と文字列の組を 1 つ外した値を返す。`wholeValue` が真のときは完全一致の組から外す。 */
export function withoutFieldTerm(
  terms: SearchTerms,
  pair: string,
  wholeValue = false,
): SearchTerms {
  const key = wholeValue ? "fieldEquals" : "fieldContains";
  return {
    ...terms,
    [key]: (terms[key] ?? []).filter((held) => held !== pair),
  };
}

/**
 * 検索式を適用した値を返す。前後の空白を外した文字列を持つ。
 *
 * **空白だけの式と、適用している式と同じ式は、元の値をそのまま返す。** handler は空の文字列を
 * 退け、同じ式を与え直しても結果は変わらない。元の値を返すと要求の取り直しが起きない。
 */
export function withExpression(terms: SearchTerms, text: string): SearchTerms {
  const expression = text.trim();
  if (expression === "" || expression === terms.expression) {
    return terms;
  }
  return { ...terms, expression };
}

/** 検索式を外した値を返す。式が無いときは元の値をそのまま返す。 */
export function withoutExpression(terms: SearchTerms): SearchTerms {
  if (terms.expression === undefined) {
    return terms;
  }
  const { expression: _expression, ...rest } = terms;
  return rest;
}

/** 文字列を 1 つ条件から外した値を返す。 */
export function withoutTerm(
  terms: SearchTerms,
  kind: SearchTermKind,
  text: string,
): SearchTerms {
  return { ...terms, [kind]: terms[kind].filter((token) => token !== text) };
}

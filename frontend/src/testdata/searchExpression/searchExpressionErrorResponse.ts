/** 検索式の誤り 1 件。`offset` と `length` は式の先頭から数えた code point の個数である。 */
export type SearchExpressionErrorJson = {
  reason: string;
  offset: number;
  length: number;
};

/**
 * 検索式を読めなかった失敗の応答の本体 (HTTP 400)。
 * `message` は英文であり、画面は読まない。
 */
export function searchExpressionErrorResponseJson(
  error: SearchExpressionErrorJson,
): unknown {
  return {
    code: "invalid_request",
    message: "the search expression could not be read",
    searchExpressionError: error,
  };
}

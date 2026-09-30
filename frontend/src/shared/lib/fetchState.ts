/** 取得の失敗の分類。利用者に見せる文言と次に行える操作を、分類ごとに決める。 */
export type FetchFailureKind =
  | "network"
  | "authorization"
  | "request_rejected"
  | "rate_limited"
  | "server"
  | "response_unreadable"
  | "unexpected";

/** `ApiError` を持たない失敗の、分類ごとの失敗の種類の短いラベル。 */
export const fetchFailureKindLabels: Record<FetchFailureKind, string> = {
  network: "通信の失敗",
  authorization: "権限なし",
  request_rejected: "指定の誤り",
  rate_limited: "回数の制限",
  server: "サーバーの失敗",
  response_unreadable: "サーバーと画面のバージョンの不一致",
  unexpected: "画面の失敗",
};

/**
 * 取得の失敗 1 件。画面は失敗した操作の名前と、失敗の種類の短いラベルを出し、ほかを
 * 詳細の「名前: 値」の組にする。文字列の値はどれも文にせず、短い名詞か操作の名前で書く。
 */
export type FetchFailure = {
  kind: FetchFailureKind;
  /** 失敗した操作の短い名前 (例「レコードの取得」)。 */
  summary: string;
  /** 次に行う操作の短いラベル (例「時間を空けて再実行」)。 */
  nextAction: string;
  /** 根拠の追跡に使う識別子。通信できた応答が返した `ApiError` の `code` を入れる。 */
  failureCode?: string;
  /** 応答が返した `code` が表す失敗の種類の短いラベル。 */
  failureDescription?: string;
  /** 失敗に関わる収集元の取り込み 1 件。 */
  sourceId?: string;
  /** 失敗に関わる収集元の内容の識別。 */
  sourceContentSha256?: string;
  /** 失敗に関わる収集元の path。 */
  originPath?: string;
  /** 要求を退けた理由の短いラベル。 */
  rejectionDescription?: string;
  /** 要求が与えた検索式を読めなかった誤り。 */
  searchExpressionError?: SearchExpressionFailure;
  /** 応答が含む競合の記録。検証していない JSON のままである。 */
  conflict?: unknown;
};

/**
 * 検索式を読めなかった誤り 1 件。位置は、要求が与えた式の先頭から数えた Unicode の
 * code point の個数で表す (JavaScript では `Array.from(式)` の添字)。
 */
export type SearchExpressionFailure = {
  /** 応答が返した誤りの種別の文字列。根拠の追跡に使う。 */
  reason: string;
  /** 誤りの範囲の始まり。0 から数える。 */
  offset: number;
  /** 誤りの範囲の長さ。0 は、`offset` の位置に文字列が足りないことを表す。 */
  length: number;
  /** 誤りの理由を日本語で書く。 */
  description: string;
};

/** 取得の 4 状態。読み込み中・取得失敗・結果なし・成功を区別する。 */
export type FetchState<T> =
  | { status: "loading" }
  | { status: "failed"; failure: FetchFailure }
  | { status: "empty"; description: string }
  | { status: "loaded"; value: T };

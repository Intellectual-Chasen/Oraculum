import { DecodeFailure, type Decoder } from "../contracts/decoding";
import type { FetchFailure } from "../lib/fetchState";
import {
  type ApiError,
  buildFetchFailure,
  classifyHttpStatus,
  decodeApiError,
} from "./apiFailure";
import { readApiBaseUrl } from "./config";

/** 取得の結果。成功した値と、分類済みの失敗のいずれかを持つ。 */
export type ApiResult<T> =
  | { ok: true; value: T }
  | { ok: false; failure: FetchFailure };

/** 要求の method。server の状態を変える操作が GET の外を使う。 */
export type ApiMethod = "GET" | "POST" | "PUT" | "DELETE";

const authenticationRequiredListeners = new Set<
  (failure: FetchFailure) => void
>();

/**
 * 要求が `authentication_required` の失敗を受け取るたびに、その失敗を渡して `listener` を呼ぶ。
 * セッションの期限切れや失効を、要求を送った画面の外 (ログインの画面を出す側) へ知らせる。
 * 返す関数を呼ぶと登録を外す。
 */
export function onAuthenticationRequired(
  listener: (failure: FetchFailure) => void,
): () => void {
  authenticationRequiredListeners.add(listener);
  return () => {
    authenticationRequiredListeners.delete(listener);
  };
}

const roleRejectedListeners = new Set<(failure: FetchFailure) => void>();

/**
 * 要求が `permission_denied` か `investigation_not_found` の失敗を受け取るたびに、その失敗を
 * 渡して `listener` を呼ぶ。ログインの途中で役割が変わったことを、役割を持つ側へ知らせる。
 * 返す関数を呼ぶと登録を外す。
 */
export function onRoleRejected(
  listener: (failure: FetchFailure) => void,
): () => void {
  roleRejectedListeners.add(listener);
  return () => {
    roleRejectedListeners.delete(listener);
  };
}

/** 1 回の取得に与える条件。 */
export type ApiRequest<T> = {
  /** 接続先の後ろに付ける path。先頭の `/` を含める。 */
  path: string;
  /** 省略したときは `GET` を送る。 */
  method?: ApiMethod;
  /** 本文に載せる組。省略したときは本文を送らない。 */
  body?: unknown;
  /** ファイルを JSON に変換せず送る本文。 */
  rawBody?: Blob;
  /**
   * query の項目。値が `undefined` の項目を送らない。
   * 集合を持つ項目には配列を渡す。同じ名前の項目を要素の個数だけ繰り返して送る。
   */
  searchParams?: Record<string, string | string[] | undefined>;
  /** 応答の本体を画面が扱う型へ変換する。 */
  decode: Decoder<T>;
  /** 失敗したときに画面へ出す 1 文。 */
  failureSummary: string;
  signal?: AbortSignal;
  /** path の前に付ける接続先。省略したときは backend の接続先を使う。 */
  baseUrl?: string;
  /** 真のとき、ページを離れた後も要求を送り続ける。 */
  keepalive?: boolean;
};

function buildUrl(request: ApiRequest<unknown>): string {
  const query = new URLSearchParams();
  for (const [key, value] of Object.entries(request.searchParams ?? {})) {
    if (value === undefined) {
      continue;
    }
    if (Array.isArray(value)) {
      for (const element of value) {
        query.append(key, element);
      }
      continue;
    }
    query.set(key, value);
  }
  const suffix = query.size === 0 ? "" : `?${query.toString()}`;
  return `${request.baseUrl ?? readApiBaseUrl()}${request.path}${suffix}`;
}

async function readApiError(response: Response): Promise<ApiError | undefined> {
  let body: unknown;
  try {
    body = await response.json();
  } catch {
    // 失敗の応答が JSON を含まない場合は HTTP の status だけで分類する。
    return undefined;
  }
  try {
    return decodeApiError(body, "error");
  } catch (cause) {
    if (cause instanceof DecodeFailure) {
      // `ApiError` の形と異なる本体は読まず、HTTP の status だけで分類する。
      return undefined;
    }
    throw cause;
  }
}

/** 要求を送り、成功の status の応答を返す。失敗は分類して返す。 */
async function send<T>(
  request: ApiRequest<T>,
  accept: string,
): Promise<ApiResult<Response>> {
  let response: Response;
  const headers: Record<string, string> = { Accept: accept };
  if (request.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (request.rawBody !== undefined)
    headers["Content-Type"] = "application/octet-stream";
  try {
    response = await fetch(buildUrl(request), {
      method: request.method ?? "GET",
      headers,
      body:
        request.rawBody ??
        (request.body === undefined ? undefined : JSON.stringify(request.body)),
      signal: request.signal,
      keepalive: request.keepalive,
    });
  } catch {
    return {
      ok: false,
      failure: buildFetchFailure("network", request.failureSummary),
    };
  }

  if (!response.ok) {
    const apiError = await readApiError(response);
    const failure = buildFetchFailure(
      classifyHttpStatus(response.status),
      request.failureSummary,
      apiError,
    );
    if (apiError?.code === "authentication_required") {
      // ponytail: 要求を送った時点のセッションを区別しない。古い要求の 401 もログインの form を
      // 出す。区別が必要になったらセッションの世代番号を要求に記録する。
      for (const listener of authenticationRequiredListeners) {
        listener(failure);
      }
    }
    if (
      apiError?.code === "permission_denied" ||
      apiError?.code === "investigation_not_found"
    ) {
      for (const listener of roleRejectedListeners) {
        listener(failure);
      }
    }
    return { ok: false, failure };
  }
  return { ok: true, value: response };
}

/** 本体 1 つを変換する。変換できない本体は `response_unreadable` の失敗にする。 */
function decodeBody<T>(request: ApiRequest<T>, body: unknown): ApiResult<T> {
  try {
    return { ok: true, value: request.decode(body, "response") };
  } catch (cause) {
    if (cause instanceof DecodeFailure) {
      return {
        ok: false,
        failure: buildFetchFailure(
          "response_unreadable",
          request.failureSummary,
        ),
      };
    }
    throw cause;
  }
}

/**
 * HTTP で取得し、応答を画面が扱う型へ変換する。
 * 通信を開始する箇所を本 module に集約する。component 本体から `fetch` を呼ばない。
 */
export async function requestJson<T>(
  request: ApiRequest<T>,
): Promise<ApiResult<T>> {
  const sent = await send(request, "application/json");
  if (!sent.ok) {
    return sent;
  }
  let body: unknown;
  try {
    // 204 は本文を持たない。`decode` に `undefined` を渡す。
    body = sent.value.status === 204 ? undefined : await sent.value.json();
  } catch {
    return {
      ok: false,
      failure: buildFetchFailure("response_unreadable", request.failureSummary),
    };
  }
  return decodeBody(request, body);
}

/**
 * HTTP で取得し、改行で区切った JSON の応答を 1 行ずつ変換して `onItem` へ渡す。
 * 応答の終わりまで読んだら成功を返す。途中で読めない行に達したら、その行より前の行を
 * 渡したまま失敗を返す。
 */
export async function requestNdjson<T>(
  request: ApiRequest<T>,
  onItem: (item: T) => void,
): Promise<ApiResult<void>> {
  const sent = await send(request, "application/x-ndjson");
  if (!sent.ok) {
    return sent;
  }
  const reader = sent.value.body?.getReader();
  if (reader === undefined) {
    return {
      ok: false,
      failure: buildFetchFailure("response_unreadable", request.failureSummary),
    };
  }
  const decoder = new TextDecoder();
  let buffer = "";
  const readLine = (line: string): ApiResult<void> => {
    if (line.trim() === "") {
      return { ok: true, value: undefined };
    }
    let body: unknown;
    try {
      body = JSON.parse(line);
    } catch {
      return {
        ok: false,
        failure: buildFetchFailure(
          "response_unreadable",
          request.failureSummary,
        ),
      };
    }
    const decoded = decodeBody(request, body);
    if (!decoded.ok) {
      return decoded;
    }
    onItem(decoded.value);
    return { ok: true, value: undefined };
  };
  for (;;) {
    let chunk: ReadableStreamReadResult<Uint8Array>;
    try {
      chunk = await reader.read();
    } catch {
      return {
        ok: false,
        failure: buildFetchFailure("network", request.failureSummary),
      };
    }
    buffer += decoder.decode(chunk.value, { stream: !chunk.done });
    let newline = buffer.indexOf("\n");
    while (newline >= 0) {
      const read = readLine(buffer.slice(0, newline));
      if (!read.ok) {
        // 読めない行の後を読まない。応答を閉じ、同じ origin の接続を空ける。
        void reader.cancel().catch(() => {});
        return read;
      }
      buffer = buffer.slice(newline + 1);
      newline = buffer.indexOf("\n");
    }
    if (chunk.done) {
      return readLine(buffer);
    }
  }
}

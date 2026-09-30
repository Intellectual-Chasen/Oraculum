import {
  decodeTerminalDetail,
  decodeTerminalEventsResponse,
  decodeTerminalsResponse,
  type TerminalCategory,
  type TerminalDetail,
  type TerminalEventsResponse,
  type TerminalsResponse,
} from "../contracts/terminals";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

type Options = { signal?: AbortSignal };

/** 端末の一覧を取る (`GET /api/v0/terminals`)。 */
export function fetchTerminals(
  matchConditions: MatchConditionSelection,
  options: Options = {},
): Promise<ApiResult<TerminalsResponse>> {
  return requestJson({
    path: "/terminals",
    searchParams: { matchCondition: matchConditionParams(matchConditions) },
    decode: decodeTerminalsResponse,
    failureSummary: "端末の一覧の取得",
    signal: options.signal,
  });
}

/** 端末 1 台の情報を取る (`GET /api/v0/terminals/{id}`)。 */
export function fetchTerminalDetail(
  terminalId: string,
  matchConditions: MatchConditionSelection,
  options: Options = {},
): Promise<ApiResult<TerminalDetail>> {
  return requestJson({
    path: `/terminals/${encodeURIComponent(terminalId)}`,
    searchParams: { matchCondition: matchConditionParams(matchConditions) },
    decode: decodeTerminalDetail,
    failureSummary: "端末の情報の取得",
    signal: options.signal,
  });
}

/** 端末のレコードを絞る条件。両方を省略すると、いずれかの分類に入る全レコードを返す。 */
export type TerminalEventFilter = {
  category?: TerminalCategory;
  sourceIp?: string;
};

/** 端末の分類に入るレコードを時刻の昇順に取る (`GET /api/v0/terminals/{id}/events`)。 */
export function fetchTerminalEvents(
  terminalId: string,
  filter: TerminalEventFilter,
  matchConditions: MatchConditionSelection,
  options: Options = {},
): Promise<ApiResult<TerminalEventsResponse>> {
  return requestJson({
    path: `/terminals/${encodeURIComponent(terminalId)}/events`,
    searchParams: {
      matchCondition: matchConditionParams(matchConditions),
      category: filter.category,
      sourceIp: filter.sourceIp,
    },
    decode: decodeTerminalEventsResponse,
    failureSummary: "端末のレコードの取得",
    signal: options.signal,
  });
}

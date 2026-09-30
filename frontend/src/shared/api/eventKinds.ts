import type { CaseId } from "../contracts/cases";
import {
  decodeEventKindsResponse,
  type EventKindsResponse,
} from "../contracts/eventKinds";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

/** 事象の種別の一覧の要求の項目。 */
export type EventKindsRequest = {
  /** 候補を絞るのに用いる条件の選択。要求が必ず含む。 */
  matchConditions: MatchConditionSelection;
  /** 数えるレコードを絞る案件。出ない場合はすべての案件を読む。 */
  caseId?: CaseId;
  /** 数えるレコードを絞る端末のノードの識別子。出ない場合は端末で絞らない。 */
  terminal?: string;
  /** 数えるレコードを絞る収集元。出ない場合は収集元で絞らない。 */
  source?: { sourceId: string; contentSha256: string };
};

/** 事象の種別の一覧の操作 (`GET /api/v0/event-kinds`) を実行する。 */
export function fetchEventKinds(
  request: EventKindsRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<EventKindsResponse>> {
  return requestJson({
    path: "/event-kinds",
    searchParams: {
      case: request.caseId,
      terminal: request.terminal,
      sourceId: request.source?.sourceId,
      sourceContentSha256: request.source?.contentSha256,
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeEventKindsResponse,
    failureSummary: "イベントの種類の一覧の取得",
    signal: options.signal,
  });
}

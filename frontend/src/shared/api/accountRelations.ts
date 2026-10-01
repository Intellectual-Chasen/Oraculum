import {
  type AccountRelationKey,
  type AccountRelationsResponse,
  decodeAccountRelationsResponse,
} from "../contracts/accountRelations";
import type { EdgeKind } from "../contracts/graph";
import { nonEmpty, numberParam, type RecordFilterCriteria } from "./graph";
import { type ApiResult, requestJson } from "./httpClient";
import {
  type MatchConditionSelection,
  matchConditionParams,
} from "./matchConditions";

export type AccountRelationsRequest = {
  accountNodeId: string;
  filter: RecordFilterCriteria;
  roles?: readonly EdgeKind[];
  selected?: AccountRelationKey;
  offset?: number;
  matchConditions: MatchConditionSelection;
};

export function fetchAccountRelations(
  request: AccountRelationsRequest,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<AccountRelationsResponse>> {
  const filter = request.filter;
  const time = filter.timeFilter;
  return requestJson({
    path: "/account-relations",
    searchParams: {
      accountNodeId: request.accountNodeId,
      counterpartId: request.selected?.counterpartId,
      originRole: request.selected?.originRole,
      otherRole: request.selected?.otherRole,
      offset: numberParam(request.offset),
      edgeKind: nonEmpty(request.roles),
      eventCategory: filter.eventCategory,
      eventAction: filter.eventAction,
      eventActionFrom: numberParam(filter.eventActionFrom),
      eventActionTo: numberParam(filter.eventActionTo),
      case: filter.caseId,
      terminal: filter.terminal?.id,
      source: nonEmpty(filter.sources?.map((source) => source.id)),
      timeFrom: time?.from?.text,
      timeFromPrecision: time?.from?.precision,
      timeTo: time?.to?.text,
      timeToPrecision: time?.to?.precision,
      filterUnit: time === undefined ? undefined : time.unit,
      matchCondition: matchConditionParams(request.matchConditions),
    },
    decode: decodeAccountRelationsResponse,
    failureSummary: "相手アカウントの取得",
    signal: options.signal,
  });
}

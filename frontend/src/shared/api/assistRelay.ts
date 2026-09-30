import type { AssistProvider } from "../contracts/assistPermissions";
import {
  type AssistEvent,
  type AssistMatchCondition,
  decodeAssistEvent,
  decodeOpenedConversation,
  decodeRelayConversations,
  decodeRelayEvents,
  decodeRelayStatus,
  type RelayConversation,
  type RelayEvents,
  type RelayStatus,
} from "../contracts/assistRelay";
import type { CaseId } from "../contracts/cases";
import type { RecordLocator } from "../contracts/common";
import type { SearchQuery } from "../contracts/searchQuery";
import { type ApiResult, requestJson, requestNdjson } from "./httpClient";
import type { MatchConditionSelection } from "./matchConditions";

/** 関連付けの条件の選択を、発言に添える条件の組にする。幅を付けない条件は幅 0 である。 */
export function assistMatchConditionsOf(
  selection: MatchConditionSelection,
): AssistMatchCondition[] {
  return selection.conditions.map((condition) => ({
    conditionKey: condition.conditionKey,
    tolerance: condition.toleranceSeconds ?? 0,
  }));
}

/**
 * 中継が自分で答える path の接頭辞。中継は画面と同じ origin にあり、画面の build の API の
 * 接続先の設定に依らない。
 */
export const relayBaseUrl = "/assist";

/** 発言に添える画面の文脈。中継が server に登録し、LLM には server が発行した参照が渡る。 */
export type AssistTurnContext = {
  matchConditions: AssistMatchCondition[];
  case?: CaseId;
  /** 画面の検索の条件。`nodeKinds` と `granularity` は、画面の図が出している対象である。 */
  searchQuery?: SearchQuery;
  /** 真のとき、画面は図に出す対象を検索の条件から決めている。 */
  nodeKindsFromConditions?: true;
  records?: RecordLocator[];
  nodeIds?: string[];
  edgeIds?: string[];
};

/** 分析者の発言 1 つ。 */
export type AssistTurnDraft = {
  /** 画面が付ける発言の識別子。英数字と `-` と `_` で 64 文字以下。 */
  turnId: string;
  text: string;
  context: AssistTurnContext;
};

/** 中継の状態 (`GET /assist/status`) を取得する。 */
export async function fetchRelayStatus(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<RelayStatus>> {
  return requestJson({
    baseUrl: relayBaseUrl,
    path: "/status",
    decode: decodeRelayStatus,
    failureSummary: "AI 支援の中継への接続",
    signal: options.signal,
  });
}

/** 中継が持つ会話の一覧 (`GET /assist/conversations`) を取得する。 */
export async function fetchRelayConversations(
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<RelayConversation[]>> {
  return requestJson({
    baseUrl: relayBaseUrl,
    path: "/conversations",
    decode: decodeRelayConversations,
    failureSummary: "AI 支援の会話の一覧の取得",
    signal: options.signal,
  });
}

/** 会話を始める (`POST /assist/conversations`)。server が送信の許可を確かめる。 */
export async function openRelayConversation(
  provider: AssistProvider,
): Promise<ApiResult<RelayConversation>> {
  return requestJson({
    baseUrl: relayBaseUrl,
    path: "/conversations",
    method: "POST",
    body: { provider },
    decode: decodeOpenedConversation,
    failureSummary: "AI 支援の会話の開始",
  });
}

/** 会話の event のうち、通番が after より大きいものを取得する。 */
export async function fetchRelayEvents(
  conversationId: string,
  after: number,
  options: { signal?: AbortSignal } = {},
): Promise<ApiResult<RelayEvents>> {
  return requestJson({
    baseUrl: relayBaseUrl,
    path: `/conversations/${encodeURIComponent(conversationId)}/events`,
    searchParams: { after: String(after) },
    decode: decodeRelayEvents,
    failureSummary: "AI 支援の会話の再読み込み",
    signal: options.signal,
  });
}

/**
 * 発言を送り、応答の event を届いた順に `onEvent` へ渡す。応答の終わりで成功を返す。
 * 接続が途中で切れても、中継は応答を続けて会話の event に残す。
 */
export async function sendRelayTurn(
  conversationId: string,
  draft: AssistTurnDraft,
  onEvent: (event: AssistEvent) => void,
): Promise<ApiResult<void>> {
  return requestNdjson(
    {
      baseUrl: relayBaseUrl,
      path: `/conversations/${encodeURIComponent(conversationId)}/turns`,
      method: "POST",
      body: draft,
      decode: decodeAssistEvent,
      failureSummary: "AI 支援への発言の送信",
    },
    onEvent,
  );
}

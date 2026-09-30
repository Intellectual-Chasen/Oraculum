import { type AssistProvider, assistProviders } from "./assistPermissions";
import { type ConditionKey, conditionKeys } from "./candidates";
import {
  DecodeFailure,
  type Decoder,
  optionalArray,
  optionalBoolean,
  optionalCount,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireBoolean,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";
import { decodeNodeKind, type NodeKind } from "./graph";
import { decodeSearchQuery, type SearchQuery } from "./searchQuery";

/** 中継が名乗る名前。画面は、この名前を持つ状態を読めたときだけ中継が有ると読む。 */
export const relayName = "oraculum-assist";

/** 中継の状態。定義元は `backend/assist/relay.go` の `statusResponse`。 */
export type RelayStatus = {
  relay: typeof relayName;
  /** 中継が起動できる提供者。 */
  providers: AssistProvider[];
};

/** 中継が持つ会話 1 件。定義元は `backend/assist/conversations.go` の `conversationSummary`。 */
export type RelayConversation = {
  id: string;
  provider: AssistProvider;
  eventCount: number;
  /** 応答の途中の発言があること。 */
  answering: boolean;
};

/** 発言の関連付けの条件 1 件と幅。定義元は `backend/core/assist_conversation.go` の `AssistMatchCondition`。 */
export type AssistMatchCondition = {
  conditionKey: ConditionKey;
  tolerance: number;
};

/** 会話の event の種類。定義元は `backend/core/assist_event.go` の `AssistEventKind`。 */
export const assistEventKinds = [
  "user_message",
  "text",
  "tool_use",
  "tool_result",
  "search_query_card",
  "turn_end",
  "provider_error",
] as const;
export type AssistEventKind = (typeof assistEventKinds)[number];

/**
 * 会話の event 1 つ。定義元は `backend/core/assist_event.go` の `AssistEvent`。
 * **LLM の出力は文字列である。** 画面は応答の文を HTML・画像・リンクを持たない Markdown として描く。
 */
export type AssistEvent = {
  /** 会話の中の通番。1 から始まる。 */
  sequence: number;
  turnId: string;
  kind: AssistEventKind;
  /** 分析者の発言、LLM の応答の文、提供者の失敗の説明。 */
  text?: string;
  /** LLM が呼んだ tool の名前。tool の呼び出しと結果が持つ。 */
  toolName?: string;
  /** tool の呼び出しの入力。JSON の値である。 */
  toolInput?: unknown;
  /** tool の結果が答える呼び出しの event の通番。 */
  toolUseSequence?: number;
  /** LLM に返した tool の結果の本文。 */
  toolResult?: string;
  /** tool の結果が失敗であること。 */
  toolFailed?: boolean;
  /**
   * server が検証を通した、画面の検索欄が表せる条件。card は必ず持ち、tool の結果は画面に
   * 適用できるときだけ持つ。`nodeIds` は LLM の短い参照を解決したノードの識別子である。
   */
  searchQuery?: SearchQuery;
  /** `searchQuery.nodeIds` の各ノードの種別と表示名。`nodeIds` と同じ順に並ぶ。 */
  origins?: AssistOrigin[];
  /** 検索の条件の card に LLM が添えた説明。 */
  explanation?: string;
  /** card を作った発言の関連付けの条件の選択。 */
  matchConditions?: AssistMatchCondition[];
};

/** LLM が検索の起点に指したノード。定義元は `backend/core/assist_event.go` の `AssistOrigin`。 */
export type AssistOrigin = {
  id: string;
  kind: NodeKind;
  /** ノードの表示名。原資料の文字列である。空の文字列を取る。 */
  label: string;
};

/** 会話の event の一覧。定義元は `backend/assist/relay.go` の `events`。 */
export type RelayEvents = {
  events: AssistEvent[];
  /** 応答の途中の発言があること。 */
  answering: boolean;
};

/** 集合の要素として提供者を読む。 */
const decodeAssistProvider: Decoder<AssistProvider> = (input, path) => {
  const found = assistProviders.find((provider) => provider === input);
  if (found === undefined) {
    throw new DecodeFailure(
      path,
      `expected one of ${assistProviders.join(" / ")}`,
    );
  }
  return found;
};

/** `RelayStatus` を検証する。 */
export const decodeRelayStatus: Decoder<RelayStatus> = (input, path) => {
  const source = readObject(input, path);
  return {
    relay: requireEnum(source, "relay", path, [relayName] as const),
    providers: requireArray(source, "providers", path, decodeAssistProvider),
  };
};

/** `RelayConversation` を検証する。 */
export const decodeRelayConversation: Decoder<RelayConversation> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    provider: requireEnum(source, "provider", path, assistProviders),
    eventCount: requireCount(source, "eventCount", path),
    answering: requireBoolean(source, "answering", path),
  };
};

/** 会話の一覧の応答を検証する。 */
export const decodeRelayConversations: Decoder<RelayConversation[]> = (
  input,
  path,
) =>
  requireArray(
    readObject(input, path),
    "conversations",
    path,
    decodeRelayConversation,
  );

/** 会話を始めた応答を検証する。 */
export const decodeOpenedConversation: Decoder<RelayConversation> = (
  input,
  path,
) =>
  requireMember(
    readObject(input, path),
    "conversation",
    path,
    decodeRelayConversation,
  );

const decodeAssistMatchCondition: Decoder<AssistMatchCondition> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    conditionKey: requireEnum(source, "conditionKey", path, conditionKeys),
    tolerance: requireCount(source, "tolerance", path),
  };
};

const decodeAssistOrigin: Decoder<AssistOrigin> = (input, path) => {
  const source = readObject(input, path);
  return {
    id: requireString(source, "id", path),
    kind: requireMember(source, "kind", path, decodeNodeKind),
    label: requireString(source, "label", path),
  };
};

/** `AssistEvent` を検証する。card は検索の条件を必ず持つ。 */
export const decodeAssistEvent: Decoder<AssistEvent> = (input, path) => {
  const source = readObject(input, path);
  const sequence = requireCount(source, "sequence", path);
  if (sequence < 1) {
    throw new DecodeFailure(`${path}.sequence`, "expected 1 or more");
  }
  const kind = requireEnum(source, "kind", path, assistEventKinds);
  const searchQuery = optionalMember(
    source,
    "searchQuery",
    path,
    decodeSearchQuery,
  );
  if (
    kind !== "tool_result" &&
    (kind === "search_query_card") !== (searchQuery !== undefined)
  ) {
    throw new DecodeFailure(
      `${path}.searchQuery`,
      "expected the search conditions on a card or a tool result only",
    );
  }
  const origins = optionalArray(source, "origins", path, decodeAssistOrigin);
  const nodeIds = searchQuery?.nodeIds ?? [];
  if (
    (origins ?? []).length !== nodeIds.length ||
    origins?.some((origin, index) => origin.id !== nodeIds[index])
  ) {
    throw new DecodeFailure(
      `${path}.origins`,
      "expected one origin for each node of searchQuery.nodeIds in the same order",
    );
  }
  return {
    sequence,
    turnId: requireString(source, "turnId", path),
    kind,
    text: optionalString(source, "text", path),
    toolName: optionalString(source, "toolName", path),
    toolInput: source.toolInput,
    toolUseSequence: optionalCount(source, "toolUseSequence", path),
    toolResult: optionalString(source, "toolResult", path),
    toolFailed: optionalBoolean(source, "toolFailed", path),
    searchQuery,
    origins,
    explanation: optionalString(source, "explanation", path),
    matchConditions: optionalArray(
      source,
      "matchConditions",
      path,
      decodeAssistMatchCondition,
    ),
  };
};

/** `RelayEvents` を検証する。 */
export const decodeRelayEvents: Decoder<RelayEvents> = (input, path) => {
  const source = readObject(input, path);
  return {
    events: requireArray(source, "events", path, decodeAssistEvent),
    answering: requireBoolean(source, "answering", path),
  };
};

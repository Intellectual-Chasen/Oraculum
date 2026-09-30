import {
  MessageSquarePlus,
  RefreshCw,
  Send,
  Undo2,
  Wrench,
} from "lucide-react";
import {
  type FormEvent,
  type ReactNode,
  useEffect,
  useRef,
  useState,
} from "react";
import type { AssistTurnContext } from "@/shared/api/assistRelay";
import type { NodeRef } from "@/shared/api/graph";
import type {
  AssistEvent,
  AssistMatchCondition,
  AssistOrigin,
} from "@/shared/contracts/assistRelay";
import type { SearchQuery } from "@/shared/contracts/searchQuery";
import type { FetchState } from "@/shared/lib/fetchState";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { AssistMarkdown, AssistText } from "./AssistText";
import { ApplyActions, SearchQueryCard } from "./SearchQueryCard";
import type {
  AssistConversationView,
  RelayState,
} from "./useAssistConversation";

/** 中継が提供者へ見せる tool の名前の接頭辞。画面は tool の名前だけを出す。 */
const toolNamePrefix = "mcp__oraculum__";

type AssistConversationProps = {
  view: AssistConversationView;
  /** 発言に添える画面の文脈。送る操作の時点の値を読む。 */
  turnContext: () => AssistTurnContext;
  /** 今の画面の関連付けの条件の選択。card を作った時点の選択と比べる。 */
  currentMatchConditions: readonly AssistMatchCondition[];
  /** card の端末に表示名を付ける端末の選択肢。 */
  terminals: FetchState<NodeRef[]>;
  /**
   * card の条件を検索の条件に適用し、適用する前の条件へ戻す操作を返す。origins は条件の
   * `nodeIds` の種別と表示名である。
   */
  onApplySearchQuery: (
    query: SearchQuery,
    origins: readonly AssistOrigin[] | undefined,
  ) => () => void;
};

/** 中継の状態の値の組と、中継を再確認する操作。 */
function RelayNotice({
  relay,
  onRecheck,
}: {
  relay: RelayState;
  onRecheck: () => void;
}) {
  const recheck = (
    <IconButton label="中継を再確認" onPress={onRecheck}>
      <RefreshCw size={14} aria-hidden="true" />
    </IconButton>
  );
  const state = (value: ReactNode) => (
    <KeyValueList pairs={[{ name: "中継", value }]} />
  );
  switch (relay.status) {
    case "checking":
      return (
        <div role="status">
          {state(<StatusLabel status="running" label="確認中" />)}
        </div>
      );
    case "absent":
      return (
        <div role="note" className="flex items-center gap-1">
          {state(<StatusLabel status="idle" label="未接続" />)}
          <HelpPopover label="中継の起動">
            <KeyValueList
              stacked
              pairs={[
                {
                  name: "分析者の端末",
                  value: (
                    <code>
                      oraculum-assist --server &lt;Oraculum の URL&gt;
                    </code>
                  ),
                },
                { name: "画面", value: "中継が表示した URL" },
              ]}
            />
          </HelpPopover>
          {recheck}
        </div>
      );
    case "disconnected":
      return (
        <div role="alert" className="flex items-center gap-1">
          {state(<StatusLabel status="failed" label="切断" />)}
          {recheck}
        </div>
      );
    case "available":
      return null;
  }
}

/** JSON の値を字下げした文字列にする。文字列は JSON として読めるときだけ字下げする。 */
function formatJson(value: unknown): string {
  if (typeof value !== "string") return JSON.stringify(value, null, 2);
  try {
    return JSON.stringify(JSON.parse(value), null, 2);
  } catch {
    return value;
  }
}

/**
 * tool の呼び出し 1 つ。名前と状態を出し、開いたときだけ入力と結果を描く。
 * 結果が無いまま応答が終わった呼び出しは「結果なし」と出す。
 */
function ToolCall({
  use,
  result,
  ended,
  actions,
}: {
  use: AssistEvent;
  result?: AssistEvent;
  /** 呼び出しを含む発言への応答が終わった。 */
  ended: boolean;
  /** 結果を画面に適用する操作。適用できない結果は undefined。 */
  actions?: ReactNode;
}) {
  const [open, setOpen] = useState(false);
  const name = use.toolName ?? "";
  const status =
    result === undefined
      ? ended
        ? ({ status: "idle", label: "結果なし" } as const)
        : ({ status: "running", label: "実行中" } as const)
      : result.toolFailed
        ? ({ status: "failed", label: "失敗" } as const)
        : ({ status: "done", label: "完了" } as const);
  return (
    <li className="assist-tool">
      <details onToggle={(event) => setOpen(event.currentTarget.open)}>
        <summary className="flex items-center gap-1">
          <Wrench size={14} aria-hidden="true" />
          <KeyValueList
            pairs={[
              {
                name: "tool",
                value: (
                  <RawText
                    text={
                      name.startsWith(toolNamePrefix)
                        ? name.slice(toolNamePrefix.length)
                        : name
                    }
                  />
                ),
              },
            ]}
          />
          <StatusLabel status={status.status} label={status.label} />
        </summary>
        {open ? (
          <KeyValueList
            stacked
            pairs={[
              {
                name: "入力",
                value: (
                  <div className="assist-tool-body">
                    <AssistText text={formatJson(use.toolInput ?? {})} />
                  </div>
                ),
              },
              {
                name: "結果",
                value:
                  result === undefined ? (
                    status.label
                  ) : (
                    <div className="assist-tool-body">
                      <AssistText text={formatJson(result.toolResult ?? "")} />
                    </div>
                  ),
              },
            ]}
          />
        ) : null}
      </details>
      {actions}
    </li>
  );
}

/** 会話の event 1 つを描く。応答の終わりと tool の結果は描かない。 */
function EventItem({
  event,
  card,
  tool,
}: {
  event: AssistEvent;
  card: (event: AssistEvent) => ReactNode;
  tool: (event: AssistEvent) => ReactNode;
}) {
  switch (event.kind) {
    case "user_message":
      return (
        <li className="assist-message is-analyst">
          <p className="assist-speaker">分析者</p>
          <AssistText text={event.text ?? ""} />
        </li>
      );
    case "text":
      return (
        <li className="assist-message is-ai">
          <p className="assist-speaker">AI</p>
          <AssistMarkdown text={event.text ?? ""} />
        </li>
      );
    case "tool_use":
      return tool(event);
    case "search_query_card":
      return <li className="assist-card">{card(event)}</li>;
    case "tool_result":
      return null;
    case "provider_error":
      return (
        <li className="assist-error" role="alert">
          <AssistText text={event.text ?? "提供者の応答の中断"} />
        </li>
      );
    case "turn_end":
      return null;
  }
}

/**
 * AI 支援との会話の欄。分析者の発言、AI の応答、AI が勧めた検索の条件の card を並べる。
 *
 * **会話を文字列として描く。** 発言は送る操作の event handler からだけ送る。
 */
export function AssistConversation({
  view,
  turnContext,
  currentMatchConditions,
  terminals,
  onApplySearchQuery,
}: AssistConversationProps) {
  const [draft, setDraft] = useState("");
  // card ごとの、適用する前の条件へ戻す操作。通番は会話ごとに 1 から始まるため、会話の
  // 識別子と通番の組を key にする。
  const [restores, setRestores] = useState<ReadonlyMap<string, () => void>>(
    new Map(),
  );
  const busy = view.sending || view.answering;
  const canSend =
    view.relay.status === "available" && !busy && draft.trim() !== "";

  // **中継が発言を受け付けた時点で欄を空にする。** 送れなかった発言を分析者が書き直さずに
  // 済むようにし、応答の間に次の発言を書けるようにする。
  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canSend) return;
    const text = draft.trim();
    const clear = () =>
      setDraft((current) => (current.trim() === text ? "" : current));
    void view.send(text, turnContext(), clear).then((accepted) => {
      if (accepted) clear();
    });
  };

  /** card か tool の結果の条件を適用し、適用する前の条件へ戻す操作。 */
  const applyActionsOf = (event: AssistEvent, query: SearchQuery) => {
    const key = `${view.conversationId ?? ""}:${event.sequence}`;
    return {
      restorable: restores.has(key),
      onApply: () => {
        const restore = onApplySearchQuery(query, event.origins);
        // 同じ条件を 2 回適用したときは、最初に適用する前の条件へ戻す。
        setRestores((current) =>
          current.has(key) ? current : new Map(current).set(key, restore),
        );
      },
      onRestore: () => {
        restores.get(key)?.();
        setRestores((current) => {
          const next = new Map(current);
          next.delete(key);
          return next;
        });
      },
    };
  };

  const card = (event: AssistEvent) => {
    const query = event.searchQuery;
    if (query === undefined) return null;
    return (
      <SearchQueryCard
        query={query}
        origins={event.origins}
        explanation={event.explanation}
        matchConditions={event.matchConditions ?? []}
        currentMatchConditions={currentMatchConditions}
        terminals={terminals}
        {...applyActionsOf(event, query)}
      />
    );
  };

  const results = new Map<number, AssistEvent>();
  const endedTurns = new Set<string>();
  for (const event of view.events) {
    if (event.kind === "tool_result" && event.toolUseSequence !== undefined) {
      results.set(event.toolUseSequence, event);
    }
    if (event.kind === "turn_end" || event.kind === "provider_error") {
      endedTurns.add(event.turnId);
    }
  }
  const tool = (event: AssistEvent) => {
    const result = results.get(event.sequence);
    const query = result?.searchQuery;
    return (
      <ToolCall
        use={event}
        result={result}
        ended={endedTurns.has(event.turnId)}
        actions={
          result === undefined || query === undefined ? undefined : (
            <ApplyActions {...applyActionsOf(result, query)} />
          )
        }
      />
    );
  };

  // 自動で適用した event の位置。通番は会話ごとに 1 から始まるため、会話の識別子と組にする。
  const [autoApply, setAutoApply] = useState(false);
  const autoApplied = useRef<{ conversationId?: string; sequence: number }>({
    sequence: 0,
  });
  const [autoRestore, setAutoRestore] = useState<(() => void) | undefined>(
    undefined,
  );
  const toggleAutoApply = (enabled: boolean) => {
    setAutoApply(enabled);
    setAutoRestore(undefined);
    // 有効にした時点の通番以下の event は、読み直した古い event として適用しない。
    autoApplied.current = {
      conversationId: view.conversationId,
      sequence: view.events.at(-1)?.sequence ?? 0,
    };
  };
  // **1 回に最新の 1 件だけを適用する。** 適用は検索の条件の全体を置き換える。
  useEffect(() => {
    if (!autoApply) return;
    const position = autoApplied.current;
    const after =
      position.conversationId === view.conversationId ? position.sequence : 0;
    const fresh = view.events.filter((event) => event.sequence > after);
    const last = fresh.at(-1);
    if (last === undefined) return;
    autoApplied.current = {
      conversationId: view.conversationId,
      sequence: last.sequence,
    };
    const applied = fresh.findLast(
      (event) =>
        event.kind === "search_query_card" ||
        (event.kind === "tool_result" && event.searchQuery !== undefined),
    );
    if (applied?.searchQuery === undefined) return;
    const restore = onApplySearchQuery(applied.searchQuery, applied.origins);
    setAutoRestore((current) => current ?? restore);
  }, [autoApply, view.events, view.conversationId, onApplySearchQuery]);

  return (
    <section
      aria-labelledby="assist-conversation-heading"
      className="assist-conversation"
    >
      <h2 id="assist-conversation-heading" className="visually-hidden">
        AI 支援との会話
      </h2>
      <RelayNotice relay={view.relay} onRecheck={view.recheck} />
      {view.events.length === 0 ? null : (
        <ol aria-label="会話" className="assist-events">
          {view.events.map((event) => (
            <EventItem
              key={event.sequence}
              event={event}
              card={card}
              tool={tool}
            />
          ))}
        </ol>
      )}
      <div className="flex items-center gap-1">
        {view.answering ? (
          <p role="status">
            <StatusLabel status="running" label="AI の応答中" />
          </p>
        ) : null}
        {view.failure === undefined ? null : (
          <FetchFailureNotice failure={view.failure} />
        )}
        {!view.sending && (view.answering || view.failure !== undefined) ? (
          <IconButton label="会話を再読み込み" onPress={view.reread}>
            <RefreshCw size={14} aria-hidden="true" />
          </IconButton>
        ) : null}
      </div>
      {view.relay.status === "available" ? (
        <div className="flex items-center gap-1">
          <label className="flex items-center gap-1">
            <input
              type="checkbox"
              checked={autoApply}
              onChange={(event) => toggleAutoApply(event.target.checked)}
            />
            AI の検索を画面に自動で適用
          </label>
          <HelpPopover label="AI の検索の自動適用">
            <KeyValueList
              stacked
              pairs={[
                {
                  name: "適用する条件",
                  value:
                    "有効にした後に届いた、AI が勧めた検索の条件と、画面の検索欄で表せるグラフの検索の条件",
                },
                { name: "適用する時点", value: "条件が届いた時点" },
              ]}
            />
          </HelpPopover>
          <IconButton
            label="自動で適用を始める前の条件に復元"
            isDisabled={autoRestore === undefined}
            disabledReason={{ title: "適用前", text: "自動の適用が必要" }}
            onPress={() => {
              autoRestore?.();
              setAutoRestore(undefined);
            }}
          >
            <Undo2 size={14} aria-hidden="true" />
          </IconButton>
        </div>
      ) : null}
      {view.relay.status === "available" ? (
        <form onSubmit={submit} className="assist-form">
          <div className="flex items-center gap-1">
            <label htmlFor="assist-draft">AI への発言</label>
            <HelpPopover label="AI への発言">
              <KeyValueList
                stacked
                pairs={[
                  {
                    name: "添える参照",
                    value:
                      "検索の条件・推定条件・開いているレコード・選んでいるエッジとノード",
                  },
                  {
                    name: "レコードの送信先",
                    value: "送信の許可を記録した提供者",
                  },
                ]}
              />
            </HelpPopover>
          </div>
          <textarea
            id="assist-draft"
            value={draft}
            rows={3}
            onChange={(event) => setDraft(event.target.value)}
          />
          <div className="card-actions">
            <IconButton
              type="submit"
              variant="primary"
              label="発言を送信"
              isDisabled={!canSend}
              disabledReason={
                busy
                  ? { title: "応答中", text: "応答の完了を待機" }
                  : { title: "発言なし", text: "発言の入力が必要" }
              }
            >
              <Send size={14} aria-hidden="true" />
            </IconButton>
            {view.conversationId === undefined ? null : (
              <IconButton
                label="新規会話を開始"
                isDisabled={view.sending}
                disabledReason={{ title: "送信中", text: "送信の完了を待機" }}
                onPress={view.startOver}
              >
                <MessageSquarePlus size={14} aria-hidden="true" />
              </IconButton>
            )}
          </div>
        </form>
      ) : null}
    </section>
  );
}

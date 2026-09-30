import { useCallback, useEffect, useRef, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  type AssistTurnContext,
  fetchRelayConversations,
  fetchRelayEvents,
  fetchRelayStatus,
  openRelayConversation,
  sendRelayTurn,
} from "@/shared/api/assistRelay";
import type { AssistProvider } from "@/shared/contracts/assistPermissions";
import type { AssistEvent } from "@/shared/contracts/assistRelay";
import type { FetchFailure } from "@/shared/lib/fetchState";

/**
 * 中継の状態。
 *
 * - `absent`: 画面を開いた時点で中継の状態を読めなかった。画面を中継を通さずに開いている。
 * - `disconnected`: 中継の状態を読めた後に、中継と通信できなくなった。
 */
export type RelayState =
  | { status: "checking" }
  | { status: "absent" }
  | { status: "available"; providers: AssistProvider[] }
  | { status: "disconnected" };

/** 会話の欄の状態と操作。 */
export type AssistConversationView = {
  relay: RelayState;
  /** 中継の状態を読み直す。 */
  recheck: () => void;
  events: AssistEvent[];
  /** 応答の途中の発言がある。 */
  answering: boolean;
  /** 会話を始めているか、発言を送っている途中である。 */
  sending: boolean;
  /** 直前の操作の失敗。読めた会話を消さない。 */
  failure?: FetchFailure;
  /** 出している会話の識別子。会話が無いときは undefined。 */
  conversationId?: string;
  /**
   * 発言を送る。会話が無ければ、中継が起動できる最初の提供者で始める。
   * 中継が発言を受け付けたときに真を返す。onAccepted は、応答の完了を待たずに、中継が発言を
   * 受け付けた時点で呼ぶ。
   */
  send: (
    text: string,
    context: AssistTurnContext,
    onAccepted?: () => void,
  ) => Promise<boolean>;
  /** 中継が持つ会話の event を読み直す。 */
  reread: () => void;
  /**
   * 出している会話を外し、次の発言で新しい会話を始める。提供者を止めた会話は以後の発言を
   * 受け付けないため、分析者はこの操作で続ける。
   */
  startOver: () => void;
};

/** 発言の識別子を作る。英数字 32 文字である。 */
function newTurnId(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(16));
  return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join(
    "",
  );
}

/** 通番で重ねずに event を足し、通番の順に並べる。 */
function mergeEvents(
  current: readonly AssistEvent[],
  incoming: readonly AssistEvent[],
): AssistEvent[] {
  const bySequence = new Map(current.map((event) => [event.sequence, event]));
  for (const event of incoming) {
    bySequence.set(event.sequence, event);
  }
  return [...bySequence.values()].sort((a, b) => a.sequence - b.sequence);
}

/** 発言 1 つへの応答が終わったことを表す event か。 */
function endsTurn(event: AssistEvent): boolean {
  return event.kind === "turn_end" || event.kind === "provider_error";
}

/**
 * 中継の状態と、中継が持つ会話を保つ。
 *
 * **応答の stream は発言 1 回の間だけ開く。** 再読み込みした画面は、中継が持つ最後の会話の
 * event を読み直して会話を出し直す。
 */
export function useAssistConversation(): AssistConversationView {
  const [relay, setRelay] = useState<RelayState>({ status: "checking" });
  const [checkCount, setCheckCount] = useState(0);
  const [conversationId, setConversationId] = useState<string | undefined>(
    undefined,
  );
  const [events, setEvents] = useState<AssistEvent[]>([]);
  const [answering, setAnswering] = useState(false);
  const [sending, setSending] = useState(false);
  const [failure, setFailure] = useState<FetchFailure | undefined>(undefined);
  // 中継の状態を 1 度でも読めたか。読めた後の失敗は「接続が切れた」と出す。
  const reached = useRef(false);

  const lose = useCallback((cause: FetchFailure) => {
    setFailure(cause);
    if (cause.kind === "network") {
      setRelay(
        reached.current ? { status: "disconnected" } : { status: "absent" },
      );
    }
  }, []);

  // biome-ignore lint/correctness/useExhaustiveDependencies: checkCount が変わったときに読み直す。
  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      const status = await fetchRelayStatus({ signal: controller.signal });
      if (controller.signal.aborted) return;
      if (!status.ok) {
        setRelay(
          reached.current ? { status: "disconnected" } : { status: "absent" },
        );
        return;
      }
      reached.current = true;
      setRelay({ status: "available", providers: status.value.providers });
      const listed = await fetchRelayConversations({
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      if (!listed.ok) {
        setFailure(listed.failure);
        return;
      }
      const last = listed.value.at(-1);
      if (last === undefined) return;
      const read = await fetchRelayEvents(last.id, 0, {
        signal: controller.signal,
      });
      if (controller.signal.aborted) return;
      if (!read.ok) {
        setFailure(read.failure);
        return;
      }
      // **会話の event を、読んだ会話の全件で置き換える。** 通番は会話ごとに 1 から始まるため、
      // 別の会話の event と重ねない。
      setConversationId(last.id);
      setEvents(read.value.events);
      setAnswering(read.value.answering);
    };
    void load().catch(() => {
      if (!controller.signal.aborted) {
        setRelay(
          reached.current ? { status: "disconnected" } : { status: "absent" },
        );
      }
    });
    return () => controller.abort();
  }, [checkCount]);

  const recheck = useCallback(() => setCheckCount((count) => count + 1), []);

  const send = useCallback(
    async (
      text: string,
      context: AssistTurnContext,
      onAccepted?: () => void,
    ): Promise<boolean> => {
      if (sending || answering || relay.status !== "available") return false;
      const provider = relay.providers[0];
      const run = async (): Promise<boolean> => {
        setSending(true);
        setFailure(undefined);
        let id = conversationId;
        if (id === undefined) {
          if (provider === undefined) {
            setFailure({
              ...buildFetchFailure("request_rejected", "AI 支援の会話の開始"),
              failureDescription: "中継が起動できる提供者なし",
            });
            return false;
          }
          const opened = await openRelayConversation(provider);
          if (!opened.ok) {
            lose(opened.failure);
            return false;
          }
          id = opened.value.id;
          setConversationId(id);
        }
        const turnId = newTurnId();
        // 中継が発言を受け付けたか。受け付けた発言は中継の会話に残り、読み直す操作で出せる。
        let accepted = false;
        setAnswering(true);
        const sent = await sendRelayTurn(
          id,
          { turnId, text, context },
          (event) => {
            if (
              !accepted &&
              event.turnId === turnId &&
              event.kind === "user_message"
            ) {
              accepted = true;
              onAccepted?.();
            }
            setEvents((current) => mergeEvents(current, [event]));
            if (endsTurn(event)) setAnswering(false);
          },
        );
        if (!sent.ok) {
          // 応答の途中で切れたときも、中継は応答を続ける。読み直す操作で続きを出す。
          setAnswering(false);
          lose(sent.failure);
        }
        return sent.ok || accepted;
      };
      try {
        return await run();
      } catch {
        setAnswering(false);
        setFailure(buildFetchFailure("unexpected", "AI 支援への発言の送信"));
        return false;
      } finally {
        setSending(false);
      }
    },
    [sending, answering, relay, conversationId, lose],
  );

  // 出している会話の識別子。読み直しの応答が届いた時点で、読んだ会話がまだ出ているかを確かめる。
  const shownConversation = useRef(conversationId);
  shownConversation.current = conversationId;

  const reread = useCallback(() => {
    if (conversationId === undefined) return;
    const after = events.at(-1)?.sequence ?? 0;
    void fetchRelayEvents(conversationId, after).then((read) => {
      // 読む間に新しい会話を始めたら、前の会話の event を足さない。
      if (shownConversation.current !== conversationId) return;
      if (!read.ok) {
        lose(read.failure);
        return;
      }
      setFailure(undefined);
      setEvents((current) => mergeEvents(current, read.value.events));
      setAnswering(read.value.answering);
    });
  }, [conversationId, events, lose]);

  const startOver = useCallback(() => {
    if (sending) return;
    shownConversation.current = undefined;
    setConversationId(undefined);
    setEvents([]);
    setAnswering(false);
    setFailure(undefined);
  }, [sending]);

  return {
    relay,
    recheck,
    conversationId,
    events,
    answering,
    sending,
    failure,
    send,
    reread,
    startOver,
  };
}

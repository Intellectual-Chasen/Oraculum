import { useCallback, useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  type AssistPermissionDraft,
  fetchAssistPermissions,
  recordAssistPermission,
} from "@/shared/api/assistPermissions";
import type { AssistProviderPermission } from "@/shared/contracts/assistPermissions";
import type { FetchFailure } from "@/shared/lib/fetchState";

const listFailureSummary = "AI 支援の送信の許可の取得";
const recordFailureSummary = "AI 支援の送信の許可の記録";

/**
 * 送信の許可の一覧の状態。
 *
 * - `unavailable`: サーバーが調査の directory を渡さずに起動しており、AI 支援を使えない。
 */
export type AssistPermissionsState =
  | { status: "loading" }
  | { status: "unavailable" }
  | { status: "failed"; failure: FetchFailure }
  | { status: "loaded"; providers: AssistProviderPermission[] };

/** 送信の許可の一覧と、記録の操作。 */
export type AssistPermissionsView = {
  state: AssistPermissionsState;
  /** 記録の途中は真である。 */
  recording: boolean;
  /** 記録が失敗したときの理由。**読めた一覧を消さない。** */
  recordFailure?: FetchFailure;
  /** 送信の許可の改訂を 1 つ記録し、成功したらその提供者の状態を置き換える。 */
  record: (draft: AssistPermissionDraft) => void;
};

/** 取得の失敗を、使えない起動とそれ以外に分ける。 */
function stateOfFailure(failure: FetchFailure): AssistPermissionsState {
  return failure.failureCode === "assist_unavailable"
    ? { status: "unavailable" }
    : { status: "failed", failure };
}

/** 提供者ごとの送信の許可を読み込んで保つ。 */
export function useAssistPermissions(): AssistPermissionsView {
  const [state, setState] = useState<AssistPermissionsState>({
    status: "loading",
  });
  const [recording, setRecording] = useState(false);
  const [recordFailure, setRecordFailure] = useState<FetchFailure | undefined>(
    undefined,
  );

  useEffect(() => {
    const controller = new AbortController();
    const load = async () => {
      const result = await fetchAssistPermissions({
        signal: controller.signal,
      });
      if (controller.signal.aborted) {
        return;
      }
      setState(
        result.ok
          ? { status: "loaded", providers: result.value.providers }
          : stateOfFailure(result.failure),
      );
    };
    void load().catch(() => {
      if (controller.signal.aborted) {
        return;
      }
      setState({
        status: "failed",
        failure: buildFetchFailure("unexpected", listFailureSummary),
      });
    });
    return () => controller.abort();
  }, []);

  const record = useCallback(
    (draft: AssistPermissionDraft) => {
      if (recording) {
        return;
      }
      setRecording(true);
      setRecordFailure(undefined);
      const send = async () => {
        const result = await recordAssistPermission(draft);
        setRecording(false);
        if (!result.ok) {
          setRecordFailure(result.failure);
          return;
        }
        setState((previous) =>
          previous.status === "loaded"
            ? {
                status: "loaded",
                providers: previous.providers.map((provider) =>
                  provider.provider === result.value.provider
                    ? result.value
                    : provider,
                ),
              }
            : previous,
        );
      };
      void send().catch(() => {
        setRecording(false);
        setRecordFailure(buildFetchFailure("unexpected", recordFailureSummary));
      });
    },
    [recording],
  );

  return { state, recording, recordFailure, record };
}

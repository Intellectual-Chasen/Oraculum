import { useCallback, useEffect, useState } from "react";
import {
  createTerminalAssignment,
  fetchTerminalAssignments,
  type TerminalAssignmentDraft,
} from "@/shared/api/terminalAssignments";
import type { TerminalAssignment } from "@/shared/contracts/terminalAssignments";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";

/** 端末の割当の一覧と、記録の結果を持つ。 */
export type TerminalAssignmentsView = {
  /** 記録済みの割当の一覧である。 */
  state: FetchState<TerminalAssignment[]>;
  /** 記録が失敗したときの理由である。 */
  recordFailure: FetchFailure | undefined;
  /** 記録した割当の件数である。入力欄を消す時点を決める材料になる。 */
  recordedCount: number;
  /** 割当を記録し、一覧を読み直すまでの間に真である。 */
  recording: boolean;
  /** 割当を 1 件記録する。 */
  record: (draft: TerminalAssignmentDraft) => Promise<void>;
};

/**
 * 利用者が与えた端末の割当を読み、画面から割当を記録する。
 *
 * **記録した後に一覧を読み直す。** 割当は端末の判定の材料であり、記録した時点で
 * グラフとレコードの応答が変わる。画面に古い一覧を残さない。
 */
export function useTerminalAssignments(): TerminalAssignmentsView {
  const [state, setState] = useState<FetchState<TerminalAssignment[]>>({
    status: "loading",
  });
  const [recordFailure, setRecordFailure] = useState<FetchFailure | undefined>(
    undefined,
  );
  const [recordedCount, setRecordedCount] = useState(0);

  const load = useCallback(async (signal?: AbortSignal) => {
    const result = await fetchTerminalAssignments({ signal });
    if (signal?.aborted === true) {
      return;
    }
    if (!result.ok) {
      setState({ status: "failed", failure: result.failure });
      return;
    }
    setState({ status: "loaded", value: result.value.assignments });
  }, []);

  useEffect(() => {
    const controller = new AbortController();
    void load(controller.signal);
    return () => {
      controller.abort();
    };
  }, [load]);

  const [recording, setRecording] = useState(false);
  const record = useCallback(
    async (draft: TerminalAssignmentDraft) => {
      setRecording(true);
      try {
        const result = await createTerminalAssignment(draft);
        if (!result.ok) {
          setRecordFailure(result.failure);
          return;
        }
        setRecordFailure(undefined);
        setRecordedCount((count) => count + 1);
        await load();
      } finally {
        setRecording(false);
      }
    },
    [load],
  );

  return { state, recordFailure, recordedCount, recording, record };
}

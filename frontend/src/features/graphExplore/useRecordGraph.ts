import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import { fetchRecordGraph } from "@/shared/api/recordGraph";
import type { RecordLocator } from "@/shared/contracts/common";
import type { RecordGraphResponse } from "@/shared/contracts/recordGraph";
import type { FetchState } from "@/shared/lib/fetchState";

const failureSummary = "レコードのノードとエッジの取得";

/** 開いたレコードが無いことと、取得の 4 状態を分ける。 */
export type RecordGraphState =
  | { status: "unselected" }
  | FetchState<RecordGraphResponse>;

/**
 * 開いたレコードを根拠に持つノードとエッジを取得する。レコードが変わるたびに取り直し、前の
 * 取得を打ち切る。`version` は端末の割当と時刻の解釈を記録した回数であり、変わるたびに
 * 取り直す。
 */
export function useRecordGraph(
  recordRef: RecordLocator | undefined,
  matchConditions: MatchConditionSelection,
  version: number,
): RecordGraphState {
  const [state, setState] = useState<RecordGraphState>({
    status: "unselected",
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じレコードを取り直す。
  useEffect(() => {
    if (recordRef === undefined) {
      setState({ status: "unselected" });
      return;
    }
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchRecordGraph(
      { recordRef, matchConditions },
      { signal: controller.signal },
    ).then(
      (result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        );
      },
      () => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure("unexpected", failureSummary),
        });
      },
    );
    return () => controller.abort();
  }, [recordRef, matchConditions, version]);

  return state;
}

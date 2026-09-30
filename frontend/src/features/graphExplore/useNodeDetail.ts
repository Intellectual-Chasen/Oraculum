import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchNodeDetail } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { NodeDetailResponse } from "@/shared/contracts/graphDetail";
import type { FetchState } from "@/shared/lib/fetchState";

const failureSummary = "ノードの詳細の取得";

/** 選んでいるノードが無いことと、取得の 4 状態を分ける。 */
export type NodeDetailState =
  | { status: "unselected" }
  | FetchState<NodeDetailResponse>;

/**
 * ノード 1 つの詳細を取得する。属性と根拠を全件受け取る。
 * 選んだノードが変わるたびに取り直し、前の取得を `AbortController` で打ち切る。
 * 古い選択に対する応答を現在の詳細に出さない。
 *
 * **関連付けの条件の選択は上位の画面が持つ。** グラフの探索と同じ選択で問い合わせないと、
 * 図に出ているノードの詳細が別の関連付けの結果を出す。
 *
 * `version` は端末の割当を記録した回数である。記録で backend のグラフが変わるため、
 * 変わるたびに取り直す。
 */
export function useNodeDetail(
  nodeId: string | undefined,
  matchConditions: MatchConditionSelection,
  version: number,
): NodeDetailState {
  const [state, setState] = useState<NodeDetailState>({
    status: "unselected",
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じノードを取り直す。
  useEffect(() => {
    if (nodeId === undefined) {
      setState({ status: "unselected" });
      return;
    }
    const controller = new AbortController();
    setState({ status: "loading" });

    const load = async () => {
      const result = await fetchNodeDetail(
        { id: nodeId, matchConditions },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) {
        return;
      }
      if (!result.ok) {
        setState({ status: "failed", failure: result.failure });
        return;
      }
      setState({ status: "loaded", value: result.value });
    };

    void load().catch(() => {
      if (controller.signal.aborted) {
        return;
      }
      setState({
        status: "failed",
        failure: buildFetchFailure("unexpected", failureSummary),
      });
    });
    return () => controller.abort();
  }, [nodeId, matchConditions, version]);

  return state;
}

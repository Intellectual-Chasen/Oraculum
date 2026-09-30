import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchEdgeDetail } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { CaseId } from "@/shared/contracts/cases";
import type {
  EdgeDetailResponse,
  EdgeEvidenceSelector,
} from "@/shared/contracts/graphDetail";
import type { FetchState } from "@/shared/lib/fetchState";

const failureSummary = "エッジの詳細の取得";

/** 選んでいる関係が無いことと、取得の 4 状態を分ける。 */
export type EdgeDetailState =
  | { status: "unselected" }
  | FetchState<EdgeDetailResponse>;

/**
 * エッジ 1 本の詳細を取得する。
 * 選んだ関係と区分と案件が変わるたびに取り直し、前の取得を `AbortController` で打ち切る。
 * 古い選択に対する応答を現在の詳細に出さない。
 *
 * **区分を指す値は応答が返した組をそのまま渡す。** 画面が比べられる値を組み立てない。
 *
 * `version` は端末の割当を記録した回数である。記録で backend のグラフが変わるため、
 * 変わるたびに取り直す。
 */
export function useEdgeDetail(
  edgeId: string | undefined,
  selector: EdgeEvidenceSelector | undefined,
  matchConditions: MatchConditionSelection,
  caseId: CaseId | undefined,
  version: number,
): EdgeDetailState {
  const [state, setState] = useState<EdgeDetailState>({
    status: "unselected",
  });
  // 区分の組をそのまま依存に置くと、同じ区分を指す別の組が毎回の描画で作られ、要求を
  // 繰り返す。値を 1 つの文字列にまとめて比べる。
  const selectorKey = JSON.stringify(selector ?? null);

  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じ関係を取り直す。
  useEffect(() => {
    if (edgeId === undefined) {
      setState({ status: "unselected" });
      return;
    }
    const controller = new AbortController();
    setState({ status: "loading" });

    const load = async () => {
      const decoded: EdgeEvidenceSelector | null = JSON.parse(selectorKey);
      const result = await fetchEdgeDetail(
        {
          id: edgeId,
          matchConditions,
          selector: decoded ?? undefined,
          caseId,
        },
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
  }, [edgeId, selectorKey, matchConditions, caseId, version]);

  return state;
}

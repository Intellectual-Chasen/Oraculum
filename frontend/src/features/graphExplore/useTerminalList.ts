import { useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchGraph, type NodeRef } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { GraphNode } from "@/shared/contracts/graph";
import type { FetchState } from "@/shared/lib/fetchState";
import { distinctTerminalNames } from "@/shared/lib/terminalSource";

const failureSummary = "端末の一覧の取得";

/**
 * 端末で絞る条件の選択肢を取得する。取り込んだ全体の端末のノードを、操作 9 で読む。
 * 選択肢は、レコードを置いた端末 (observation が observed のノード) である。
 *
 * **表示名は原資料の文字列を優先する。** 文字列を持たない端末は正規化値、どちらも無い端末は
 * 識別子を出す。同じ表示名の端末が 2 つ以上あるときは、端末を記録した収集元の表示名を足す
 * (distinctTerminalNames)。収集元の表示名は `fileNamesByContent` (内容の sha256 から表示名)
 * で探す。
 */
export function useTerminalList(
  matchConditions: MatchConditionSelection,
  version: number,
  fileNamesByContent: ReadonlyMap<string, string>,
): FetchState<NodeRef[]> {
  const [state, setState] = useState<FetchState<GraphNode[]>>({
    status: "loading",
  });

  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じ条件で取り直す。
  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });
    const load = async () => {
      const result = await fetchGraph(
        {
          matchConditions,
          depth: 0,
          granularity: "object",
          nodeKinds: ["terminal"],
        },
        { signal: controller.signal },
      );
      if (controller.signal.aborted) {
        return;
      }
      if (!result.ok) {
        // **失敗した操作を端末の一覧として書く。** 部分グラフの取得の文言のままだと、
        // 図を出せている画面で、分析者は図の取得が失敗したと読む。
        setState({
          status: "failed",
          failure: { ...result.failure, summary: failureSummary },
        });
        return;
      }
      // **参照だけの端末を出さない。** 割当が IP を与えただけの端末には置いたレコードが
      // 無く、選ぶと必ず 0 件になる。グラフのノードとしては残る。
      setState({
        status: "loaded",
        value: result.value.nodes.filter(
          (node) => node.observation !== "referenced",
        ),
      });
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
  }, [matchConditions, version]);

  return useMemo(() => {
    if (state.status !== "loaded") return state;
    const names = distinctTerminalNames(state.value, fileNamesByContent);
    return {
      status: "loaded",
      value: state.value.map((node) => ({
        id: node.id,
        label: names.get(node.id) ?? node.id,
      })),
    };
  }, [state, fileNamesByContent]);
}

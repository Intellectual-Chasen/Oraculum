import { ChartBar } from "lucide-react";
import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchNodeValueCounts } from "@/shared/api/graph";

const failureSummary = "隣接ノードの集計の取得";

import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { EdgeDirection, EdgeKind } from "@/shared/contracts/graph";
import type { NodeDetailResponse } from "@/shared/contracts/graphDetail";
import type { NodeValueCountsResponse } from "@/shared/contracts/nodeValueCounts";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { TextField } from "@/shared/ui/TextField";
import {
  edgeDirectionLabels,
  edgeKindLabels,
  valueCountsEmptyReasonLabels,
} from "./labels";
import { ValueCountList } from "./ValueCountList";

/** 数えるエッジとフィールドの組。 */
type RelatedCountRequest = {
  relation: string;
  countBy: string;
};

/**
 * 選んだノードに繋がる、ある種別・ある向きの関係の相手側のノードが持つ欄の値ごとの件数を出す。
 *
 * **関係の根拠のレコードに観測した値だけを数える。** 期間と案件では絞らない。関係の件数の
 * 表と同じ範囲である。ノードを替えたら、呼び出し側が key を替えて結果を捨てる。
 */
export function RelatedValueCounts({
  detail,
  matchConditions,
}: {
  detail: NodeDetailResponse;
  matchConditions: MatchConditionSelection;
}) {
  const relations = detail.edgeCounts.map((count) => ({
    value: `${count.edgeKind}\u0000${count.direction}`,
    label: `${edgeKindLabels[count.edgeKind]} · ${edgeDirectionLabels[count.direction]} · ${formatCount(count.edgeCount)}`,
    edgeKind: count.edgeKind,
    direction: count.direction,
  }));
  const [relation, setRelation] = useState(relations[0]?.value ?? "");
  const [countBy, setCountBy] = useState("");
  const [request, setRequest] = useState<RelatedCountRequest | undefined>(
    undefined,
  );
  const state = useRelatedValueCounts(
    detail.node.id,
    request,
    relations,
    matchConditions,
  );
  if (relations.length === 0) {
    return null;
  }
  return (
    <section aria-label="隣接ノードの集計">
      <h3>隣接ノードの集計</h3>
      <form
        className="flex flex-col items-start gap-2"
        onSubmit={(event) => {
          event.preventDefault();
          // 集計するフィールドが空のままの Enter で、空のフィールドを数える要求を出さない。
          if (countBy.trim() === "") {
            return;
          }
          setRequest({ relation, countBy });
        }}
      >
        <label className="flex w-full flex-col gap-1 text-sm font-medium text-ink">
          エッジ
          <select
            className="w-full font-normal"
            value={relation}
            onChange={(event) => setRelation(event.target.value)}
          >
            {relations.map((option) => (
              <option key={option.value} value={option.value}>
                {option.label}
              </option>
            ))}
          </select>
        </label>
        <TextField
          label="集計するフィールド"
          mono
          className="w-full"
          value={countBy}
          onChange={setCountBy}
          placeholder="connection.source_address"
        />
        <IconButton
          type="submit"
          variant="secondary"
          label="隣接ノードを集計"
          isDisabled={countBy.trim() === ""}
          disabledReason={{
            title: "フィールドが空",
            text: "集計するフィールドの入力",
          }}
        >
          <ChartBar size={14} aria-hidden="true" />
        </IconButton>
      </form>
      {request === undefined || state === undefined ? null : (
        <FetchStateView state={state} loadingDescription="隣接ノードの集計中">
          {(response) =>
            response.valueCounts.length === 0 ? (
              <p role="status">
                {response.emptyReason === undefined
                  ? "値なし"
                  : valueCountsEmptyReasonLabels[response.emptyReason]}
              </p>
            ) : (
              <>
                <KeyValueList
                  pairs={[
                    {
                      name: "期間と案件のフィルタ",
                      value: (
                        <Hint text="件数: 隣接ノードのレコードのうち値を記録したレコードの数">
                          未適用
                        </Hint>
                      ),
                    },
                  ]}
                />
                <ValueCountList
                  key={`${request.relation}\u0000${response.countBy}`}
                  counts={response.valueCounts}
                  caption="隣接ノードのフィールドの値ごとのレコード数"
                  absentSelection="集計の結果に無い値"
                />
              </>
            )
          }
        </FetchStateView>
      )}
    </section>
  );
}

/** 要求の組が変わるたびに取り直す。古い要求は打ち切る。 */
function useRelatedValueCounts(
  nodeId: string,
  request: RelatedCountRequest | undefined,
  relations: { value: string; edgeKind: EdgeKind; direction: EdgeDirection }[],
  matchConditions: MatchConditionSelection,
): FetchState<NodeValueCountsResponse> | undefined {
  const [state, setState] = useState<
    FetchState<NodeValueCountsResponse> | undefined
  >(undefined);
  const chosen = relations.find((option) => option.value === request?.relation);
  const edgeKind = chosen?.edgeKind;
  const direction = chosen?.direction;
  const countBy = request?.countBy;
  useEffect(() => {
    if (
      edgeKind === undefined ||
      direction === undefined ||
      countBy === undefined
    ) {
      setState(undefined);
      return;
    }
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchNodeValueCounts(
      { id: nodeId, edgeKind, direction, countBy, matchConditions },
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        );
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure("unexpected", failureSummary),
        });
      });
    return () => controller.abort();
  }, [nodeId, edgeKind, direction, countBy, matchConditions]);
  return state;
}

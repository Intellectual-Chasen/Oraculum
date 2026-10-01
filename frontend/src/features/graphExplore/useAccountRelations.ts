import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { fetchAccountRelations } from "@/shared/api/accountRelations";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { RecordFilterCriteria } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type {
  AccountRelationKey,
  AccountRelationsResponse,
} from "@/shared/contracts/accountRelations";
import type { EdgeKind } from "@/shared/contracts/graph";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";

export function useAccountRelations(
  rootId: string | undefined,
  filter: RecordFilterCriteria,
  roles: readonly EdgeKind[] | undefined,
  selected: AccountRelationKey | undefined,
  matchConditions: MatchConditionSelection,
  dataVersion: number,
): {
  state: FetchState<AccountRelationsResponse>;
  loadMore: () => void;
  loadingMore: boolean;
  moreFailure?: FetchFailure;
} {
  const [state, setState] = useState<FetchState<AccountRelationsResponse>>({
    status: "loading",
  });
  const [loadingMore, setLoadingMore] = useState(false);
  const [moreFailure, setMoreFailure] = useState<FetchFailure | undefined>();
  const pagingController = useRef<AbortController | undefined>(undefined);
  const request = useMemo(
    () =>
      rootId === undefined
        ? undefined
        : {
            accountNodeId: rootId,
            filter,
            roles,
            selected,
            matchConditions,
          },
    [rootId, filter, roles, selected, matchConditions],
  );
  // biome-ignore lint/correctness/useExhaustiveDependencies: 端末割当などで観測グラフが変わったときに再取得する。
  useEffect(() => {
    pagingController.current?.abort();
    if (request === undefined) return;
    const controller = new AbortController();
    setState({ status: "loading" });
    setLoadingMore(false);
    setMoreFailure(undefined);
    void fetchAccountRelations(request, { signal: controller.signal })
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
          failure: buildFetchFailure("unexpected", "相手アカウントの取得"),
        });
      });
    return () => {
      controller.abort();
      pagingController.current?.abort();
    };
  }, [request, dataVersion]);
  const loadMore = useCallback(() => {
    if (
      request === undefined ||
      state.status !== "loaded" ||
      state.value.nextOffset === undefined ||
      loadingMore
    )
      return;
    const controller = new AbortController();
    pagingController.current = controller;
    setLoadingMore(true);
    setMoreFailure(undefined);
    void fetchAccountRelations(
      { ...request, offset: state.value.nextOffset },
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted) return;
        setLoadingMore(false);
        if (!result.ok) {
          setMoreFailure(result.failure);
          return;
        }
        setState((current) =>
          current.status === "loaded"
            ? {
                status: "loaded",
                value: {
                  ...result.value,
                  records: [
                    ...(current.value.records ?? []),
                    ...(result.value.records ?? []),
                  ],
                },
              }
            : current,
        );
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setLoadingMore(false);
        setMoreFailure(buildFetchFailure("unexpected", "続きを読み込む"));
      });
  }, [request, state, loadingMore]);
  return { state, loadMore, loadingMore, moreFailure };
}

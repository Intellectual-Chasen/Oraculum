import { useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { GraphTimeFilter } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import { fetchRecord } from "@/shared/api/records";
import { fetchTimeline } from "@/shared/api/timeline";
import type { RecordLocator, Timestamp } from "@/shared/contracts/common";
import type { TimelineResponse } from "@/shared/contracts/timeline";
import type { FetchState } from "@/shared/lib/fetchState";
import { contextTimeFilter } from "@/shared/lib/timeFilter";
import { type ContextWindowSeconds, findEventTime } from "./contextWindow";

const failureSummary = "レコードの前後の取得";

/** 開いたレコードの前後の取得結果。 */
export type RecordContextValue =
  | {
      kind: "time_unreadable";
      /** 事象の時刻の項目が無い場合は `undefined`。 */
      eventTime?: Timestamp;
    }
  | {
      kind: "window";
      eventTime: Timestamp;
      timeFilter: GraphTimeFilter;
      timeline: TimelineResponse;
    };

type ContextRequest = {
  recordRef: RecordLocator;
  windowSeconds: ContextWindowSeconds;
  matchConditions: MatchConditionSelection;
  dataVersion: number;
};

async function loadContext(
  request: ContextRequest,
  signal: AbortSignal,
): Promise<FetchState<RecordContextValue>> {
  const { recordRef } = request;
  const record = await fetchRecord(
    {
      record: {
        sourceId: recordRef.sourceId,
        sourceContentSha256: recordRef.sourceContentSha256,
        sequenceNumber: recordRef.sequenceNumber,
        lineNumber: recordRef.lineNumber,
        byteOffset: recordRef.byteOffset,
      },
    },
    { signal },
  );
  if (!record.ok) {
    return { status: "failed", failure: record.failure };
  }
  const eventTime = findEventTime(record.value.fields);
  const timeFilter =
    eventTime === undefined
      ? undefined
      : contextTimeFilter(eventTime, request.windowSeconds);
  if (eventTime === undefined || timeFilter === undefined) {
    return { status: "loaded", value: { kind: "time_unreadable", eventTime } };
  }
  const timeline = await fetchTimeline(
    { matchConditions: request.matchConditions, timeFilter },
    { signal },
  );
  if (!timeline.ok) {
    return { status: "failed", failure: timeline.failure };
  }
  return {
    status: "loaded",
    value: { kind: "window", eventTime, timeFilter, timeline: timeline.value },
  };
}

/**
 * 開いたレコードの事象の時刻を元レコードの取得 (操作 3) で読み、前後の期間の時系列を
 * 取得する。引数のどれかが変わるたびに取り直し、前の取得を `AbortController` で打ち切る。
 * 状態に取得の引数を持たせ、今の引数と違う取得の結果を読み込み中として返す。
 */
export function useRecordContext(
  recordRef: RecordLocator,
  windowSeconds: ContextWindowSeconds,
  matchConditions: MatchConditionSelection,
  dataVersion: number,
): FetchState<RecordContextValue> {
  const request = useMemo<ContextRequest>(
    () => ({ recordRef, windowSeconds, matchConditions, dataVersion }),
    [recordRef, windowSeconds, matchConditions, dataVersion],
  );
  const [settled, setSettled] = useState<{
    request: ContextRequest;
    state: FetchState<RecordContextValue>;
  }>();

  useEffect(() => {
    const controller = new AbortController();
    loadContext(request, controller.signal)
      .then((state) => {
        if (!controller.signal.aborted) {
          setSettled({ request, state });
        }
      })
      .catch(() => {
        if (!controller.signal.aborted) {
          setSettled({
            request,
            state: {
              status: "failed",
              failure: buildFetchFailure("unexpected", failureSummary),
            },
          });
        }
      });
    return () => controller.abort();
  }, [request]);

  return settled?.request === request ? settled.state : { status: "loading" };
}

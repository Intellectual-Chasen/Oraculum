import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchSources } from "@/shared/api/sources";
import type { FetchState } from "@/shared/lib/fetchState";
import { buildSourceInventory, type SourceInventory } from "./inventory";
import { sourcesEmptyReasonLabels } from "./labels";

/** 収集元の一覧の取得の状態と、取り直しの途中かを持つ。 */
export type SourceInventoryView = {
  /** 最後に取得を終えた一覧。**取り直しの間も前に読めた一覧を出し続ける。** */
  state: FetchState<SourceInventory>;
  /** `version` の取り直しを終えていない間は真である。 */
  refreshing: boolean;
};

/**
 * 収集元の一覧を取得する。取り込んだ収集元を全件受け取る。
 * 取得の途中で画面を離れたときは `AbortController` で打ち切る。
 *
 * `version` が変わるたびに取り直す。時刻の解釈を記録すると、解釈で読んだ観測期間が変わる。
 */
export function useSourceInventory(version = 0): SourceInventoryView {
  const [state, setState] = useState<FetchState<SourceInventory>>({
    status: "loading",
  });
  const [settledVersion, setSettledVersion] = useState<number | undefined>(
    undefined,
  );

  useEffect(() => {
    const controller = new AbortController();

    const load = async () => {
      const result = await fetchSources({ signal: controller.signal });
      if (controller.signal.aborted) {
        return;
      }
      setSettledVersion(version);
      if (!result.ok) {
        setState({ status: "failed", failure: result.failure });
        return;
      }
      const { emptyReason } = result.value;
      if (emptyReason !== undefined) {
        setState({
          status: "empty",
          description: sourcesEmptyReasonLabels[emptyReason],
        });
        return;
      }
      setState({ status: "loaded", value: buildSourceInventory(result.value) });
    };

    void load().catch(() => {
      if (controller.signal.aborted) {
        return;
      }
      setSettledVersion(version);
      setState({
        status: "failed",
        failure: buildFetchFailure("unexpected", "収集元の一覧の取得"),
      });
    });
    return () => controller.abort();
  }, [version]);

  return { state, refreshing: settledVersion !== version };
}

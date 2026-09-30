import { useCallback, useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchSourceFiles } from "@/shared/api/sourceFiles";
import type { SourceFileListing } from "@/shared/contracts/sourceFiles";
import type { FetchState } from "@/shared/lib/fetchState";

const unexpectedSummary = "基準の directory の一覧の取得";

/** 基準の directory の下の directory 1 つの一覧と、取り直す操作。 */
export type SourceFileListingView = {
  state: FetchState<SourceFileListing>;
  /** 同じ directory の一覧を取り直す。 */
  reload: () => void;
};

/**
 * 基準の directory の下の directory `path` の一覧を取得する。
 *
 * **path が変わったら前の取得を打ち切る。** 先に送った要求の応答が、後に開いた directory の
 * 一覧を上書きしない。
 */
export function useSourceFileListing(path: string): SourceFileListingView {
  // 状態を、取得した path と組にして持つ。path を変えた直後の描画で、前の directory の一覧を
  // 新しい path の一覧として出さない。
  const [fetched, setFetched] = useState<{
    path: string;
    state: FetchState<SourceFileListing>;
  }>({ path, state: { status: "loading" } });
  const [attempt, setAttempt] = useState(0);

  // biome-ignore lint/correctness/useExhaustiveDependencies: attempt が変わったとき、つまり reload のたびに取得をやり直す。
  useEffect(() => {
    const controller = new AbortController();
    setFetched({ path, state: { status: "loading" } });
    fetchSourceFiles(path, { signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        setFetched({
          path,
          state: result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        });
      })
      .catch(() => {
        if (controller.signal.aborted) {
          return;
        }
        setFetched({
          path,
          state: {
            status: "failed",
            failure: buildFetchFailure("unexpected", unexpectedSummary),
          },
        });
      });
    return () => {
      controller.abort();
    };
  }, [path, attempt]);

  const reload = useCallback(() => {
    setAttempt((count) => count + 1);
  }, []);

  return {
    state: fetched.path === path ? fetched.state : { status: "loading" },
    reload,
  };
}

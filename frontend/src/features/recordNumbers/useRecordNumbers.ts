import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchRecordNumbers } from "@/shared/api/recordNumbers";
import type { RecordNumbersResponse } from "@/shared/contracts/recordNumbers";
import type { SourceIdentity } from "@/shared/contracts/sources";
import type { FetchState } from "@/shared/lib/fetchState";

/** 要求の条件を 1 つの文字列にする。取得の結果がどの条件の結果かを見分ける。 */
function requestKey(
  source: SourceIdentity | undefined,
  compared: SourceIdentity | undefined,
  dataVersion: number,
): string {
  return JSON.stringify([
    source?.sourceId,
    source?.contentSha256,
    compared?.sourceId,
    compared?.contentSha256,
    dataVersion,
  ]);
}

/**
 * 選んだ収集元のレコードの番号の抜けと、比べる収集元との突き合わせを取得する。
 * `dataVersion` が変わると取り直す。時刻の解釈を記録すると、時刻の鍵の突き合わせが変わる。
 *
 * **前の選択の結果を新しい選択の下に出さない。** 取得の結果は要求の条件と組で持ち、条件が
 * 今の選択と食い違う結果は読み込み中として返す。
 */
export function useRecordNumbers(
  source: SourceIdentity | undefined,
  compared: SourceIdentity | undefined,
  dataVersion: number,
): FetchState<RecordNumbersResponse> | undefined {
  const [fetched, setFetched] = useState<{
    key: string;
    state: FetchState<RecordNumbersResponse>;
  }>();
  const key = requestKey(source, compared, dataVersion);

  // biome-ignore lint/correctness/useExhaustiveDependencies: key が変わったときだけ取り直す。key は source と compared と dataVersion から決まる。
  useEffect(() => {
    if (source === undefined) {
      return;
    }
    const controller = new AbortController();
    fetchRecordNumbers(source, compared, { signal: controller.signal })
      .then((result) => {
        if (controller.signal.aborted) {
          return;
        }
        setFetched({
          key,
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
          key,
          state: {
            status: "failed",
            failure: buildFetchFailure(
              "unexpected",
              "レコードの番号の抜けの取得",
            ),
          },
        });
      });
    return () => controller.abort();
  }, [key]);

  if (source === undefined) {
    return undefined;
  }
  return fetched?.key === key ? fetched.state : { status: "loading" };
}

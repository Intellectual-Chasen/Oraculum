import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchEventKinds } from "@/shared/api/eventKinds";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { EventKindsResponse } from "@/shared/contracts/eventKinds";
import type { SourceIdentity } from "@/shared/contracts/sources";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { formatRecordingScopeNotes, isProxyFormat } from "./labels";
import { ProxyBypassCounts } from "./ProxyBypassCounts";

const failureSummary = "イベントの種類の取得";

/**
 * 選んだ収集元の取り込めたレコードが持つイベントの種類と件数を出す。
 *
 * **0 件の種類を「記録していない」と言い切らない。** レコードから言えるのは、取り込めた
 * レコードにその種類が無いことまでである。入力形式の説明は形式ごとの一般的な説明であり、
 * この収集元を見て判定した結果ではない。どちらも help の吹き出しに置く。
 */
export function SourceEventKinds({
  source,
  failedRecordCount,
  matchConditions,
  dataVersion,
}: {
  source: SourceIdentity;
  /** 取り込めなかったレコードの件数。取り込みの状態を読めないときは出ない。 */
  failedRecordCount: number | undefined;
  matchConditions: MatchConditionSelection;
  dataVersion: number;
}) {
  const state = useSourceEventKinds(source, matchConditions, dataVersion);
  const note = formatRecordingScopeNotes[source.formatKey];
  return (
    <section aria-label="イベントの種類">
      <h3>
        イベントの種類
        <HelpPopover label="イベントの種類">
          <KeyValueList
            stacked
            pairs={[
              { name: "一覧に無い種類", value: "取り込めたレコードに 0 件" },
              {
                name: "判定しないこと",
                value: "記録の設定の外か、イベントが起きなかったか",
              },
              {
                name: "取り込めなかったレコードの種類",
                value: "一覧に無い種類の可能性あり",
              },
            ]}
          />
          {note === undefined ? null : (
            <>
              <strong className="block">入力形式の記録の範囲</strong>
              <KeyValueList stacked pairs={note} />
            </>
          )}
        </HelpPopover>
      </h3>
      <FetchStateView state={state} loadingDescription="イベントの種類の集計中">
        {(response) => (
          <>
            {response.kinds.length === 0 ? (
              <p role="status">イベントの種類を持つレコードなし</p>
            ) : (
              <table>
                <caption className="sr-only">イベントの種類</caption>
                <thead>
                  <tr>
                    <th scope="col">分類</th>
                    <th scope="col">動作</th>
                    <th scope="col">件数</th>
                  </tr>
                </thead>
                <tbody>
                  {response.kinds.map((kind) => (
                    <tr key={`${kind.category}\u0000${kind.action ?? ""}`}>
                      <td>
                        <RawText text={kind.category} />
                      </td>
                      <td>
                        {kind.action === undefined ? (
                          <MissingValue description="フィールドなし" />
                        ) : (
                          <RawText text={kind.action} />
                        )}
                      </td>
                      <td>{formatCount(kind.recordCount)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}
            <KeyValueList
              pairs={[
                {
                  name: "種類なし",
                  value: formatCount(response.uncategorizedRecordCount),
                },
                {
                  name: "取り込めなかったレコード",
                  value:
                    failedRecordCount === undefined ? (
                      <MissingValue description="件数を取得できない" />
                    ) : (
                      formatCount(failedRecordCount)
                    ),
                },
              ]}
            />
          </>
        )}
      </FetchStateView>
      {isProxyFormat(source.formatKey) ? (
        <ProxyBypassCounts
          source={source}
          matchConditions={matchConditions}
          dataVersion={dataVersion}
        />
      ) : null}
    </section>
  );
}

/** 収集元の事象の種別を取得する。応答の収集元が要求と違うときは取得の失敗にする。 */
function useSourceEventKinds(
  source: SourceIdentity,
  matchConditions: MatchConditionSelection,
  dataVersion: number,
): FetchState<EventKindsResponse> {
  const [state, setState] = useState<FetchState<EventKindsResponse>>({
    status: "loading",
  });
  const { sourceId, contentSha256 } = source;
  useEffect(() => {
    // dataVersion が変わると取り込み結果が変わりうるため、取り直す。
    void dataVersion;
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchEventKinds(
      { matchConditions, source: { sourceId, contentSha256 } },
      { signal: controller.signal },
    )
      .then((result) => {
        if (controller.signal.aborted) return;
        if (!result.ok) {
          setState({ status: "failed", failure: result.failure });
          return;
        }
        setState(
          result.value.sourceId === sourceId
            ? { status: "loaded", value: result.value }
            : {
                status: "failed",
                failure: buildFetchFailure("unexpected", failureSummary),
              },
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
  }, [sourceId, contentSha256, matchConditions, dataVersion]);
  return state;
}

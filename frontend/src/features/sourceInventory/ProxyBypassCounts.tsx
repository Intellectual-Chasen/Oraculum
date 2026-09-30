import { useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import { fetchProxyBypass } from "@/shared/api/proxyBypass";
import type {
  ProxyBypassDestination,
  ProxyBypassResponse,
} from "@/shared/contracts/proxyBypass";
import type { SourceIdentity } from "@/shared/contracts/sources";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";

const failureSummary = "Proxy の経由の件数の取得";

const uncountedDescription = "Proxy のアドレスなし";

const noSourceDescription = "接続を記録する収集元なし";

/** 接続先の名前。port を記録していない接続先はアドレスだけを書く。 */
function destinationName(destination: ProxyBypassDestination): string {
  const address = toVisibleRawText(destination.address);
  return destination.port === undefined
    ? address
    : `${address}:${destination.port}`;
}

/**
 * Proxy のログの収集元について、接続元のアドレスごとに、Proxy のログが記録した要求と、
 * 他の収集元のレコードが記録した接続を件数で並べる。
 *
 * **Proxy を経由しない接続は、Proxy のログの記録の範囲の外である。** 端末の記録が示す
 * Proxy のアドレス以外への接続を、Proxy の記録と同じ表に並べる。
 */
export function ProxyBypassCounts({
  source,
  matchConditions,
  dataVersion,
}: {
  source: SourceIdentity;
  matchConditions: MatchConditionSelection;
  dataVersion: number;
}) {
  const state = useProxyBypass(source, matchConditions, dataVersion);
  return (
    <FetchStateView state={state} loadingDescription="Proxy の経由の集計中">
      {(response) => {
        // Proxy のアドレスが無いと、backend は接続を数えずに 0 を返す。
        const counted = response.proxyAddresses.length > 0;
        return (
          <>
            <h4>
              Proxy の経由
              <HelpPopover label="Proxy の経由">
                <KeyValueList
                  stacked
                  pairs={[
                    {
                      name: "Proxy を経由しない接続",
                      value:
                        "他の収集元のレコードが記録した接続元から Proxy のアドレス以外への接続",
                    },
                    { name: "件数の単位", value: "レコード" },
                    {
                      name: "1 つの接続を記録した複数のレコード",
                      value: "レコードごとに 1 件",
                    },
                    {
                      name: "割り当てた端末",
                      value:
                        "適用期間を問わず、このアドレスに結んだ端末のすべて",
                    },
                  ]}
                />
              </HelpPopover>
            </h4>
            <KeyValueList
              pairs={[
                {
                  name: "Proxy のアドレス",
                  value: counted ? (
                    <RawText text={response.proxyAddresses.join(", ")} />
                  ) : (
                    <MissingValue description="端末の IP なし" />
                  ),
                },
                { name: "接続元", value: formatCount(response.clients.length) },
                {
                  name: "接続元が IP でないレコード",
                  value:
                    response.unreadableProxyRecordCount > 0
                      ? formatCount(response.unreadableProxyRecordCount)
                      : undefined,
                },
              ]}
            />
            <table>
              <caption className="sr-only">接続元ごとの Proxy の経由</caption>
              <thead>
                <tr>
                  <th scope="col">接続元 IP</th>
                  <th scope="col">割り当てた端末</th>
                  <th scope="col">Proxy ログの要求</th>
                  <th scope="col">Proxy への接続</th>
                  <th scope="col">Proxy を通らない接続</th>
                  <th scope="col">Proxy を通らない接続の接続先</th>
                </tr>
              </thead>
              <tbody>
                {response.clients.map((client) => (
                  <tr key={client.clientIp}>
                    <td>
                      <RawText text={client.clientIp} />
                    </td>
                    <td>
                      {client.terminals.length === 0 ? (
                        <MissingValue description="割り当てなし" />
                      ) : (
                        <RawText text={client.terminals.join(", ")} />
                      )}
                    </td>
                    <td>{formatCount(client.proxyRequestCount)}</td>
                    {counted && client.directConnectionCountable ? (
                      <>
                        <td>{formatCount(client.proxyConnectionCount)}</td>
                        <td>{formatCount(client.directConnectionCount)}</td>
                        <td>
                          <KeyValueList
                            stacked
                            pairs={client.directDestinations.map(
                              (destination) => ({
                                key: `${destination.address}\u0000${destination.port ?? ""}`,
                                name: destinationName(destination),
                                value: formatCount(destination.recordCount),
                              }),
                            )}
                          />
                        </td>
                      </>
                    ) : (
                      [0, 1, 2].map((column) => (
                        <td key={column}>
                          <MissingValue
                            description={
                              counted
                                ? noSourceDescription
                                : uncountedDescription
                            }
                          />
                        </td>
                      ))
                    )}
                  </tr>
                ))}
              </tbody>
            </table>
          </>
        );
      }}
    </FetchStateView>
  );
}

/** 比較を取得する。応答の収集元が要求と違うときは取得の失敗にする。 */
function useProxyBypass(
  source: SourceIdentity,
  matchConditions: MatchConditionSelection,
  dataVersion: number,
): FetchState<ProxyBypassResponse> {
  const [state, setState] = useState<FetchState<ProxyBypassResponse>>({
    status: "loading",
  });
  const { sourceId, contentSha256 } = source;
  useEffect(() => {
    // dataVersion が変わると端末の割当が変わりうるため、取り直す。
    void dataVersion;
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchProxyBypass(
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

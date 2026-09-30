import { ArrowDownWideNarrow, Clock, FileText, Server } from "lucide-react";
import { type ReactNode, useEffect, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { NodeRef } from "@/shared/api/graph";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import {
  fetchTerminalDetail,
  fetchTerminalEvents,
  type TerminalEventFilter,
} from "@/shared/api/terminals";
import type {
  RecordField,
  RecordLocator,
  Timestamp,
} from "@/shared/contracts/common";
import type {
  RemoteLogonOutcome,
  TerminalAddressPeriod,
  TerminalCategory,
  TerminalCategoryCount,
  TerminalDetail as TerminalDetailValue,
  TerminalEvent,
  TerminalEventsResponse,
  TerminalProfileEntry,
  TerminalRemoteLogonSummary,
} from "@/shared/contracts/terminals";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { readRecordFieldRawText } from "@/shared/lib/recordField";
import { terminalLabel } from "@/shared/lib/terminalSource";
import { DataTable } from "@/shared/ui/DataTable";
import { offsetText } from "@/shared/ui/DisplayOffset";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Fold } from "@/shared/ui/Fold";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { RecordFieldList } from "@/shared/ui/RecordFieldList";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TimestampText } from "@/shared/ui/TimestampText";
import { operatingSystemAbsent } from "./TerminalList";

const categoryLabels: Record<TerminalCategory, string> = {
  remote_logon: "別の端末からのログオン",
  account_management: "アカウントの作成・変更",
  program_execution: "プログラムの実行",
  task_service_registration: "タスクとサービスの登録",
  powershell: "PowerShell のスクリプト",
  installation: "インストール",
  defense_evasion: "保護の無効化とログの消去",
};

const logonOutcomeLabels: Record<RemoteLogonOutcome, string> = {
  failure: "失敗",
  success: "成功",
  connection: "認証の前の接続",
};

/** インストールの行に足して出す欄の語彙の項目。インストーラが書いた結果の文を指す。 */
const installationSemantics = new Set(["event.message"]);

/**
 * レコードの行に出す欄の語彙の項目。イベント ID・アカウント・ファイル・製品・サービス・タスク・
 * コマンドを指す。
 */
const mainSemantics = new Set([
  "windows_event.id",
  "account.name",
  "target_account.name",
  "subject_account.name",
  "event.account_name",
  "process.user_name",
  "file.path",
  "process.binary_path",
  "file.product_name",
  "file.product_version",
  "process_binary.product",
  "file.original_file_name",
  "service.name",
  "scheduled_task.name",
  "scheduled_task.command",
  "process.command_line",
]);

/**
 * registry の DWORD の文字列 (`<10 進> (0x<16 進>)`) の ActiveTimeBias から、UTC からのずれの
 * 文字列を返す。bias は UTC から地方時を引いた分であり、符号なしで記録される。
 * 読めないときは `undefined` を返す。
 */
export function offsetFromActiveTimeBias(value: string): string | undefined {
  const match = /^\s*(-?\d+)/.exec(value);
  if (match === null) return undefined;
  let bias = Number(match[1]);
  if (bias >= 2 ** 31) bias -= 2 ** 32;
  if (!Number.isSafeInteger(bias) || Math.abs(bias) > 24 * 60) {
    return undefined;
  }
  return offsetText(-bias);
}

type Callbacks = {
  onSelectRecord: (ref: RecordLocator) => void;
  onSelectNode: (node: NodeRef) => void;
};

function RecordButton({
  recordRef,
  label = "レコードを開く",
  onSelectRecord,
}: {
  recordRef: RecordLocator;
  label?: string;
  onSelectRecord: (ref: RecordLocator) => void;
}) {
  return (
    <IconButton label={label} onPress={() => onSelectRecord(recordRef)}>
      <FileText size={14} aria-hidden="true" />
    </IconButton>
  );
}

function TimeOrMissing({ time }: { time: Timestamp | undefined }) {
  return time === undefined ? (
    <MissingValue description="時刻なし" />
  ) : (
    <TimestampText timestamp={time} />
  );
}

/** 期間を「始まり – 終わり」で出す。 */
function Period({
  from,
  to,
}: {
  from: Timestamp | undefined;
  to: Timestamp | undefined;
}) {
  return (
    <>
      <TimeOrMissing time={from} /> – <TimeOrMissing time={to} />
    </>
  );
}

/** registry などから読み取った「名前・値」の表。1 行が 1 件の値である。 */
function ProfileTable({
  label,
  entries,
  empty,
  onSelectRecord,
}: {
  label: string;
  entries: TerminalProfileEntry[];
  empty: string;
  onSelectRecord: (ref: RecordLocator) => void;
}) {
  if (entries.length === 0) return <p>{empty}</p>;
  return (
    <DataTable
      label={label}
      rows={entries}
      rowKey={(entry) => `${entry.name}\u0000${entry.value}`}
      columns={[
        {
          key: "name",
          header: "名前",
          cell: (entry) => <RawText text={entry.name} />,
        },
        {
          key: "value",
          header: "値",
          cell: (entry) => <RawText text={entry.value} />,
        },
        {
          key: "record",
          header: "レコード",
          cell: (entry) => (
            <RecordButton
              recordRef={entry.recordRef}
              onSelectRecord={onSelectRecord}
            />
          ),
        },
      ]}
    />
  );
}

/**
 * IP アドレスの表の 1 行。インターフェースの 1 件の値である。値を持たない期間は、entry を持たない
 * 1 行にする。
 */
type AddressRow = {
  address: TerminalAddressPeriod;
  entry: TerminalProfileEntry | undefined;
};

function Attributes({
  detail,
  onSelectRecord,
  onApplyDisplayOffset,
}: {
  detail: TerminalDetailValue;
  onSelectRecord: (ref: RecordLocator) => void;
  onApplyDisplayOffset: (offset: string) => void;
}) {
  const bias = detail.timeZone.find((entry) => entry.name === "ActiveTimeBias");
  const offset =
    bias === undefined ? undefined : offsetFromActiveTimeBias(bias.value);
  const addressRows = detail.addresses.flatMap((address): AddressRow[] =>
    address.values.length === 0
      ? [{ address, entry: undefined }]
      : address.values.map((entry) => ({ address, entry })),
  );
  return (
    <section>
      <h3 className="section-title">端末の情報</h3>
      <h4>OS</h4>
      <ProfileTable
        label="OS"
        entries={detail.operatingSystem}
        empty={operatingSystemAbsent}
        onSelectRecord={onSelectRecord}
      />
      <h4>名前の履歴</h4>
      {detail.names.length === 0 ? (
        <p>名前を記録したレコードなし</p>
      ) : (
        <DataTable
          label="名前の履歴"
          rows={detail.names}
          rowKey={(entry) => entry.name}
          columns={[
            {
              key: "name",
              header: "名前",
              cell: (entry) => <RawText text={entry.name} />,
            },
            {
              key: "period",
              header: "期間",
              mono: true,
              cell: (entry) => <Period from={entry.first} to={entry.last} />,
            },
            {
              key: "count",
              header: "レコード数",
              numeric: true,
              cell: (entry) => formatCount(entry.recordRefs.length),
            },
            {
              key: "record",
              header: "レコード",
              cell: (entry) =>
                entry.recordRefs[0] === undefined ? null : (
                  <RecordButton
                    recordRef={entry.recordRefs[0]}
                    label="最初のレコードを開く"
                    onSelectRecord={onSelectRecord}
                  />
                ),
            },
          ]}
        />
      )}
      <h4>IP アドレス</h4>
      {addressRows.length === 0 ? (
        <p>IP アドレスを記録したレコードなし</p>
      ) : (
        <DataTable
          label="IP アドレス"
          rows={addressRows}
          rowKey={({ address, entry }) =>
            `${address.interface}\u0000${address.from?.rawText ?? ""}\u0000${entry?.name ?? ""}\u0000${entry?.value ?? ""}`
          }
          columns={[
            {
              key: "interface",
              header: "インターフェース",
              cell: ({ address }) => <RawText text={address.interface} />,
            },
            {
              key: "period",
              header: "期間",
              mono: true,
              cell: ({ address }) =>
                address.from === undefined ? (
                  <MissingValue description="割り当ての時刻なし" />
                ) : (
                  <Period from={address.from} to={address.to} />
                ),
            },
            {
              key: "name",
              header: "名前",
              cell: ({ entry }) =>
                entry === undefined ? null : <RawText text={entry.name} />,
            },
            {
              key: "value",
              header: "値",
              mono: true,
              cell: ({ entry }) =>
                entry === undefined ? (
                  <MissingValue description="値なし" />
                ) : (
                  <RawText text={entry.value} />
                ),
            },
            {
              key: "record",
              header: "レコード",
              cell: ({ entry }) =>
                entry === undefined ? null : (
                  <RecordButton
                    recordRef={entry.recordRef}
                    onSelectRecord={onSelectRecord}
                  />
                ),
            },
          ]}
        />
      )}
      <h4>タイムゾーン</h4>
      <ProfileTable
        label="タイムゾーン"
        entries={detail.timeZone}
        empty="タイムゾーンを記録したレコードなし"
        onSelectRecord={onSelectRecord}
      />
      {offset === undefined ? null : (
        <div className="flex items-center gap-1">
          <KeyValueList
            pairs={[
              {
                name: "ActiveTimeBias のタイムゾーン",
                value: `UTC${offset}`,
              },
            ]}
          />
          <IconButton
            label="表示のタイムゾーンに適用"
            onPress={() => onApplyDisplayOffset(offset)}
          >
            <Clock size={14} aria-hidden="true" />
          </IconButton>
          <HelpPopover label="ActiveTimeBias のタイムゾーン">
            <KeyValueList
              stacked
              pairs={[{ name: "夏時間の切り替えの履歴", value: "不使用" }]}
            />
          </HelpPopover>
        </div>
      )}
    </section>
  );
}

type SortKey = "failureCount" | "successCount" | "connectionCount";
const sortLabels: Record<SortKey, string> = {
  failureCount: logonOutcomeLabels.failure,
  successCount: logonOutcomeLabels.success,
  connectionCount: logonOutcomeLabels.connection,
};

function RemoteLogons({
  terminalId,
  logons,
  matchConditions,
  version,
  callbacks,
}: {
  terminalId: string;
  logons: TerminalRemoteLogonSummary[];
  matchConditions: MatchConditionSelection;
  version: number;
  callbacks: Callbacks;
}) {
  const [sortKey, setSortKey] = useState<SortKey>("failureCount");
  const [sourceIp, setSourceIp] = useState<string | undefined>(undefined);
  if (logons.length === 0) {
    return (
      <section>
        <h3 className="section-title">別の端末からのログオン</h3>
        <p>レコードなし</p>
      </section>
    );
  }
  const sorted = [...logons].sort(
    (left, right) => right[sortKey] - left[sortKey],
  );
  // 件数の button の読み上げの名前に、接続元と何の件数かを入れる。
  const countButton = (logon: TerminalRemoteLogonSummary, key: SortKey) => (
    <button
      type="button"
      aria-label={`${logon.sourceIp} の${sortLabels[key]}: ${formatCount(logon[key])}`}
      onClick={() => setSourceIp(logon.sourceIp)}
    >
      {formatCount(logon[key])}
    </button>
  );
  return (
    <section>
      <h3 className="section-title">別の端末からのログオン</h3>
      <table>
        <caption>{`接続元: ${formatCount(logons.length)}`}</caption>
        <thead>
          <tr>
            <th scope="col">接続元の IP アドレス</th>
            {(Object.keys(sortLabels) as SortKey[]).map((key) => (
              <th scope="col" key={key}>
                <button
                  type="button"
                  aria-pressed={sortKey === key}
                  onClick={() => setSortKey(key)}
                >
                  {sortLabels[key]}
                  <ArrowDownWideNarrow size={14} aria-hidden="true" />
                </button>
              </th>
            ))}
            <th scope="col">失敗したアカウント</th>
            <th scope="col">成功したアカウント</th>
            <th scope="col">期間</th>
            <th scope="col">レコード</th>
          </tr>
        </thead>
        <tbody>
          {sorted.map((logon) => (
            <tr
              key={logon.sourceIp}
              aria-current={logon.sourceIp === sourceIp ? "true" : undefined}
            >
              <th scope="row">
                <RawText text={logon.sourceIp} />
              </th>
              <td>{countButton(logon, "failureCount")}</td>
              <td>{countButton(logon, "successCount")}</td>
              <td>{countButton(logon, "connectionCount")}</td>
              <td>
                <RawText text={logon.attemptedAccounts.join(", ")} />
              </td>
              <td>
                <RawText text={logon.loggedOnAccounts.join(", ")} />
              </td>
              <td className="font-mono">
                <Period from={logon.first} to={logon.last} />
              </td>
              <td>
                {logon.firstRecordRef === undefined ? null : (
                  <RecordButton
                    recordRef={logon.firstRecordRef}
                    label="最初のレコードを開く"
                    onSelectRecord={callbacks.onSelectRecord}
                  />
                )}
                {logon.lastRecordRef === undefined ? null : (
                  <RecordButton
                    recordRef={logon.lastRecordRef}
                    label="最後のレコードを開く"
                    onSelectRecord={callbacks.onSelectRecord}
                  />
                )}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
      {sourceIp === undefined ? null : (
        <>
          <h4>
            接続元: <RawText text={sourceIp} />
          </h4>
          <EventList
            key={sourceIp}
            terminalId={terminalId}
            filter={{ sourceIp }}
            matchConditions={matchConditions}
            version={version}
            callbacks={callbacks}
          />
        </>
      )}
    </section>
  );
}

function EventRow({
  event,
  callbacks,
}: {
  event: TerminalEvent;
  callbacks: Callbacks;
}) {
  const installation = event.categories.includes("installation");
  const fields = event.fields.filter(
    (field: RecordField) =>
      field.semantic !== undefined &&
      (mainSemantics.has(field.semantic) ||
        (installation && installationSemantics.has(field.semantic))),
  );
  return (
    <li>
      <button
        type="button"
        onClick={() => callbacks.onSelectRecord(event.recordRef)}
      >
        {event.eventTime === undefined ? (
          "時刻の無いレコード"
        ) : (
          <TimestampText timestamp={event.eventTime} />
        )}
      </button>
      {event.logonOutcome === undefined ? null : (
        <span> 結果: {logonOutcomeLabels[event.logonOutcome]}</span>
      )}
      {event.originalFileNameDiffers ? (
        <strong> 元のファイル名と不一致</strong>
      ) : null}
      {fields.length === 0 ? null : (
        <RecordFieldList fields={fields} readValue={readRecordFieldRawText} />
      )}
      {event.otherEventTimes.length === 0 ? null : (
        <p>
          ほかの実行時刻:{" "}
          {event.otherEventTimes.map((time, index) => (
            // 同じ時刻が並ぶことがあるため、位置で区別する。
            // biome-ignore lint/suspicious/noArrayIndexKey: 並びは応答のまま変わらない。
            <span key={index}>
              {index === 0 ? null : "、"}
              <TimestampText timestamp={time} />
            </span>
          ))}
        </p>
      )}
      {event.namedNodes.length === 0 ? null : (
        <p>
          関連するノード:{" "}
          {event.namedNodes.map((node) => (
            <button
              key={node.id}
              type="button"
              onClick={() =>
                callbacks.onSelectNode({
                  id: node.id,
                  label: terminalLabel(node),
                })
              }
            >
              <RawText text={terminalLabel(node)} />
            </button>
          ))}
        </p>
      )}
    </li>
  );
}

const eventsFailureSummary = "端末のレコードの取得";

/** 条件に合う端末のレコードを時刻の順に並べ、続きを取る操作を出す。 */
function EventList({
  terminalId,
  filter,
  matchConditions,
  version,
  callbacks,
}: {
  terminalId: string;
  filter: TerminalEventFilter;
  matchConditions: MatchConditionSelection;
  version: number;
  callbacks: Callbacks;
}) {
  const [state, setState] = useState<FetchState<TerminalEventsResponse>>({
    status: "loading",
  });
  const { category, sourceIp } = filter;
  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じ条件で取り直す。
  useEffect(() => {
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchTerminalEvents(terminalId, { category, sourceIp }, matchConditions, {
      signal: controller.signal,
    }).then(
      (result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        );
      },
      () => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure("unexpected", eventsFailureSummary),
        });
      },
    );
    return () => controller.abort();
  }, [terminalId, category, sourceIp, matchConditions, version]);

  return (
    <FetchStateView state={state} loadingDescription="レコードを読み込み中">
      {(response) =>
        response.events.length === 0 ? (
          <p role="status">条件に一致するレコードなし</p>
        ) : (
          <>
            <KeyValueList
              pairs={[
                {
                  name: "レコード",
                  value: formatCount(response.events.length),
                },
              ]}
            />
            <ul>
              {response.events.map((event) => (
                <EventRow
                  key={`${event.recordRef.sourceId}\u0000${event.recordRef.recordRawTextRef}`}
                  event={event}
                  callbacks={callbacks}
                />
              ))}
            </ul>
          </>
        )
      }
    </FetchStateView>
  );
}

function CategorySection({
  terminalId,
  count,
  matchConditions,
  version,
  callbacks,
}: {
  terminalId: string;
  count: TerminalCategoryCount;
  matchConditions: MatchConditionSelection;
  version: number;
  callbacks: Callbacks;
}) {
  const label = categoryLabels[count.category];
  let body: ReactNode;
  if (count.recordCount > 0) {
    body = (
      <Fold
        summary={`${label}: ${formatCount(count.recordCount)}`}
        rowCount={count.recordCount}
      >
        <EventList
          terminalId={terminalId}
          filter={{ category: count.category }}
          matchConditions={matchConditions}
          version={version}
          callbacks={callbacks}
        />
      </Fold>
    );
  } else if (count.presentSources.length === 0) {
    // 必要な収集元が無いときは、記録が無いのか収集していないのかを判定できない。
    body = (
      <KeyValueList
        pairs={[
          { name: label, value: "0" },
          {
            name: "必要な収集元",
            value: (
              <StatusLabel
                status="idle"
                label="なし"
                details={
                  <KeyValueList
                    stacked
                    pairs={[
                      {
                        name: "必要な収集元",
                        value: (
                          <RawText text={count.requiredSources.join(", ")} />
                        ),
                      },
                      { name: "記録の有無", value: "判定不可" },
                    ]}
                  />
                }
              />
            ),
          },
        ]}
      />
    );
  } else {
    body = (
      <KeyValueList
        pairs={[
          { name: label, value: "0" },
          {
            name: "収集元",
            value: <RawText text={count.presentSources.join(", ")} />,
          },
        ]}
      />
    );
  }
  return <li>{body}</li>;
}

/**
 * 端末 1 台の属性・別の端末からのログオン・分類ごとのレコードを出す。
 * 値ごとに根拠のレコードを開く操作と、関わるノードをグラフで出す操作を持つ。
 */
export function TerminalDetail({
  terminalId,
  matchConditions,
  version,
  onSelectRecord,
  onSelectNode,
  onApplyDisplayOffset,
  onOpenHosts,
}: {
  /** 選んでいない間は `undefined` である。 */
  terminalId: string | undefined;
  matchConditions: MatchConditionSelection;
  version: number;
  onSelectRecord: (ref: RecordLocator) => void;
  onSelectNode: (node: NodeRef) => void;
  onApplyDisplayOffset: (offset: string) => void;
  /** 端末を選ぶ Hosts の区画を前面に出す。渡さないときは button を出さない。 */
  onOpenHosts?: () => void;
}) {
  const [state, setState] = useState<FetchState<TerminalDetailValue>>({
    status: "loading",
  });
  // biome-ignore lint/correctness/useExhaustiveDependencies: version が変わったときに、同じ条件で取り直す。
  useEffect(() => {
    if (terminalId === undefined) return;
    const controller = new AbortController();
    setState({ status: "loading" });
    fetchTerminalDetail(terminalId, matchConditions, {
      signal: controller.signal,
    }).then(
      (result) => {
        if (controller.signal.aborted) return;
        setState(
          result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        );
      },
      () => {
        if (controller.signal.aborted) return;
        setState({
          status: "failed",
          failure: buildFetchFailure("unexpected", "端末の情報の取得"),
        });
      },
    );
    return () => controller.abort();
  }, [terminalId, matchConditions, version]);

  if (terminalId === undefined) {
    return (
      <p className="flex items-center gap-1.5">
        <span role="status">端末: 未選択</span>
        {onOpenHosts === undefined ? null : (
          <IconButton label="Hosts を表示" onPress={onOpenHosts}>
            <Server className="size-3.5" aria-hidden="true" />
          </IconButton>
        )}
      </p>
    );
  }
  const callbacks = { onSelectRecord, onSelectNode };
  return (
    <FetchStateView state={state} loadingDescription="端末の情報を読み込み中">
      {(detail) => (
        <article className="terminal-detail">
          <h2 className="host-title">
            <RawText text={terminalLabel(detail.node)} />
          </h2>
          <Attributes
            detail={detail}
            onSelectRecord={onSelectRecord}
            onApplyDisplayOffset={onApplyDisplayOffset}
          />
          <RemoteLogons
            key={detail.node.id}
            terminalId={detail.node.id}
            logons={detail.remoteLogons}
            matchConditions={matchConditions}
            version={version}
            callbacks={callbacks}
          />
          <section>
            <h3 className="section-title">分類ごとのレコード</h3>
            <ul>
              {detail.categories.map((count) => (
                <CategorySection
                  key={`${detail.node.id}\u0000${count.category}`}
                  terminalId={detail.node.id}
                  count={count}
                  matchConditions={matchConditions}
                  version={version}
                  callbacks={callbacks}
                />
              ))}
            </ul>
          </section>
        </article>
      )}
    </FetchStateView>
  );
}

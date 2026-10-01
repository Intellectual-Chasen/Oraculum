import type {
  AccountRelation,
  AccountRelationKey,
  AccountRelationsResponse,
} from "@/shared/contracts/accountRelations";
import type { RecordLocator } from "@/shared/contracts/common";
import type { GraphNode } from "@/shared/contracts/graph";
import type { FetchFailure, FetchState } from "@/shared/lib/fetchState";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { RawText } from "@/shared/ui/RawText";
import { edgeKindLabels } from "./labels";

function accountLabel(node: GraphNode): string {
  return (
    node.label.rawText ??
    node.label.normalized ??
    node.identity.map((item) => item.value).join(" / ")
  );
}

function relationKey(group: AccountRelationKey): string {
  return `${group.counterpartId}\u0000${group.originRole}\u0000${group.otherRole}`;
}

function timeText(group: AccountRelation): string {
  const first = group.firstTime?.normalized ?? group.firstTime?.rawText;
  const last = group.lastTime?.normalized ?? group.lastTime?.rawText;
  return first === undefined
    ? "時刻不明"
    : first === last
      ? first
      : `${first} – ${last ?? "?"}`;
}

export function AccountRelationsFigure({
  state,
  selected,
}: {
  state: FetchState<AccountRelationsResponse>;
  selected: AccountRelationKey | undefined;
}) {
  return (
    <div className="account-relations-figure">
      <FetchStateView
        state={state}
        loadingDescription="相手アカウントの読み込み中"
      >
        {(response) => {
          const height = Math.max(240, response.groups.length * 86 + 68);
          const selectedGroup = response.groups.find(
            (group) =>
              relationKey(group) ===
              (selected === undefined ? "" : relationKey(selected)),
          );
          return (
            <>
              <p>
                起点から同じレコードを介して名指された相手です。線は集計上のつながりです。
              </p>
              <svg
                viewBox={`0 0 900 ${height}`}
                role="img"
                aria-label={`${accountLabel(response.origin)} から ${response.groups.length} 組の相手への関係図`}
              >
                <title>起点アカウントと相手候補の集計</title>
                {response.groups.map((group, index) => {
                  const y = 75 + index * 86;
                  const active = selectedGroup === group;
                  return (
                    <g key={relationKey(group)}>
                      <path
                        d={`M 135 75 L 420 ${y} L 680 ${y}`}
                        fill="none"
                        stroke={active ? "#cf6d24" : "#677787"}
                        strokeWidth={active ? 3 : 1.5}
                      />
                      <rect
                        x="370"
                        y={y - 21}
                        width="100"
                        height="42"
                        rx="5"
                        fill={active ? "#f4b766" : "#dce6ed"}
                        stroke="#445464"
                      />
                      <text x="420" y={y + 5} textAnchor="middle" fontSize="14">
                        {group.recordCount} 件
                      </text>
                      <circle
                        cx="698"
                        cy={y}
                        r="18"
                        fill={active ? "#f4b766" : "#b6d6e0"}
                        stroke="#445464"
                      />
                      <text x="725" y={y + 5} fontSize="13">
                        {accountLabel(group.counterpart).slice(0, 23)}
                      </text>
                      <title>{`${accountLabel(group.counterpart)} / ${edgeKindLabels[group.originRole]} → ${edgeKindLabels[group.otherRole]} / ${group.recordCount} 件`}</title>
                    </g>
                  );
                })}
                <circle
                  cx="115"
                  cy="75"
                  r="20"
                  fill="#a2bce8"
                  stroke="#445464"
                />
                <text x="30" y="120" fontSize="14">
                  {accountLabel(response.origin).slice(0, 24)}
                </text>
              </svg>
              {selectedGroup === undefined ? null : (
                <>
                  <p>
                    選択中:{" "}
                    <RawText text={accountLabel(selectedGroup.counterpart)} />
                    。以下の矢印はレコードが各アカウントを名指した向きです。
                  </p>
                  <div className="account-relations-record-figure">
                    <svg
                      viewBox={`0 0 900 ${Math.max(140, (response.records?.length ?? 0) * 48 + 60)}`}
                      role="img"
                      aria-label={`${response.records?.length ?? 0} 件の個別レコードと両アカウントを結ぶ図`}
                    >
                      <defs>
                        <marker
                          id="account-relation-arrow"
                          markerWidth="8"
                          markerHeight="8"
                          refX="7"
                          refY="4"
                          orient="auto"
                        >
                          <path d="M 0 0 L 8 4 L 0 8 z" fill="#677787" />
                        </marker>
                      </defs>
                      <text x="25" y="26" fontSize="14">
                        起点: {accountLabel(response.origin).slice(0, 20)}
                      </text>
                      <text x="680" y="26" fontSize="14">
                        相手:{" "}
                        {accountLabel(selectedGroup.counterpart).slice(0, 20)}
                      </text>
                      {(response.records ?? []).map((record, index) => {
                        const y = index * 48 + 62;
                        return (
                          <g key={record.nodeId}>
                            <path
                              d={`M 424 ${y} L 118 ${y}`}
                              stroke="#677787"
                              markerEnd="url(#account-relation-arrow)"
                            />
                            <path
                              d={`M 476 ${y} L 735 ${y}`}
                              stroke="#677787"
                              markerEnd="url(#account-relation-arrow)"
                            />
                            <circle
                              cx="450"
                              cy={y}
                              r="17"
                              fill="#dce6ed"
                              stroke="#445464"
                            />
                            <text
                              x="450"
                              y={y + 5}
                              textAnchor="middle"
                              fontSize="12"
                            >
                              {index + 1}
                            </text>
                            <text x="125" y={y - 5} fontSize="11">
                              {edgeKindLabels[selectedGroup.originRole]}
                            </text>
                            <text x="545" y={y - 5} fontSize="11">
                              {edgeKindLabels[selectedGroup.otherRole]}
                            </text>
                            <title>{`Event ID ${record.summary.eventAction || "なし"} / ${record.summary.eventRecordId || record.summary.recordHeaderId || record.summary.recordRef.recordRawTextRef}`}</title>
                          </g>
                        );
                      })}
                    </svg>
                  </div>
                </>
              )}
            </>
          );
        }}
      </FetchStateView>
    </div>
  );
}

export function AccountRelationsDetail({
  state,
  selected,
  onSelect,
  onSelectRecord,
  onSelectNode,
  onLoadMore,
  loadingMore,
  moreFailure,
  sourceFileNames,
}: {
  state: FetchState<AccountRelationsResponse>;
  selected: AccountRelationKey | undefined;
  onSelect: (key: AccountRelationKey | undefined) => void;
  onSelectRecord: (ref: RecordLocator) => void;
  onSelectNode: (id: string) => void;
  onLoadMore: () => void;
  loadingMore: boolean;
  moreFailure?: FetchFailure;
  sourceFileNames?: ReadonlyMap<string, string>;
}) {
  return (
    <section
      className="account-relations-detail"
      aria-label="相手アカウントの候補"
    >
      <h3>同じ記録に現れた相手</h3>
      <FetchStateView
        state={state}
        loadingDescription="相手アカウントの読み込み中"
      >
        {(response) => (
          <>
            <p>
              集計条件: 同じレコードが起点と相手を役割付きで名指す。候補{" "}
              {response.groups.length}{" "}
              組。件数は各組の値で、同じレコードが複数組に入ることがあります。
            </p>
            {response.periodUnjudgedRecordCount > 0 ? (
              <p>
                期間を判定できないレコード: {response.periodUnjudgedRecordCount}{" "}
                件（候補の件数には含めません）
              </p>
            ) : null}
            {response.groups.length === 0 ? (
              <p>条件に合う相手はありません。</p>
            ) : null}
            <ul className="account-relation-list">
              {response.groups.map((group) => {
                const active =
                  selected !== undefined &&
                  relationKey(selected) === relationKey(group);
                const sources = group.sources
                  .map(
                    (source) =>
                      `${sourceFileNames?.get(source.sourceId) ?? source.sourceId}: ${source.count}`,
                  )
                  .join("、");
                return (
                  <li key={relationKey(group)}>
                    <button
                      type="button"
                      aria-expanded={active}
                      onClick={() => onSelect(active ? undefined : group)}
                    >
                      <RawText text={accountLabel(group.counterpart)} /> ·{" "}
                      {group.recordCount} 件
                    </button>
                    <dl>
                      <dt>識別鍵</dt>
                      <dd>
                        <RawText
                          text={`${group.counterpart.keyForm}: ${group.counterpart.identity.map((item) => item.value).join(" / ")}`}
                        />
                      </dd>
                      <dt>役割</dt>
                      <dd>
                        起点: {edgeKindLabels[group.originRole]} / 相手:{" "}
                        {edgeKindLabels[group.otherRole]}
                      </dd>
                      <dt>時刻</dt>
                      <dd>
                        <RawText text={timeText(group)} />
                        {group.unknownTimeCount > 0
                          ? `・時刻不明 ${group.unknownTimeCount} 件`
                          : null}
                      </dd>
                      <dt>事象</dt>
                      <dd>
                        {group.events
                          .map(
                            (event) =>
                              `${event.category || "分類なし"} / ${event.action || "動作なし"}: ${event.count}`,
                          )
                          .join("、")}
                      </dd>
                      <dt>収集元</dt>
                      <dd>
                        <RawText text={sources} />
                      </dd>
                      {group.sigmaRulePaths.length > 0 ? (
                        <>
                          <dt>Sigma 一致</dt>
                          <dd>
                            <RawText text={group.sigmaRulePaths.join("、")} />
                            （補足情報）
                          </dd>
                        </>
                      ) : null}
                    </dl>
                    {active ? (
                      <>
                        <p>
                          個別レコード: {response.records?.length ?? 0} /{" "}
                          {response.selectedRecordCount} 件表示。未表示:{" "}
                          {response.selectedRecordCount -
                            (response.records?.length ?? 0)}{" "}
                          件。
                        </p>
                        {response.nextOffset === undefined ? null : (
                          <button
                            type="button"
                            disabled={loadingMore}
                            onClick={onLoadMore}
                          >
                            {loadingMore
                              ? "読み込み中"
                              : "続きを読み込む（次の 50 件）"}
                          </button>
                        )}
                        {moreFailure === undefined ? null : (
                          <p role="alert">
                            続きを読み込めませんでした。もう一度試してください。
                          </p>
                        )}
                        <ul>
                          {(response.records ?? []).map((record) => (
                            <li key={record.nodeId}>
                              <button
                                type="button"
                                onClick={() =>
                                  onSelectRecord(record.summary.recordRef)
                                }
                              >
                                レコード{" "}
                                {record.summary.eventRecordId ||
                                  record.summary.recordHeaderId ||
                                  record.summary.recordRef
                                    .recordRawTextRef}{" "}
                                · Event ID{" "}
                                {record.summary.eventAction || "なし"}
                              </button>
                              <span>　</span>
                              <button
                                type="button"
                                onClick={() => onSelectNode(record.nodeId)}
                              >
                                ノード詳細
                              </button>
                              <p>
                                根拠エッジ:{" "}
                                <RawText text={record.originEdgeId} /> /{" "}
                                <RawText text={record.otherEdgeId} />
                              </p>
                              {record.sigmaRulePaths.length > 0 ? (
                                <p>
                                  Sigma 一致:{" "}
                                  <RawText
                                    text={record.sigmaRulePaths.join("、")}
                                  />
                                </p>
                              ) : null}
                            </li>
                          ))}
                        </ul>
                      </>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          </>
        )}
      </FetchStateView>
    </section>
  );
}

import { RefreshCw } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { NodeRef } from "@/shared/api/graph";
import { fetchInfluencePath } from "@/shared/api/influencePath";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { RecordLocator } from "@/shared/contracts/common";
import type { GraphEvidence, GraphNode } from "@/shared/contracts/graph";
import {
  type InfluenceBasis,
  type InfluenceEndpoint,
  type InfluenceFrontierReason,
  type InfluencePathResponse,
  type InfluencePathStop,
  type InfluenceTimeBasis,
  type InfluenceVertex,
  influenceBases,
} from "@/shared/contracts/influencePath";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { recordLocatorKey } from "@/shared/lib/listKey";
import { describeRecordPosition } from "@/shared/lib/recordPosition";
import { DataTable } from "@/shared/ui/DataTable";
import { DerivedLabelNote } from "@/shared/ui/DerivedLabelNote";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Highlighted } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TimestampText } from "@/shared/ui/TimestampText";
import type { CosmosLayoutSettings } from "./cosmosLayout";
import {
  candidateTallyPairs,
  edgePairConditionDescriptions,
  edgePairConditionLabels,
} from "./EdgeRecordPairs";
import { edgeKindLabels } from "./labels";
import { NodeLabelValue, NodeLabelView } from "./NodeLabelView";
import { SubgraphCanvas } from "./SubgraphCanvas";
import { SubgraphCanvasBoundary } from "./SubgraphCanvasBoundary";
import { readNodeLabel, type SubgraphDrawing } from "./subgraph";

/** 根拠の種類の短いラベル。値の間に順序を付けない。 */
export const influenceBasisLabels: Record<InfluenceBasis, string> = {
  specified_operation: "入力形式の仕様の定める操作",
  inferred_record: "仕様に定めの無い記録からの推定",
  undetermined_direction: "決められない向き",
  account_management: "アカウントの管理操作の対象",
  argument_name: "プロセスの引数に現れた名前",
  requested_destination: "プロセスの接続の接続先",
  credential_use: "資格情報の使用",
  observed: "観測したエッジ",
  candidate: "推定したエッジ",
  uncertain_chain: "未確定の推定エッジ",
  single_node: "2 つのノードの間",
  equivalence: "同じ対象の 2 つのノードの等価",
};

/** 根拠の種類の分類。根拠の種類の表の列に出す。 */
const influenceBasisAxes: Record<InfluenceBasis, string> = {
  specified_operation: "向きの決め方",
  inferred_record: "向きの決め方",
  undetermined_direction: "向きの決め方",
  account_management: "向きの決め方",
  argument_name: "向きの決め方",
  requested_destination: "向きの決め方",
  credential_use: "向きの決め方",
  observed: "元のエッジの作り方",
  candidate: "元のエッジの作り方",
  uncertain_chain: "元のエッジの作り方",
  single_node: "影響の種類",
  equivalence: "影響の種類",
};

/**
 * 画面を開いたときに除外する根拠の種類。操作の記録を持たず、レコードが対象を指すことから作った影響のエッジであり、
 * 分析者が外したときだけ経路に入る。アカウントの管理操作と資格情報の使用は、既定で経路に入れる。
 */
export const defaultExcludedBases: readonly InfluenceBasis[] = [
  "argument_name",
  "requested_destination",
];

const timeBasisLabels: Record<InfluenceTimeBasis, string> = {
  same_terminal: "1 つの端末の時刻",
  unbounded_offset: "時差の上限の無い端末の組",
  precision_width: "精度の幅で広げた区間",
};

/** 止まった理由の短いラベルと、理由が指す経路の端。 */
const stopSpecs: Record<
  InfluencePathStop,
  { label: string; end?: "起点" | "終点"; detail?: string }
> = {
  origin_without_timestamp: {
    label: "時刻を持つレコードなし",
    end: "起点",
  },
  destination_without_timestamp: {
    label: "時刻を持つレコードなし",
    end: "終点",
  },
  no_influence_route: {
    label: "影響のエッジの経路なし",
    detail: "時刻を問わない探索",
  },
  time_order_unsatisfied: {
    label: "時刻の順を満たす経路なし",
  },
  computation_limit: {
    label: "探索の上限に到達",
  },
  origin_not_influence_end: {
    label: "経路の端にならないノード",
    end: "起点",
    detail:
      "端になるエッジなし: IP・端末などのノードと影響のエッジの無いレコード",
  },
  destination_not_influence_end: {
    label: "経路の端にならないノード",
    end: "終点",
    detail:
      "端になるエッジなし: IP・端末などのノードと影響のエッジの無いレコード",
  },
  truncated_route_unverified: {
    label: "表示の上限で経路が未確認",
    detail: "表示したエッジだけで時刻の順を満たす経路は未確認",
  },
  route_through_excluded_basis: {
    label: "除外の解除で経路あり",
  },
  excluded_basis_route_limit: {
    label: "除外を解除した探索が上限に到達",
  },
};

/** 端にならないノードが IP のときの次の操作。 */
const ipEndGuides: Partial<Record<InfluencePathStop, string>> = {
  origin_not_influence_end: "その IP から来たログオンのレコードを起点に選択",
  destination_not_influence_end:
    "その IP から来たログオンのレコードを終点に選択",
};

/** 起点か終点を経路の端に使えず、backend が経路を求めなかった理由。 */
const unsearchedStops: ReadonlySet<InfluencePathStop> = new Set([
  "origin_without_timestamp",
  "destination_without_timestamp",
  "origin_not_influence_end",
  "destination_not_influence_end",
]);

const frontierReasonLabels: Record<InfluenceFrontierReason, string> = {
  no_outgoing_influence: "先への影響の記録なし",
  outgoing_time_unsatisfied: "届いた時刻より後のエッジなし",
  outgoing_excluded: "すべてのエッジが除外した根拠",
};

/** 求める影響の経路の起点と終点。 */
export type InfluenceEnds = { from: NodeRef; to: NodeRef };

/**
 * 影響の経路を取得する。`ends` が無い間は取得せず、undefined を返す。
 *
 * **取得の状態を要求の組と一緒に持つ。** 起点か終点を変えた直後の描画で、前の経路の応答を
 * 新しい起点と終点の結果として出さない。
 */
export function useInfluencePath(
  ends: InfluenceEnds | undefined,
  excludedBases: readonly InfluenceBasis[],
  matchConditions: MatchConditionSelection,
  version: number,
): FetchState<InfluencePathResponse> | undefined {
  const from = ends?.from.id;
  const to = ends?.to.id;
  const request = useMemo(
    () =>
      from === undefined || to === undefined
        ? undefined
        : { from, to, excludedBases, matchConditions, version },
    [from, to, excludedBases, matchConditions, version],
  );
  const [fetched, setFetched] = useState<{
    request: object;
    state: FetchState<InfluencePathResponse>;
  }>();
  useEffect(() => {
    if (request === undefined) return;
    const { from, to, excludedBases, matchConditions } = request;
    const controller = new AbortController();
    fetchInfluencePath(
      { from, to, excludedBases, matchConditions },
      controller.signal,
    )
      .then((result) => {
        if (controller.signal.aborted) return;
        setFetched({
          request,
          state: result.ok
            ? { status: "loaded", value: result.value }
            : { status: "failed", failure: result.failure },
        });
      })
      .catch(() => {
        if (controller.signal.aborted) return;
        setFetched({
          request,
          state: {
            status: "failed",
            failure: buildFetchFailure("unexpected", "影響の経路の取得"),
          },
        });
      });
    return () => controller.abort();
  }, [request]);
  if (request === undefined) return undefined;
  return fetched?.request === request ? fetched.state : { status: "loading" };
}

/** 応答の vertices と先へ進まないノードから、key の要素を探す。 */
function vertexOf(
  response: InfluencePathResponse,
  key: string,
): InfluenceVertex | undefined {
  return (
    response.vertices.find((vertex) => vertex.key === key) ??
    response.frontier.find((vertex) => vertex.key === key)
  );
}

/**
 * 図と表で選んでいる要素の key。押した要素がそのノードを指していればその key、指していなければ
 * そのノードの最初の要素の key を返す。
 *
 * **押した要素の key を優先する。** 内容のバージョンごとの要素は同じノードの識別子を持つため、
 * ノードの識別子から探すと、押していないバージョンの要素を選んだことになる。
 */
export function selectedVertexKey(
  response: InfluencePathResponse | undefined,
  clickedKey: string | undefined,
  selectedNodeId: string | undefined,
): string | undefined {
  if (response === undefined) return undefined;
  if (
    clickedKey !== undefined &&
    vertexOf(response, clickedKey)?.node.id === selectedNodeId
  ) {
    return clickedKey;
  }
  return response.vertices.find((vertex) => vertex.node.id === selectedNodeId)
    ?.key;
}

/** key の要素のノード。表示名を添え、検索の応答に無いノードも表示名で出せるようにする。 */
export function vertexNodeRef(
  response: InfluencePathResponse,
  key: string,
): NodeRef | undefined {
  const vertex = vertexOf(response, key);
  return vertex === undefined
    ? undefined
    : { id: vertex.node.id, label: nodeText(vertex.node) };
}

/**
 * 応答の vertices と影響のエッジだけを図の入力へ写す。点の識別子は vertex の key である。
 * 先へ影響が進まないノード (frontier) は vertices に入らず、図に描かない。
 */
function drawingOf(response: InfluencePathResponse): SubgraphDrawing {
  const ends = new Set([response.origin?.key, response.destination?.key]);
  return {
    points: response.vertices.map((vertex) => ({
      id: vertex.key,
      kind: vertex.node.kind,
      selection: ends.has(vertex.key) ? "matched" : "edge_endpoint",
      label: readNodeLabel(vertex.node.label),
    })),
    links: response.edges.map((edge) => ({
      id: edge.id,
      kind: edge.graphEdgeKind,
      state: edge.bases.includes("observed")
        ? "observed"
        : edge.bases.includes("uncertain_chain")
          ? "uncertain_chain"
          : "candidate",
      sourceNodeId: edge.sourceKey,
      targetNodeId: edge.targetKey,
      evidenceCount: edge.evidence.length,
    })),
  };
}

/** 根拠のレコード 1 件。収集元と位置を Record に表示する値にする。 */
function RecordButton({
  recordRef,
  onSelectRecord,
}: {
  recordRef: RecordLocator;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  return (
    <button
      type="button"
      className="value-link"
      onClick={() => onSelectRecord(recordRef)}
    >
      <RawText text={recordRef.sourceFileName} />{" "}
      {describeRecordPosition(recordRef)}
    </button>
  );
}

function EvidenceButtons({
  evidence,
  onSelectRecord,
}: {
  evidence: readonly GraphEvidence[];
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  return (
    <ul>
      {evidence.map((item) => (
        <li key={recordLocatorKey(item.recordRef)}>
          <RecordButton
            recordRef={item.recordRef}
            onSelectRecord={onSelectRecord}
          />
        </li>
      ))}
    </ul>
  );
}

/** ノードの表示名の文字列。表示名を持たないノードは識別子を使う。 */
function nodeText(node: GraphNode): string {
  return node.label.rawText ?? node.label.normalized ?? node.id;
}

/** 経路の時刻を決めたレコードの表の 1 行。起点か終点の、時刻を決めたレコードを持つ。 */
type EndpointRow = { end: string; endpoint: InfluenceEndpoint };

/**
 * 経路の時刻を決めたレコードの表。起点のノードの最初のレコードと、終点のノードの最後の
 * レコードを、時刻と Record に表示する値で並べる。
 *
 * 起点と終点の時刻は、ノードの根拠全体の最初と最後のレコードから取る。収集元が違うことだけを
 * 出す。同じ端末の別の収集元もあるため、端末をまたぐかどうかは収集元から決めない。
 */
function EndpointRecordTable({
  response,
  onSelectRecord,
}: {
  response: InfluencePathResponse;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const rows: EndpointRow[] = [
    ...(response.origin === undefined
      ? []
      : [{ end: "起点の最初", endpoint: response.origin }]),
    ...(response.destination === undefined
      ? []
      : [{ end: "終点の最後", endpoint: response.destination }]),
  ];
  if (rows.length === 0) return null;
  // 表に出した影響のエッジの根拠のレコードの収集元。応答がエッジを持たないときは空である。
  const edgeSources = new Set(
    response.edges.flatMap((edge) =>
      edge.evidence.map((item) => item.recordRef.sourceId),
    ),
  );
  const otherSource = (row: EndpointRow) =>
    edgeSources.size > 0 &&
    !edgeSources.has(row.endpoint.record.recordRef.sourceId);
  return (
    <DataTable
      label="経路の時刻を決めたレコード"
      rows={rows}
      rowKey={(row) => row.end}
      columns={[
        {
          key: "end",
          header: "端",
          rowHeader: true,
          className: "short-cell",
          cell: (row) => row.end,
        },
        {
          key: "time",
          header: "時刻",
          mono: true,
          className: "short-cell",
          cell: ({ endpoint }) =>
            endpoint.record.eventTime === undefined ? (
              <MissingValue description="時刻なし" />
            ) : (
              <TimestampText timestamp={endpoint.record.eventTime} />
            ),
        },
        {
          key: "record",
          header: "レコード",
          className: "label-cell",
          cell: ({ endpoint }) => (
            <RecordButton
              recordRef={endpoint.record.recordRef}
              onSelectRecord={onSelectRecord}
            />
          ),
        },
        {
          key: "source",
          header: "収集元",
          className: "short-cell",
          cell: (row) =>
            otherSource(row) ? (
              <StatusLabel
                status="idle"
                label="エッジの根拠と異なる"
                details={
                  response.omittedEdgeCount > 0
                    ? `未比較: 省いたエッジ ${formatCount(response.omittedEdgeCount)} 本の根拠`
                    : undefined
                }
              />
            ) : null,
        },
      ]}
    />
  );
}

/** 経路が無いときの状態。図の枠と Path のビューに短いラベルで出す。 */
type EmptyRouteState = "unsearched" | "undetermined" | "none";

function emptyRouteStateOf(response: InfluencePathResponse): EmptyRouteState {
  if (response.stops.some((stop) => unsearchedStops.has(stop))) {
    return "unsearched";
  }
  // **広げた計算が経路の無いことを確かめたときは、経路は無い。** 広げた計算は時刻の条件を緩めた
  // 探索であり、完走して経路が無ければ、広げない計算が上限に達しても経路は無い。経路の有無が
  // 分からないのは、広げた計算が上限に達し、経路が無い理由を返さなかったときである。
  const routeRuledOut = response.stops.some(
    (stop) =>
      stop === "no_influence_route" || stop === "time_order_unsatisfied",
  );
  return response.stops.includes("computation_limit") && !routeRuledOut
    ? "undetermined"
    : "none";
}

const emptyRouteLabels: Record<EmptyRouteState, string> = {
  unsearched: "対象外",
  undetermined: "未確定",
  none: "経路なし",
};

/** 経路が無いときの状態の補足。状態のラベルの tooltip に出す。 */
const emptyRouteDetails: Record<EmptyRouteState, string | undefined> = {
  unsearched: "起点か終点が経路の端にならない",
  undetermined: "探索の上限で停止",
  none: undefined,
};

/**
 * Graph のビューに描く影響の経路の図。影響のエッジと vertices だけを描く。経路が無いときと、
 * 取得の途中と失敗は、短いラベルだけを出す。理由は Path のビューが出す。
 */
export function InfluencePathFigure({
  state,
  settings,
  selectedKey,
  onSelectVertex,
}: {
  state: FetchState<InfluencePathResponse>;
  settings: CosmosLayoutSettings;
  selectedKey: string | undefined;
  onSelectVertex: (key: string) => void;
}) {
  const response = state.status === "loaded" ? state.value : undefined;
  const drawing = useMemo(
    () => (response === undefined ? undefined : drawingOf(response)),
    [response],
  );
  if (response === undefined || drawing === undefined) {
    return (
      <div className="figure">
        <p role="status" className="figure-placeholder">
          {state.status === "loading" ? (
            <StatusLabel status="running" label="読み込み中" />
          ) : (
            <StatusLabel status="failed" label="取得失敗" />
          )}
        </p>
      </div>
    );
  }
  if (response.edges.length === 0) {
    return (
      <div className="figure">
        <p role="status" className="figure-placeholder">
          {emptyRouteLabels[emptyRouteStateOf(response)]}
        </p>
      </div>
    );
  }
  return (
    <div className="figure">
      <SubgraphCanvasBoundary>
        <SubgraphCanvas
          drawing={drawing}
          settings={settings}
          selectedNodeId={selectedKey}
          onSelectNode={onSelectVertex}
        />
      </SubgraphCanvasBoundary>
    </div>
  );
}

function InfluencePathResult({
  response,
  selectedKey,
  onSelectVertex,
  onSelectRecord,
}: {
  response: InfluencePathResponse;
  selectedKey: string | undefined;
  onSelectVertex: (key: string) => void;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  // 表と一覧の button は、図の点と、図に描かない先へ進まないノードの両方を選ぶ。
  const vertexLabel = (key: string) => {
    const vertex = vertexOf(response, key);
    return vertex === undefined ? (
      key
    ) : (
      <>
        <button
          type="button"
          aria-pressed={key === selectedKey}
          onClick={() => onSelectVertex(key)}
        >
          <NodeLabelValue label={readNodeLabel(vertex.node.label)} />
        </button>
        {/* 導き方の印は button の外に置く。button の名前は表示名の値だけにする。 */}
        {vertex.node.label.valueState === "derived" ? (
          <DerivedLabelNote derivation={vertex.node.label.derivation} />
        ) : null}
        <KeyValueList
          stacked
          className="text-xs text-muted"
          pairs={[
            {
              name: "バージョンの始まり",
              value:
                vertex.versionStart === undefined ? undefined : (
                  <RawText text={vertex.versionStart} />
                ),
            },
            {
              name: "影響のエッジ",
              value: formatCount(vertex.influenceEdgeCount),
            },
            {
              name: "入口のプロセス",
              value:
                vertex.enteredFrom === undefined ? undefined : (
                  <Hint text="ほかの候補のプロセスへは進まない">
                    <NodeLabelView
                      label={readNodeLabel(vertex.enteredFrom.label)}
                    />
                  </Hint>
                ),
            },
          ]}
        />
      </>
    );
  };
  // **区間を広げない計算が上限に達したとき、応答は広げた計算の集合を返す。** どのエッジが
  // 精度の幅を広げたときにだけ条件を満たすかは求めきれておらず、時刻の根拠の欄は確定しない。
  const limited = response.stops.includes("computation_limit");
  // 起点か終点がタイムスタンプを持たないとき、backend は根拠のレコードを数える前に止まる。
  const untimedCounted = !response.stops.some(
    (stop) =>
      stop === "origin_without_timestamp" ||
      stop === "destination_without_timestamp",
  );
  const endKinds: Partial<Record<InfluencePathStop, string>> = {
    origin_not_influence_end: response.originNodeKind,
    destination_not_influence_end: response.destinationNodeKind,
  };
  const basisList = (bases: readonly InfluenceBasis[]) =>
    bases.map((basis) => influenceBasisLabels[basis]).join("、");
  const stopTable = (
    <DataTable
      label="止まった理由"
      rows={response.stops}
      rowKey={(stop) => stop}
      columns={[
        {
          key: "reason",
          header: "理由",
          rowHeader: true,
          cell: (stop) => stopSpecs[stop].label,
        },
        { key: "end", header: "端", cell: (stop) => stopSpecs[stop].end },
        {
          key: "detail",
          header: "補足",
          cell: (stop) => (
            <KeyValueList
              stacked
              pairs={[
                { name: "範囲", value: stopSpecs[stop].detail },
                {
                  name: "次の操作",
                  value:
                    endKinds[stop] === "ip" ? ipEndGuides[stop] : undefined,
                },
                {
                  name: "経路のエッジの除外した根拠",
                  value:
                    stop === "route_through_excluded_basis" &&
                    response.routeExcludedBases.length > 0
                      ? basisList(response.routeExcludedBases)
                      : undefined,
                },
              ]}
            />
          ),
        },
      ]}
    />
  );
  const emptyState = emptyRouteStateOf(response);
  const shownEdgeCount = response.edges.length;
  return (
    <>
      {response.edges.length === 0 ? (
        <div role="status" className="influence-path-empty">
          <StatusLabel
            status="idle"
            label={emptyRouteLabels[emptyState]}
            details={emptyRouteDetails[emptyState]}
          />
        </div>
      ) : null}
      {response.stops.length === 0 ? null : (
        <div role="status">
          <h3>止まった理由</h3>
          {stopTable}
        </div>
      )}
      {response.edges.length > 0 && limited ? (
        <p role="status">
          <StatusLabel
            status="idle"
            label="精度の幅で広げた区間の集合"
            details="広げない計算: 上限に到達"
          />
        </p>
      ) : null}
      <h3>計算の条件</h3>
      <EndpointRecordTable
        response={response}
        onSelectRecord={onSelectRecord}
      />
      <KeyValueList
        stacked
        pairs={[
          {
            name: "除外した根拠",
            value:
              response.excludedBases.length === 0
                ? "なし"
                : basisList(response.excludedBases),
          },
          {
            name: "経路のエッジ",
            value:
              response.omittedEdgeCount === 0
                ? undefined
                : formatCount(shownEdgeCount + response.omittedEdgeCount),
          },
          {
            name: "表示したエッジ",
            value:
              response.omittedEdgeCount === 0 ? undefined : (
                <Hint text="表示の順: 最も早く届く経路のエッジ · 短い経路のエッジ">
                  {formatCount(shownEdgeCount)}
                </Hint>
              ),
          },
          {
            name: "残りのエッジの表示",
            value:
              response.omittedEdgeCount === 0
                ? undefined
                : "除外する根拠の追加",
          },
          {
            name: "時刻の無い根拠のレコード",
            value: untimedCounted
              ? formatCount(response.untimedRecordCount)
              : undefined,
          },
        ]}
      />
      {response.edges.length === 0 ? null : <h3>影響のエッジ</h3>}
      {response.edges.length === 0 ? null : (
        <table aria-label="影響のエッジ">
          <thead>
            <tr>
              <th>始点</th>
              <th>終点</th>
              <th>エッジの種類</th>
              <th>根拠</th>
              <th>時刻の根拠</th>
              <th>根拠のレコード</th>
            </tr>
          </thead>
          <tbody>
            {response.edges.map((edge) => (
              <tr key={edge.id}>
                <td className="label-cell">{vertexLabel(edge.sourceKey)}</td>
                <td className="label-cell">{vertexLabel(edge.targetKey)}</td>
                <td className="label-cell">
                  {edgeKindLabels[edge.graphEdgeKind]}
                </td>
                <td className="label-cell">
                  <ul>
                    {edge.bases.map((basis) => (
                      <li key={basis}>
                        <Hint text={influenceBasisAxes[basis]}>
                          {influenceBasisLabels[basis]}
                        </Hint>
                      </li>
                    ))}
                    {edge.candidateOrder?.map((key) => (
                      <li key={key}>
                        {edgePairConditionDescriptions[key] === undefined ? (
                          edgePairConditionLabels[key]
                        ) : (
                          <Hint text={edgePairConditionDescriptions[key]}>
                            {edgePairConditionLabels[key]}
                          </Hint>
                        )}
                      </li>
                    ))}
                  </ul>
                  <KeyValueList
                    stacked
                    pairs={[
                      {
                        name: "同じ終点の候補",
                        value:
                          edge.candidateCount === undefined
                            ? undefined
                            : formatCount(edge.candidateCount),
                      },
                      ...(edge.candidateTally === undefined
                        ? []
                        : candidateTallyPairs(edge.candidateTally)),
                    ]}
                  />
                </td>
                <td className="label-cell">
                  <ul>
                    {edge.timeBases.map((basis) => (
                      <li key={basis}>{timeBasisLabels[basis]}</li>
                    ))}
                    {limited ? <li>精度の幅の条件: 未確定</li> : null}
                  </ul>
                </td>
                <td className="short-cell">
                  <EvidenceButtons
                    evidence={edge.evidence}
                    onSelectRecord={onSelectRecord}
                  />
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {response.frontierCount === 0 ? null : (
        <div>
          <h3>
            <Hint text="起点から届いたノード">先へ影響が進まないノード</Hint>
          </h3>
          <KeyValueList
            pairs={[
              { name: "件数", value: formatCount(response.frontierCount) },
              {
                name: "表示",
                value:
                  response.frontier.length < response.frontierCount
                    ? formatCount(response.frontier.length)
                    : undefined,
              },
            ]}
          />
          <DataTable
            label="先へ影響が進まないノード"
            rows={response.frontier}
            rowKey={(item) => item.key}
            columns={[
              {
                key: "node",
                header: "ノード",
                cell: (item) => vertexLabel(item.key),
              },
              {
                key: "reason",
                header: "理由",
                cell: (item) => frontierReasonLabels[item.reason],
              },
            ]}
          />
        </div>
      )}
    </>
  );
}

/** 起点と終点の値の組。Graph の図の上と Path のビューの先頭に出す。 */
export function InfluenceEndsPairs({ ends }: { ends: InfluenceEnds }) {
  return (
    <>
      <li>
        <span className="pair-name">起点:</span>{" "}
        <Highlighted text={ends.from.label} />
      </li>
      <li>
        <span className="pair-name">終点:</span>{" "}
        <Highlighted text={ends.to.label} />
      </li>
    </>
  );
}

/**
 * 影響の経路の Path のビュー。各影響のエッジの根拠、止まった理由、先へ影響が進まないノードを
 * 出す。除外する根拠の種類を選んで求め直せる。図は Graph のビューが描く。
 *
 * 経路の取得と、結果に使った除外する根拠の種類は上位が持つ。ビューを隠して描き直しても、取得と
 * 除外する根拠の種類を保つ。
 */
export function InfluencePathView({
  ends,
  state,
  excluded,
  onApplyExcluded,
  selectedKey,
  onSelectVertex,
  onSelectRecord,
  mergeSameAccount = false,
}: {
  ends: InfluenceEnds | undefined;
  state: FetchState<InfluencePathResponse> | undefined;
  excluded: readonly InfluenceBasis[];
  onApplyExcluded: (excluded: readonly InfluenceBasis[]) => void;
  selectedKey: string | undefined;
  onSelectVertex: (key: string) => void;
  onSelectRecord: (recordRef: RecordLocator) => void;
  /** 同じアカウントのまとめを Graph に適用しているか。経路はまとめる前のグラフで求める。 */
  mergeSameAccount?: boolean;
}) {
  // 選び途中の種類はビューが持つ。描き直したときは結果に使った種類から始める。
  const [draft, setDraft] = useState<ReadonlySet<InfluenceBasis>>(
    () => new Set(excluded),
  );
  const draftApplied =
    excluded.length === draft.size &&
    excluded.every((basis) => draft.has(basis));
  if (ends === undefined || state === undefined) {
    return (
      <section aria-label="影響の経路" className="view-pane">
        <p className="note">未選択</p>
      </section>
    );
  }
  return (
    <section aria-label="影響の経路" className="view-pane influence-path">
      <ul className="value-pairs">
        <InfluenceEndsPairs ends={ends} />
        <li>
          <span className="pair-name">推定条件:</span> 使用
        </li>
        <li>
          <span className="pair-name">検索の条件:</span> 未使用
        </li>
        {mergeSameAccount ? (
          <li>
            <span className="pair-name">同じアカウントのまとめ:</span> 未使用
          </li>
        ) : null}
      </ul>
      {/* 根拠の一覧は既定で閉じる。見出しに今の結果で除外した数を出す。 */}
      <details>
        <summary>
          除外する根拠{" "}
          <span className="tab-count" title="除外する根拠の数">
            {formatCount(excluded.length)}
          </span>
        </summary>
        <fieldset className="m-0 flex min-w-0 flex-col items-start gap-1 border-0 p-0">
          <legend className="sr-only">除外する根拠</legend>
          <DataTable
            label="除外する根拠"
            rows={influenceBases}
            rowKey={(basis) => basis}
            columns={[
              {
                key: "exclude",
                header: "除外",
                cell: (basis) => (
                  <input
                    type="checkbox"
                    aria-label={influenceBasisLabels[basis]}
                    checked={draft.has(basis)}
                    onChange={(event) => {
                      const next = new Set(draft);
                      if (event.currentTarget.checked) next.add(basis);
                      else next.delete(basis);
                      setDraft(next);
                    }}
                  />
                ),
              },
              {
                key: "axis",
                header: "分類",
                cell: (basis) => influenceBasisAxes[basis],
              },
              {
                key: "basis",
                header: "根拠",
                cell: (basis) => influenceBasisLabels[basis],
              },
            ]}
          />
          <div className="mt-1 flex items-center gap-2">
            <IconButton
              variant="secondary"
              label="選択した根拠を除外して再計算"
              onPress={() =>
                onApplyExcluded(
                  influenceBases.filter((basis) => draft.has(basis)),
                )
              }
            >
              <RefreshCw size={14} aria-hidden="true" />
            </IconButton>
            {draftApplied ? null : (
              <p role="status">
                <StatusLabel status="pending" label="未適用" />
              </p>
            )}
          </div>
        </fieldset>
      </details>
      <FetchStateView state={state} loadingDescription="影響の経路の読み込み中">
        {(response) => (
          <InfluencePathResult
            response={response}
            selectedKey={selectedKey}
            onSelectVertex={onSelectVertex}
            onSelectRecord={onSelectRecord}
          />
        )}
      </FetchStateView>
    </section>
  );
}

import { Filter, FilterX } from "lucide-react";
import {
  type CSSProperties,
  memo,
  type ReactNode,
  useCallback,
  useMemo,
  useState,
} from "react";
import type { MatchAssumption } from "@/shared/contracts/candidates";
import type { CaseEvidenceCount } from "@/shared/contracts/cases";
import type {
  RecordField,
  RecordLocator,
  TimestampInterpretation,
} from "@/shared/contracts/common";
import type {
  EdgeKind,
  GraphEvidence,
  GraphNode,
} from "@/shared/contracts/graph";
import {
  type EdgeAssignmentBasis,
  type EdgeDetailResponse,
  type EdgeEvidenceGroup,
  type EdgeEvidenceSelector,
  type EdgeMatchStage,
  edgeMatchTimeComparison,
  type MatchStageTally,
} from "@/shared/contracts/graphDetail";
import { describeEvidenceByCase } from "@/shared/lib/caseCounts";
import { formatCount } from "@/shared/lib/format";
import { listKey, recordLocatorKey } from "@/shared/lib/listKey";
import {
  assumptionKeyLabels,
  comparisonUnitLabels,
  evidenceClassLabels,
  matchStageEmptyReasonLabels,
  stageKeyLabels,
} from "@/shared/lib/matchLabels";
import {
  readRecordFieldNormalized,
  readRecordFieldRawText,
} from "@/shared/lib/recordField";
import { describeRecordLocation } from "@/shared/lib/recordPosition";
import {
  EvidenceRecordRow,
  EvidenceTableHead,
  evidenceColumnCount,
} from "@/shared/ui/EvidenceRecordRow";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Fold } from "@/shared/ui/Fold";
import { Highlighted } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MatchConditionTable } from "@/shared/ui/MatchConditionTable";
import { MissingValue } from "@/shared/ui/MissingValue";
import { ObservationKindView } from "@/shared/ui/ObservationKindView";
import { RawText } from "@/shared/ui/RawText";
import { TimestampText } from "@/shared/ui/TimestampText";
import { TimeWindowView } from "@/shared/ui/TimeWindowView";
import { useVirtualRows } from "@/shared/ui/useVirtualRows";
import { useValueMenu } from "@/shared/ui/ValueMenu";
import {
  SpacerBody,
  stickyHeaderCellStyle,
  virtualTableStyle,
} from "@/shared/ui/virtualTable";
import { EdgeRecordPairs } from "./EdgeRecordPairs";
import { EdgeTerminalAssignments } from "./EdgeTerminalAssignments";
import {
  edgeKindDescriptions,
  edgeKindLabels,
  relationStateDescriptions,
  relationStateLabels,
} from "./labels";
import { NodeLabelValue } from "./NodeLabelView";
import { isSquidCacheHit } from "./proxyStatus";
import { readNodeLabel } from "./subgraph";
import type { EdgeDetailState } from "./useEdgeDetail";

/** レコードを開く。`origin` はエッジの推定の元のレコードで、候補を開くときだけ渡す。 */
type SelectRecordWithOrigin = (
  recordRef: RecordLocator,
  origin?: RecordLocator,
) => void;

const loadingDescription = "エッジの詳細の読み込み中";
const noAccountDescription = "アカウントなし";
const noAuthenticationDescription = "認証の方式なし";
const wholeGroupLabel = "すべてのグループ";

/**
 * 描く前に見積もる行の高さ (px)。根拠のレコードは 1 件を折り返さない 1 行で描き、段階は
 * 見出しの文字列が折り返す。描いた行は測った高さへ直す。
 */
const estimatedEvidenceRowHeight = 30;
const estimatedStageRowHeight = 64;
/** 見えている範囲の前後に、先に描いておく高さ (px)。 */
const overscanPx = 400;

// 既知の制限: 全行の高さの和がブラウザーの要素の高さの上限 (Chromium で約 3,355 万 px) を
// 超えると、末尾の行までスクロールできない,
// 利用者が配置した実資料で測った。幅 1,400 px の画面で
// 根拠は 1 行あたり約 74 px、段階は 1 段階あたり約 84 px だった,
// 1 本の関係の根拠か段階が 30 万を超えたときに見直す
const scrollerStyle: CSSProperties = {
  maxHeight: "50vh",
  overflow: "auto",
  overflowAnchor: "none",
  scrollbarGutter: "stable",
};

/**
 * 行を入れ替える表の見た目。詳細の列の表は `layout.css` が block にして横へスクロール
 * させる。行を入れ替える表は列の幅を固定するため、table に戻す。
 */
const detailVirtualTableStyle: CSSProperties = {
  ...virtualTableStyle,
  display: "table",
};

/**
 * 根拠のレコードの表の見た目。値を 1 行で出し、区画の幅に収まらないときはスクロールする要素の
 * 中で表を横にスクロールさせる。
 */
const evidenceTableStyle: CSSProperties = {
  display: "table",
  borderCollapse: "collapse",
  width: "max-content",
  minWidth: "100%",
};

/**
 * 段階を並べる欄の見た目。段階の中の表は `layout.css` が横へスクロールさせる。文字列をどこでも
 * 折り返す指定を欄に置くと、中の表の列が 1 字の幅まで縮む。
 */
const stageCellStyle: CSSProperties = {
  verticalAlign: "top",
  padding: "0.25rem 0",
  borderTop: "1px solid GrayText",
};

/**
 * 区分 1 つを一覧の中で指す key を作る。
 *
 * **要求で指す値を持たない区分も区別する。** backend の区分の鍵
 * (`backend/pipeline/graph_edge_detail.go` の `evidenceGroupKey`) は観測の種別の値と、
 * 接続先 port とログオンの種別の値と値の持ち方を含む。指す値を持たない区分では、観測の種別と
 * 接続先 port とログオンの種別の欄の文字列が区分を分ける。
 */
function groupKey(group: EdgeEvidenceGroup): string {
  const selector = group.selector;
  return listKey([
    selector?.eventCategory,
    selector?.eventAction,
    selector?.destinationPort,
    selector?.destinationPortAbsent === true ? "port_absent" : undefined,
    selector?.logonType,
    selector?.logonTypeAbsent === true ? "logon_type_absent" : undefined,
    selector?.httpStatus,
    selector?.httpStatusAbsent === true ? "http_status_absent" : undefined,
    group.selectorAbsence,
    group.destinationPortAbsence,
    group.logonTypeAbsence,
    group.httpStatusAbsence,
    ...group.observationKind.raw.flatMap(fieldKeyParts),
    ...(group.destinationPort === undefined
      ? []
      : fieldKeyParts(group.destinationPort)),
    ...(group.logonType === undefined ? [] : fieldKeyParts(group.logonType)),
    ...(group.httpStatus === undefined ? [] : fieldKeyParts(group.httpStatus)),
  ]);
}

/** 欄 1 つを key の材料にする。欄の名前と、値の状態と、原資料の文字列と正規化値である。 */
function fieldKeyParts(field: RecordField): (string | undefined)[] {
  const value = field.kind === "text" ? field.text : field.timestamp;
  return [
    field.name,
    field.semantic,
    value.valueState,
    value.rawText,
    value.normalized,
  ];
}

/** 2 つの区分を指す値が同じ区分を指すかを返す。 */
function sameSelector(
  left: EdgeEvidenceSelector | undefined,
  right: EdgeEvidenceSelector | undefined,
): boolean {
  if (left === undefined || right === undefined) {
    return left === undefined && right === undefined;
  }
  return (
    left.eventCategory === right.eventCategory &&
    left.eventAction === right.eventAction &&
    left.destinationPort === right.destinationPort &&
    left.destinationPortAbsent === right.destinationPortAbsent &&
    left.logonType === right.logonType &&
    left.logonTypeAbsent === right.logonTypeAbsent &&
    left.httpStatus === right.httpStatus &&
    left.httpStatusAbsent === right.httpStatusAbsent
  );
}

function EndpointRow({ label, node }: { label: string; node: GraphNode }) {
  return (
    <tr>
      <th scope="row">{label}</th>
      <td>
        <NodeLabelValue label={readNodeLabel(node.label)} />
        {/* 識別の値は区切りの無い長い文字列 (ハッシュ、SID) を持つ。区画の幅で折り返す。 */}
        <ul className="break-all">
          {node.identity.map((value) => (
            <li key={listKey([value.semantic, value.value])}>
              <Highlighted
                text={value.value}
                field={{ name: value.semantic ?? "", semantic: value.semantic }}
              />
            </li>
          ))}
        </ul>
      </td>
    </tr>
  );
}

/**
 * 区分のレコードに現れたアカウントを、識別した値と現れた件数とともに出す。
 *
 * **識別した値を添える。** 同じ名前のアカウントが、SID で識別したノードと、領域と名前で
 * 識別したノードの 2 つで並ぶ。表示名だけでは 2 つを読み分けられない。
 */
function GroupAccounts({ group }: { group: EdgeEvidenceGroup }) {
  if (group.accounts.length === 0) {
    return <MissingValue description={noAccountDescription} />;
  }
  return (
    <ul>
      {group.accounts.map((account) => (
        <li key={account.node.id}>
          <NodeLabelValue label={readNodeLabel(account.node.label)} />{" "}
          <span className="text-xs text-muted">
            <RawText
              text={account.node.identity
                .map((value) => value.value)
                .join(" \\ ")}
            />
          </span>{" "}
          <Hint text="レコード数" className="tab-count">
            {formatCount(account.evidenceCount)}
          </Hint>
        </li>
      ))}
    </ul>
  );
}

/**
 * 区分を分けた値 (接続先 port、ログオンの種別のコード) を出す。値を持たない区分は持たない
 * 理由を出し、0 と書かない。
 *
 * **比べる値である正規化値を先に読み、無ければ原資料の文字列を読む。** 値は原資料の文字列から導いたことが
 * ある。Proxy の要求の接続先 port の原資料の文字列は要求先の URL であり、ログオンの種別のコードは
 * 原資料の文字列を持たないことがある。
 *
 * 値と理由は排他であり、読み込みがどちらか一方を持つ区分だけを通す
 * (`decodeEdgeEvidenceGroup`)。理由を持たない区分に該当する経路が無い。
 */
function GroupFieldValue({
  field,
  absence,
}: {
  field: RecordField | undefined;
  absence: string | undefined;
}) {
  if (field === undefined) {
    return <MissingValue description={absence ?? ""} />;
  }
  const normalized = readRecordFieldNormalized(field);
  const value =
    "text" in normalized ? normalized : readRecordFieldRawText(field);
  return "text" in value ? (
    <Highlighted text={value.text} field={field} />
  ) : (
    <MissingValue description={value.absence} />
  );
}

/** 区分のレコードが記録した認証の方式を、記録した件数とともに出す。 */
function GroupAuthentications({ group }: { group: EdgeEvidenceGroup }) {
  if (group.authentications.length === 0) {
    return <MissingValue description={noAuthenticationDescription} />;
  }
  return (
    <ul>
      {group.authentications.map((authentication) => (
        <li key={listKey(fieldKeyParts(authentication.value))}>
          <GroupFieldValue field={authentication.value} absence={undefined} />{" "}
          <Hint text="レコード数" className="tab-count">
            {formatCount(authentication.evidenceCount)}
          </Hint>
        </li>
      ))}
    </ul>
  );
}

/**
 * 区分を選ぶ操作を出す。指せない区分は、指せない理由を出す。
 *
 * 指す値と理由は排他であり、読み込みがどちらか一方を持つ区分だけを通す。
 */
function GroupSelectCell({
  group,
  selected,
  onSelectGroup,
}: {
  group: EdgeEvidenceGroup;
  selected: EdgeEvidenceSelector | undefined;
  onSelectGroup: (selector: EdgeEvidenceSelector | undefined) => void;
}) {
  const selector = group.selector;
  if (selector === undefined) {
    return <MissingValue description={group.selectorAbsence ?? ""} />;
  }
  return (
    <IconButton
      variant="secondary"
      label="このグループの根拠を表示"
      isDisabled={sameSelector(selected, selector)}
      disabledReason={{ title: "表示中", text: "別のグループの選択" }}
      onPress={() => onSelectGroup(selector)}
    >
      <Filter size={14} aria-hidden="true" />
    </IconButton>
  );
}

type EvidenceGroupListProps = {
  groups: EdgeEvidenceGroup[];
  selected: EdgeEvidenceSelector | undefined;
  onSelectGroup: (selector: EdgeEvidenceSelector | undefined) => void;
};

/**
 * 根拠の区分を、区分ごとに項目名と値の組の一覧で出す。詳細の区画の幅に収めるため、
 * 項目を列に並べない。
 *
 * **観測と分析者の判断を分けて出す。** 接続先 port は観測であり、その port が
 * どのプロトコルであるかは所見の欄が持つ。
 */
function EvidenceGroupList({
  groups,
  selected,
  onSelectGroup,
}: EvidenceGroupListProps) {
  // HTTP の状態の列は、HTTP の要求の区分を持つ関係だけに出す。
  const hasHttp = groups.some(
    (group) =>
      group.httpStatus !== undefined || group.httpStatusAbsence !== undefined,
  );
  const groupedBy = `イベントの種類・接続先 port・ログオンタイプ${hasHttp ? "・HTTP の状態" : ""}`;
  return (
    <Fold
      summary="根拠のグループ"
      summaryNote={`グループ: ${formatCount(groups.length)}`}
      rowCount={groups.length}
    >
      <KeyValueList
        className="text-xs text-muted"
        pairs={[
          { name: "グループの軸", value: groupedBy },
          { name: "件数の母数", value: "エッジの根拠の全数" },
        ]}
      />
      <div className="flex items-center gap-2 py-1">
        <span>{wholeGroupLabel}</span>
        <IconButton
          variant="secondary"
          label="すべての根拠を表示"
          isDisabled={selected === undefined}
          disabledReason={{ title: "表示中", text: "グループの選択後" }}
          onPress={() => onSelectGroup(undefined)}
        >
          <FilterX size={14} aria-hidden="true" />
        </IconButton>
      </div>
      <ul aria-label="根拠のグループ">
        {groups.map((group) => (
          <li
            key={groupKey(group)}
            className="grid justify-items-start gap-1.5 border-t border-line py-2"
          >
            <dl className="grid w-full grid-cols-[8.5rem_minmax(0,1fr)] gap-x-3 gap-y-1 [&>dd]:min-w-0 [&>dd]:break-words [&>dt]:text-muted">
              <dt>イベントの種類</dt>
              <dd>
                <ObservationKindView observationKind={group.observationKind} />
              </dd>
              <dt>接続先 port</dt>
              <dd>
                <GroupFieldValue
                  field={group.destinationPort}
                  absence={group.destinationPortAbsence}
                />
              </dd>
              <dt>ログオンタイプ</dt>
              <dd>
                <GroupFieldValue
                  field={group.logonType}
                  absence={group.logonTypeAbsence}
                />
              </dd>
              {hasHttp ? (
                <>
                  <dt>HTTP の状態</dt>
                  <dd>
                    {group.httpStatus === undefined &&
                    group.httpStatusAbsence === undefined ? (
                      <MissingValue description="HTTP の要求以外" />
                    ) : (
                      <GroupFieldValue
                        field={group.httpStatus}
                        absence={group.httpStatusAbsence}
                      />
                    )}
                  </dd>
                </>
              ) : null}
              <dt>アカウント</dt>
              <dd>
                <GroupAccounts group={group} />
              </dd>
              <dt>認証の方式</dt>
              <dd>
                <GroupAuthentications group={group} />
              </dd>
              <dt>根拠のレコード数</dt>
              <dd>{formatCount(group.evidenceCount)}</dd>
            </dl>
            <GroupSelectCell
              group={group}
              selected={selected}
              onSelectGroup={onSelectGroup}
            />
          </li>
        ))}
      </ul>
    </Fold>
  );
}

type EvidenceTableProps = {
  evidence: GraphEvidence[];
  /** 根拠のレコードの総数。絞り込みを通った全件である。 */
  evidenceCount: number;
  /** `evidenceCount` を案件ごとに分けた件数。案件を区別しない取り込みでは出ない。 */
  evidenceByCase: CaseEvidenceCount[] | undefined;
};

/** 根拠のレコード 1 件の行。スクロールで行を入れ替えるたびに、残る行を描き直さない。 */
const EvidenceRow = memo(EvidenceRecordRow);

/**
 * 根拠のレコードを 1 行ずつ出す。絞り込みを通った全件が並ぶ。
 *
 * **見えている行とその前後だけを描く。** 1 本の関係が根拠を数千件持つことがあり、全行を
 * 描くと、ブラウザーが行の配置を計算する間、画面が止まる。
 */
function EvidenceTable({
  evidence,
  evidenceCount,
  evidenceByCase,
}: EvidenceTableProps) {
  // 件数の多い表は閉じて出し、開いたときだけ行を描画する。
  return (
    <Fold
      summary="根拠のレコード"
      summaryNote={
        `件数: ${formatCount(evidenceCount)}` +
        (evidenceByCase === undefined
          ? ""
          : ` · 案件ごと: ${describeEvidenceByCase(evidenceByCase)}`)
      }
      rowCount={evidence.length}
    >
      <VirtualEvidenceTable evidence={evidence} />
    </Fold>
  );
}

function VirtualEvidenceTable({ evidence }: { evidence: GraphEvidence[] }) {
  const termMenu = useValueMenu();
  const virtual = useVirtualRows(
    evidence.length,
    evidence,
    estimatedEvidenceRowHeight,
    overscanPx,
  );
  return (
    <section
      ref={virtual.scrollerRef}
      aria-label="根拠のレコードの表"
      // biome-ignore lint/a11y/noNoninteractiveTabindex: スクロールする領域をキーボードで動かせるようにする。
      tabIndex={0}
      onScroll={virtual.onScroll}
      style={scrollerStyle}
    >
      {/* 見出しは Fold の summary が出すため、表の名前は読み上げだけに渡す。 */}
      <table
        className="evidence-table"
        style={evidenceTableStyle}
        aria-rowcount={evidence.length + 1}
        aria-label="根拠のレコード"
      >
        <EvidenceTableHead cellStyle={stickyHeaderCellStyle} />
        <SpacerBody
          height={virtual.spaceBefore}
          columnCount={evidenceColumnCount()}
        />
        <tbody ref={virtual.bodyRef}>
          {evidence.slice(virtual.start, virtual.end).map((item, offset) => (
            <EvidenceRow
              key={recordLocatorKey(item.recordRef)}
              evidence={item}
              rowIndex={virtual.start + offset}
              termActions={termMenu.actions}
            />
          ))}
        </tbody>
        <SpacerBody
          height={virtual.spaceAfter}
          columnCount={evidenceColumnCount()}
        />
      </table>
      {termMenu.menu}
    </section>
  );
}

/**
 * エッジを何から作ったか。
 *
 * - `records`: レコードから直接作ったエッジか、エッジの推定が挙げたエッジ。端末の割り当てを持たない。
 * - `assignment`: 端末の割り当てだけから作ったエッジ。
 * - `both`: レコードが直に観測し、端末の割り当ても同じエッジを与えているエッジ。
 */
type EdgeOrigin = "records" | "assignment" | "both";

function edgeOriginOf(detail: EdgeDetailResponse): EdgeOrigin {
  if (detail.terminalAssignments === undefined) {
    return "records";
  }
  return detail.observedInRecords === true ? "both" : "assignment";
}

const assignmentStateLabel = "端末の割り当て";

/** エッジの作り方の表示。割り当てから作ったエッジは、そのことを作り方の値に出す。 */
function relationStateText(detail: EdgeDetailResponse): string {
  switch (edgeOriginOf(detail)) {
    case "assignment":
      return assignmentStateLabel;
    case "both":
      return `${relationStateLabels[detail.edge.state]} · ${assignmentStateLabel}`;
    default:
      return relationStateLabels[detail.edge.state];
  }
}

/**
 * エッジの推定を経ていないエッジの、作った元。エッジの作り方と、何から作ったかに合わせる。
 *
 * **推定したエッジを「直接作った」と書かない。** 観測の層が同じ収集元のレコードの値から
 * 作った候補は、エッジの推定の段階を経ずに推定したエッジである。
 */
function unmatchedText(detail: EdgeDetailResponse): string {
  if (detail.edge.state !== "observed" && edgeOriginOf(detail) === "records") {
    return (detail.recordPairCount ?? 0) > 0
      ? "レコードの組の条件"
      : "レコードの値";
  }
  switch (edgeOriginOf(detail)) {
    case "assignment":
      return assignmentStateLabel;
    case "both":
      return `レコード · ${assignmentStateLabel}`;
    default:
      return "レコード";
  }
}

/** エッジの種類のラベル。補足を持つ種類は補足を tooltip に出す。 */
function EdgeKindText({ kind }: { kind: EdgeKind }) {
  const description = edgeKindDescriptions[kind];
  return description === undefined ? (
    edgeKindLabels[kind]
  ) : (
    <Hint text={description}>{edgeKindLabels[kind]}</Hint>
  );
}

function EdgeSummary({ detail }: { detail: EdgeDetailResponse }) {
  return (
    <table>
      <caption>選択中のエッジ</caption>
      <tbody>
        <tr>
          <th scope="row">エッジの種類</th>
          <td>
            <EdgeKindText kind={detail.edge.kind} />
          </td>
        </tr>
        <tr>
          <th scope="row">作り方</th>
          <td>
            <Hint text={relationStateDescriptions[detail.edge.state]}>
              {relationStateText(detail)}
            </Hint>
          </td>
        </tr>
        <EndpointRow label="始点" node={detail.sourceNode} />
        <EndpointRow label="終点" node={detail.targetNode} />
      </tbody>
    </table>
  );
}

/**
 * 段階ごとに候補が何件から何件へ減ったかを出す。
 *
 * **絞り込みの過程を段階ごとに読む。** 最後の段階の件数だけでは、時刻を使わない段階が何件に
 * 絞り、時刻の一致を足した段階が何件に減らしたかを読めない。その差が、時刻の条件が
 * どれだけ作用したかである。
 */
function StageTallyTable({ tallies }: { tallies: MatchStageTally[] }) {
  return (
    <table>
      <caption>段階ごとの候補</caption>
      <thead>
        <tr>
          <th scope="col">段階</th>
          <th scope="col">候補</th>
          <th scope="col">候補のプロセス</th>
          <th scope="col">候補が無い理由</th>
        </tr>
      </thead>
      <tbody>
        {tallies.map((tally) => (
          <tr key={tally.stageKey}>
            <th scope="row">{stageKeyLabels[tally.stageKey]}</th>
            <td>{formatCount(tally.memberCount)}</td>
            <td>
              {tally.distinctProcessCount === undefined ? (
                <MissingValue description="プロセスのフィールドなし" />
              ) : (
                formatCount(tally.distinctProcessCount)
              )}
            </td>
            <td>
              {tally.emptyReason === undefined ? (
                <MissingValue description="候補あり" />
              ) : (
                matchStageEmptyReasonLabels[tally.emptyReason]
              )}
            </td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}

/** レコードの位置を、収集元の file 名と file の中の位置で出す。 */
function RecordPositionText({ locator }: { locator: RecordLocator }) {
  return <RawText text={describeRecordLocation(locator)} />;
}

/**
 * 起点 1 件に対して候補を挙げた段階を、条件と前提と確度の材料と、段階が挙げた候補のうち
 * この関係を作った候補と一緒に出す。
 *
 * **確度の材料を同じ画面に置く。** 同じ起点が候補を 1 件だけ挙げたのか 5 件挙げたのかで、
 * 関連付けが表す確からしさが変わる。条件だけを見て候補を確定と読ませない。
 *
 * **見出しに、この関係を作った候補の件数と、段階が挙げた候補の総数を並べる。** 段階が挙げた
 * 候補は、候補の端点ごとに別の関係へ分かれる。この関係の件数だけを出すと、他の関係へ
 * 分かれた候補が無いと読める。
 *
 * **開いた段階だけ中身を描く。** 1 本のエッジが段階を数千個、関連付けを数十万件持つことがある。
 *
 * **開いているかを上位の一覧が持つ。** 一覧は見えている段階だけを描くため、スクロールで
 * 外れた段階は描き直すときに作り直される。
 */
const MatchStageView = memo(function MatchStageView({
  detail,
  stage,
  stageIndex,
  matchIndexes,
  open,
  onToggle,
  onSelectRecord,
}: {
  detail: EdgeDetailResponse;
  stage: EdgeMatchStage;
  stageIndex: number;
  matchIndexes: number[];
  open: boolean;
  onToggle: (stageIndex: number, open: boolean) => void;
  onSelectRecord: SelectRecordWithOrigin;
}) {
  const stageLabel = stageKeyLabels[stage.stageKey];
  // 末尾の段階がこの段階である。decoder が要素数 1 以上を確かめている。
  const stageMemberCount =
    stage.stageTallies[stage.stageTallies.length - 1].memberCount;
  return (
    <details
      className="match-stage"
      open={open}
      onToggle={(event) => onToggle(stageIndex, event.currentTarget.open)}
    >
      <summary>
        {stageLabel}
        <KeyValueList
          inline
          className="ml-2"
          pairs={[
            {
              name: "元のレコード",
              value: (
                <RecordPositionText
                  locator={detail.matchRecords[stage.origin].ref}
                />
              ),
            },
            { name: "エッジの候補", value: formatCount(matchIndexes.length) },
            { name: "段階の候補", value: formatCount(stageMemberCount) },
          ]}
        />
      </summary>
      {open && (
        <>
          <ProxyStatusLine
            field={detail.matchRecords[stage.origin].proxyStatus}
          />
          <StageTallyTable tallies={stage.stageTallies} />
          <MatchConditionTable
            conditions={stage.conditions}
            tableLabel="この段階"
            assumptions={stage.assumptions}
          />
          <TimeWindowView
            timeWindow={stage.timeWindow}
            stageLabel={stageLabel}
          />
          <KeyValueList
            pairs={[
              {
                name: "端末時刻への依存",
                value: <RawText text={stage.clockDependencyNote} />,
              },
            ]}
          />
          <AssumptionList assumptions={stage.assumptions} />
          <IndistinguishableGroupList
            groups={stage.indistinguishableGroups.map((group) =>
              group.map((at) => detail.matchRecords[at].ref),
            )}
          />
          <StageCandidateTable
            detail={detail}
            matchIndexes={matchIndexes}
            onSelectRecord={onSelectRecord}
          />
        </>
      )}
    </details>
  );
});

// 既知の制限: 開いた段階の候補を上限なしで全行描く, 利用者が配置した実資料で測った。候補が最も多い段階を開くのに 1.2 s だった, 1 つの段階の候補が数千件になる入力で段階を開くのが遅れたときに見直す
/**
 * 段階が挙げた候補のうちこの関係を作った候補を、時刻の比較と確定しない理由と一緒に出す。
 * 候補のレコードは、段階の起点を添えて開く。開いたレコードの欄が起点からの経路を出す。
 */
function StageCandidateTable({
  detail,
  matchIndexes,
  onSelectRecord,
}: {
  detail: EdgeDetailResponse;
  matchIndexes: number[];
  onSelectRecord: SelectRecordWithOrigin;
}) {
  return (
    <table>
      <caption>エッジを作った候補</caption>
      <thead>
        <tr>
          <th scope="col">候補のレコード</th>
          <th scope="col">時刻の比較の単位</th>
          <th scope="col">比較の前提</th>
          <th scope="col">未確定の理由</th>
        </tr>
      </thead>
      <tbody>
        {matchIndexes.map((index) => {
          const match = detail.matches[index];
          const comparison = edgeMatchTimeComparison(detail, match);
          const stage = detail.matchStages[match.stage];
          const candidate = detail.matchRecords[match.candidate].ref;
          return (
            <tr key={index}>
              <td>
                <button
                  type="button"
                  className="value-link"
                  title="元のレコードからの経路と一緒に Record に表示"
                  onClick={() =>
                    onSelectRecord(
                      candidate,
                      detail.matchRecords[stage.origin].ref,
                    )
                  }
                >
                  <RecordPositionText locator={candidate} />
                </button>
              </td>
              <td>{comparisonUnitLabels[comparison.comparisonUnit]}</td>
              <td>
                <AssumptionList assumptions={comparison.assumptions} />
              </td>
              <td>
                <UnresolvedReasonList
                  reasons={stage.unresolvedReasonSets[match.unresolvedReasons]}
                />
              </td>
            </tr>
          );
        })}
      </tbody>
    </table>
  );
}

/** 段階が依拠する前提を、根拠の種類と一緒に出す。 */
function AssumptionList({ assumptions }: { assumptions: MatchAssumption[] }) {
  if (assumptions.length === 0) {
    return <KeyValueList pairs={[{ name: "前提", value: "なし" }]} />;
  }
  // 前提ごとに「前提の名前: 根拠の種類 · 内容」の組を 1 行に置く。候補の表のセルにも置くため、
  // 表を入れ子にしない。
  return (
    <KeyValueList
      stacked
      pairs={assumptions.map((assumption) => ({
        key: assumption.assumptionKey,
        name: assumptionKeyLabels[assumption.assumptionKey],
        value: (
          <>
            {evidenceClassLabels[assumption.evidenceClass]} ·{" "}
            <RawText text={assumption.statement} />
          </>
        ),
      }))}
    />
  );
}

/**
 * 段階が用いた条件だけでは互いに区別できない候補の組を出す。
 *
 * **組が 1 つでもあれば、候補は 1 件に定まらない。** 組が無いことも組数 0 で出す。
 */
function IndistinguishableGroupList({ groups }: { groups: RecordLocator[][] }) {
  if (groups.length === 0) {
    return (
      <KeyValueList pairs={[{ name: "区別できない候補の組", value: "0" }]} />
    );
  }
  return (
    <Fold
      summary="区別できない候補の組"
      summaryNote={`組数: ${formatCount(groups.length)}`}
      rowCount={groups.reduce((total, group) => total + group.length, 0)}
    >
      <ul>
        {groups.map((group) => (
          <li key={group.map((ref) => ref.recordRawTextRef).join("|")}>
            <ul>
              {group.map((ref) => (
                <li key={ref.recordRawTextRef}>
                  <RawText text={describeRecordLocation(ref)} />
                </li>
              ))}
            </ul>
          </li>
        ))}
      </ul>
    </Fold>
  );
}

/** 未確定の理由を出す。 */
function UnresolvedReasonList({ reasons }: { reasons: string[] }) {
  return (
    <ul aria-label="未確定の理由">
      {reasons.map((reason) => (
        <li key={reason}>
          <RawText text={reason} />
        </li>
      ))}
    </ul>
  );
}

/**
 * 元のレコードの Proxy の行が記録した要求処理の結果を出す。結果が cache からの応答を示す行
 * (isSquidCacheHit) は、そのことを添える。
 */
function ProxyStatusLine({ field }: { field: RecordField | undefined }) {
  const status = field?.kind === "text" ? field.text.rawText : undefined;
  if (status === undefined) {
    return null;
  }
  return (
    <KeyValueList
      pairs={[
        { name: "Proxy の処理結果", value: <RawText text={status} /> },
        {
          name: "Proxy の cache",
          value: isSquidCacheHit(status) ? "cache からの応答" : undefined,
        },
      ]}
    />
  );
}

/**
 * 接続元の IP アドレスから端末を決めて作った推定のエッジの、成立の根拠を出す。
 *
 * **IP アドレスから決めたことを明示する。** 接続元の IP アドレスから端末を決めたエッジは、
 * 接続元 IP から端末への割り当てを通って成立している。
 */
function AssignmentBasisList({
  bases,
  sourceFileNames,
}: {
  bases: EdgeAssignmentBasis[] | undefined;
  sourceFileNames: ReadonlyMap<string, string> | undefined;
}) {
  if (bases === undefined || bases.length === 0) {
    return null;
  }
  // 収集元と適用期間だけが違う根拠は、同じアドレスから同じ端末を導いている。1 つにまとめ、
  // 収集元ごとの適用期間を表で出す。導いた端末の導き方は用いた割当を指して割当ごとに
  // 違うため、まとめる鍵に入れず、収集元ごとの表の行に根拠ごとの導き方を出す。
  const groups = new Map<string, EdgeAssignmentBasis[]>();
  for (const basis of bases) {
    const key = JSON.stringify({
      ...basis,
      sourceId: undefined,
      conditions: basis.conditions.map((condition) => ({
        ...condition,
        assignmentValidRange: undefined,
        rightValue: condition.rightValue?.map((field) =>
          field.kind === "text"
            ? { ...field, text: { ...field.text, derivation: undefined } }
            : field,
        ),
      })),
    });
    groups.set(key, [...(groups.get(key) ?? []), basis]);
  }
  return (
    <>
      <h3>IP アドレスから端末を決めた根拠</h3>
      {[...groups.entries()].map(([key, members]) => {
        const basis = members[0];
        return (
          <div key={key}>
            <KeyValueList
              pairs={[
                {
                  name: "使った IP アドレス",
                  value: <RawText text={basis.clientIp} />,
                },
              ]}
            />
            <table>
              <caption>割り当ての適用期間</caption>
              <thead>
                <tr>
                  <th scope="col">収集元</th>
                  <th scope="col">適用期間</th>
                  <th scope="col">端末の決め方</th>
                </tr>
              </thead>
              <tbody>
                {members.map((member, index) => {
                  const range = member.conditions.find(
                    (condition) => condition.assignmentValidRange !== undefined,
                  )?.assignmentValidRange;
                  return (
                    // biome-ignore lint/suspicious/noArrayIndexKey: 根拠は識別子を持たず、並びは応答が決める。
                    <tr key={index}>
                      <td>
                        <RawText
                          text={
                            sourceFileNames?.get(member.sourceId) ??
                            member.sourceId
                          }
                        />
                      </td>
                      <td>
                        {range === undefined ? null : (
                          <>
                            <TimestampText timestamp={range.from} />
                            {" – "}
                            <TimestampText timestamp={range.to} />
                          </>
                        )}
                      </td>
                      <td>
                        {member.conditions
                          .flatMap((condition) => condition.rightValue ?? [])
                          .map((field) =>
                            field.kind === "text"
                              ? field.text.derivation
                              : undefined,
                          )
                          .filter((text) => text !== undefined)
                          .map((text) => (
                            <div key={text}>
                              <RawText text={text} />
                            </div>
                          ))}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
            <MatchConditionTable
              conditions={basis.conditions}
              tableLabel="端末を決める判定"
            />
            <KeyValueList
              pairs={[
                {
                  name: "端末時刻への依存",
                  value: <RawText text={basis.clockDependencyNote} />,
                },
              ]}
            />
            <AssumptionList assumptions={basis.assumptions} />
          </div>
        );
      })}
    </>
  );
}

/** 関連付けの位置を、関連付けを出した段階ごとに、関連付けの並び順のまま集める。 */
function matchIndexesByStage(detail: EdgeDetailResponse): number[][] {
  const byStage: number[][] = detail.matchStages.map(() => []);
  detail.matches.forEach((match, index) => {
    byStage[match.stage].push(index);
  });
  return byStage;
}

/**
 * エッジを作った関連付けを、起点ごとの段階に分けて全件出す。
 *
 * **観測から直接作ったエッジは関連付けを持たない。** 持たないことを文で出し、
 * 関連付けが 0 件であることと、関連付けを読んでいないことを分ける。
 *
 * **同じ応答の間は描き直さない。** 段階の見出しが数千個あり、画面の他の状態が変わるたびに
 * 描き直すと操作が遅れる。
 */
const MatchSection = memo(function MatchSection({
  detail,
  sourceFileNames,
  sourceInterpretations,
  onSelectRecord,
}: {
  detail: EdgeDetailResponse;
  sourceFileNames: ReadonlyMap<string, string> | undefined;
  sourceInterpretations:
    | ReadonlyMap<string, TimestampInterpretation>
    | undefined;
  onSelectRecord: SelectRecordWithOrigin;
}) {
  return (
    <>
      {/* 推定を経ていないエッジに「作った推定」と見出しを付けると、推定があると読める。 */}
      <h3>{detail.matchCount === 0 ? "エッジの推定" : "エッジを作った推定"}</h3>
      {detail.matchCount === 0 ? (
        <KeyValueList
          pairs={[
            { name: "エッジの推定", value: "なし" },
            { name: "作った元", value: unmatchedText(detail) },
          ]}
        />
      ) : (
        <Fold
          summary="推定の結果"
          summaryNote={`件数: ${formatCount(detail.matchCount)} · 段階: ${formatCount(detail.matchStages.length)}`}
          rowCount={detail.matchStages.length}
          openLimit={2}
        >
          <MatchStageList detail={detail} onSelectRecord={onSelectRecord} />
        </Fold>
      )}
      <EdgeRecordPairs
        pairs={detail.recordPairs}
        pairCount={detail.recordPairCount}
        onSelectRecord={onSelectRecord}
      />
      <AssignmentBasisList
        bases={detail.assignmentBases}
        sourceFileNames={sourceFileNames}
      />
      <EdgeTerminalAssignments
        edgeKind={detail.edge.kind}
        assignments={detail.terminalAssignments}
        sourceFileNames={sourceFileNames}
        sourceInterpretations={sourceInterpretations}
      />
    </>
  );
});

/**
 * 関連付けの段階を、見えている段階とその前後だけ描く。段階を開くと高さが変わり、描いた後に測り直す。
 * 表は段階を縦に並べる枠であり、支援技術には段階の見出しだけを出す。
 */
function MatchStageList({
  detail,
  onSelectRecord,
}: {
  detail: EdgeDetailResponse;
  onSelectRecord: SelectRecordWithOrigin;
}) {
  const byStage = useMemo(() => matchIndexesByStage(detail), [detail]);
  const [openStages, setOpenStages] = useState<ReadonlySet<number>>(
    () => new Set(),
  );
  const toggleStage = useCallback((stageIndex: number, open: boolean) => {
    setOpenStages((current) => {
      if (current.has(stageIndex) === open) {
        return current;
      }
      const next = new Set(current);
      if (open) {
        next.add(stageIndex);
      } else {
        next.delete(stageIndex);
      }
      return next;
    });
  }, []);
  const virtual = useVirtualRows(
    detail.matchStages.length,
    detail,
    estimatedStageRowHeight,
    overscanPx,
  );
  return (
    <section
      ref={virtual.scrollerRef}
      aria-label="推定の段階の一覧"
      // biome-ignore lint/a11y/noNoninteractiveTabindex: スクロールする領域をキーボードで動かせるようにする。
      tabIndex={0}
      onScroll={virtual.onScroll}
      style={scrollerStyle}
    >
      <table role="presentation" style={detailVirtualTableStyle}>
        <SpacerBody height={virtual.spaceBefore} columnCount={1} />
        <tbody ref={virtual.bodyRef}>
          {detail.matchStages
            .slice(virtual.start, virtual.end)
            .map((stage, offset) => {
              const index = virtual.start + offset;
              return (
                // 段階の並びは応答が決め、同じ応答の中で変わらない。
                <tr key={index} data-row-index={index}>
                  <td style={stageCellStyle}>
                    <MatchStageView
                      detail={detail}
                      stage={stage}
                      stageIndex={index}
                      matchIndexes={byStage[index]}
                      open={openStages.has(index)}
                      onToggle={toggleStage}
                      onSelectRecord={onSelectRecord}
                    />
                  </td>
                </tr>
              );
            })}
        </tbody>
        <SpacerBody height={virtual.spaceAfter} columnCount={1} />
      </table>
    </section>
  );
}

type EdgeDetailProps = {
  state: EdgeDetailState;
  /** 詳細を出している区分。すべての根拠を出しているときは出ない。 */
  selectedSelector: EdgeEvidenceSelector | undefined;
  onSelectGroup: (selector: EdgeEvidenceSelector | undefined) => void;
  /**
   * レコードを開く。関連付けの候補を開くときは、段階の起点を添える。
   * **描画をまたいで同じ関数を渡す。** 関連付けの一覧は memo である。
   */
  onSelectRecord: SelectRecordWithOrigin;
  /** 関係に付いた所見の表示と記録。関係を選んでいるときだけ出す。 */
  assertions: (detail: EdgeDetailResponse) => ReactNode;
  /** 根拠の URL の断片をつなぐ欄。HTTP の要求の関係だけに出す。渡さないときは出さない。 */
  urlFragments?: (detail: EdgeDetailResponse) => ReactNode;
  /** 詳細の先頭に並べる操作。出ない場合は出さない。 */
  headerActions?: (detail: EdgeDetailResponse) => ReactNode;
  /**
   * 収集元の sourceId から file 名を探す表。割当の「適用する収集元」を file 名で出す。
   * 渡さないときは sourceId をそのまま出す。
   */
  sourceFileNames?: ReadonlyMap<string, string>;
  /**
   * 収集元の sourceId から、その収集元の今の時刻の解釈を探す表。関係を作った割当の地方時の
   * 適用期間を、この解釈で読んで出す。渡さないときは期間をそのまま出す。
   */
  sourceInterpretations?: ReadonlyMap<string, TimestampInterpretation>;
};

/**
 * 関係 1 本の詳細を出す。
 *
 * 用いた手段を読む材料は、観測の種別と接続先 port ごとの区分である。
 * **port がどのプロトコルであるかを自動で確定しない。** その判断は所見の欄が持つ。
 */
export function EdgeDetail({
  state,
  selectedSelector,
  onSelectGroup,
  onSelectRecord,
  assertions,
  urlFragments,
  headerActions,
  sourceFileNames,
  sourceInterpretations,
}: EdgeDetailProps) {
  // 関係を選んでいない間は欄を出さない。右の列はノードの詳細に使う。
  if (state.status === "unselected") {
    return null;
  }
  return (
    <section>
      <h2>Edge Detail</h2>
      <FetchStateView state={state} loadingDescription={loadingDescription}>
        {(detail) => (
          <>
            {headerActions?.(detail)}
            <EdgeSummary detail={detail} />
            <EvidenceGroupList
              groups={detail.evidenceGroups}
              selected={selectedSelector}
              onSelectGroup={onSelectGroup}
            />
            <MatchSection
              detail={detail}
              sourceFileNames={sourceFileNames}
              sourceInterpretations={sourceInterpretations}
              onSelectRecord={onSelectRecord}
            />
            <EvidenceTable
              evidence={detail.edge.evidence}
              evidenceCount={detail.edge.evidenceCount}
              evidenceByCase={detail.edge.evidenceByCase}
            />
            {detail.edge.kind === "http_request"
              ? urlFragments?.(detail)
              : null}
            {assertions(detail)}
          </>
        )}
      </FetchStateView>
    </section>
  );
}

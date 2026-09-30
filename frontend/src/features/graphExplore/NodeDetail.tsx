import {
  ArrowLeft,
  ArrowRight,
  FileText,
  Filter,
  Flag,
  Focus,
  GitFork,
  History,
  Route,
  Share2,
} from "lucide-react";
import type { ReactNode } from "react";
import type { NodeRef } from "@/shared/api/graph";
import type { RecordLocator, Timestamp } from "@/shared/contracts/common";
import type {
  EdgeDirection,
  GraphEvidence,
  NodeIdentityValue,
} from "@/shared/contracts/graph";
import type {
  NodeAttribute,
  NodeDetailResponse,
  NodeEdgeCount,
} from "@/shared/contracts/graphDetail";
import {
  describeEvidenceByCase,
  formatEvidenceCountWithCases,
} from "@/shared/lib/caseCounts";
import { formatCount } from "@/shared/lib/format";
import { recordLocatorKey } from "@/shared/lib/listKey";
import { toVisibleRawText } from "@/shared/lib/rawText";
import {
  type FieldValue,
  readRecordFieldNormalized,
  readRecordFieldRawText,
} from "@/shared/lib/recordField";
import {
  describeRecordLocation,
  describeRecordPosition,
} from "@/shared/lib/recordPosition";
import { fieldConditions } from "@/shared/lib/valueCondition";
import { AddTermButton } from "@/shared/ui/AddTermButton";
import { Button } from "@/shared/ui/Button";
import { CopyButton } from "@/shared/ui/CopyButton";
import { LocalTimeNote } from "@/shared/ui/DisplayOffset";
import {
  EvidenceRecordRow,
  EvidenceTableHead,
} from "@/shared/ui/EvidenceRecordRow";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { Fold } from "@/shared/ui/Fold";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { Highlighted, PeriodMark } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TimestampOffsetNote } from "@/shared/ui/TimestampOffsetNote";
import { useCopyText } from "@/shared/ui/useCopyText";
import {
  useValueMenu,
  type ValueMenuActions,
  ValueMenuButton,
} from "@/shared/ui/ValueMenu";
import {
  edgeDirectionLabels,
  edgeKindLabels,
  logonSessionRejectionReasonDetails,
  logonSessionRejectionReasonLabels,
  nodeCreationRecordLabels,
  nodeEvidenceTitleLabels,
  nodeKeyFormLabelOf,
  nodeKindLabelOf,
  nodeReferencedOnlyMark,
  referencedCreationFact,
  referencedOnlyFacts,
  relationDerivationBasisLabels,
  relationDerivationNextActions,
  relationDerivationOutcomeLabels,
} from "./labels";
import { NodeLabelView } from "./NodeLabelView";
import { readNodeLabel } from "./subgraph";
import type { NodeDetailState } from "./useNodeDetail";

const eventTimeAbsent = "時刻なし";
const registryValueNameSemantic = "registry_value.name";
const noVocabularyItemDescription = "共通フィールド名なし";

/**
 * 集合の要素を並びの中で見分ける値を作る。React の再描画だけに使い、要求にも表示にも
 * 出さない。
 */
function renderKey(parts: unknown[]): string {
  return JSON.stringify(parts);
}

function FieldValueCell({
  field,
  value,
}: {
  field: NodeAttribute["values"][number]["field"];
  value: FieldValue;
}) {
  return "text" in value ? (
    <PeriodMark
      timestamp={field.kind === "timestamp" ? field.timestamp : undefined}
    >
      <Highlighted text={value.text} field={field} />
    </PeriodMark>
  ) : (
    <MissingValue description={value.absence} />
  );
}

function NodeIdentityItem({
  value,
  time,
  onAddTerm,
  onCopy,
}: {
  value: NodeIdentityValue;
  /** 値が時刻の正規化値であるときの、その時刻。PID の区間の始まりの値だけが持つ。 */
  time?: Timestamp;
  onAddTerm: ((text: string) => void) | undefined;
  onCopy?: ((text: string, what: string) => void) | undefined;
}) {
  // 地方時の文字列の値は、読んだずれで求めた UTC と、ずれの出どころを添える (TimestampOffsetNote)。
  return (
    <li>
      {value.semantic === undefined ? null : (
        <>
          <RawText text={value.semantic} />
          {": "}
        </>
      )}
      <PeriodMark timestamp={time}>
        <Highlighted
          text={value.value}
          field={{ name: value.semantic ?? "", semantic: value.semantic }}
        />
      </PeriodMark>
      <span className="inline-flex items-center ml-1 gap-0.5">
        {onAddTerm === undefined ? null : (
          <AddTermButton text={value.value} onAdd={onAddTerm} />
        )}
        {onCopy === undefined ? null : (
          <CopyButton
            text={value.value}
            onCopy={(text) => onCopy(text, value.semantic ?? "同一性の値")}
          />
        )}
      </span>
      {time === undefined ? null : (
        <span className="mt-0.5 block text-xs text-muted">
          <TimestampOffsetNote timestamp={time} />
        </span>
      )}
    </li>
  );
}

/** 詳細の値から次の検索へ移る操作。出ない場合は操作の button を出さない。 */
export type NodeDetailActions = {
  /** 値を「含む文字列」の条件に足す。 */
  onAddTerm: (text: string) => void;
  /** 端末のノードで根拠のレコードを絞る。 */
  onNarrowToTerminal: (terminal: NodeRef) => void;
  /** プロセスのノードから親子の連鎖をたどる。 */
  onTraceLineage: (process: NodeRef) => void;
  /** ノードとその関係先だけをグラフに出す。 */
  onShowNeighbours: (node: NodeRef) => void;
  /** アカウントを名指した記録を時刻順に表示する。 */
  onShowAccountRecords?: (node: NodeRef) => void;
  /** 今のグラフにノードの関係先を足す。出ない場合は足す操作を出さない。 */
  onAddNeighbours?: (node: NodeRef) => void;
  /** ノードを影響の経路の起点に決める。出ない場合は経路の操作を出さない。 */
  onMarkPathStart?: (node: NodeRef) => void;
  /** 決めてある影響の経路の起点。出ない場合は起点を決めていない。 */
  pathStart?: NodeRef;
  /** 影響の経路の起点からノードまでの影響の経路を求める。 */
  onShowPath?: (to: NodeRef) => void;
};

/** ノードの表示名を、操作に渡す文字列へ直す。 */
function labelTextOf(detail: NodeDetailResponse): string {
  return (
    detail.node.label.rawText ?? detail.node.label.normalized ?? detail.node.id
  );
}

function NodeActionButtons({
  detail,
  actions,
}: {
  detail: NodeDetailResponse;
  actions: NodeDetailActions;
}) {
  const reference = {
    id: detail.node.id,
    label: labelTextOf(detail),
    kind: detail.node.kind,
  };
  const { onAddNeighbours, pathStart } = actions;
  // 端末は影響の経路の端にならない。端末のノードには経路の操作を出さない。IP のノードは端になる
  // ことがあるため残す。
  const pathEnd = detail.node.kind !== "terminal";
  const onMarkPathStart = pathEnd ? actions.onMarkPathStart : undefined;
  const onShowPath = pathEnd ? actions.onShowPath : undefined;
  const isPathStart = pathStart?.id === reference.id;
  return (
    <div className="node-actions">
      <Button
        size="sm"
        variant="secondary"
        onPress={() => actions.onShowNeighbours(reference)}
      >
        <Focus size={14} aria-hidden="true" />
        隣接ノードだけを表示
      </Button>
      {detail.node.kind === "account" &&
      actions.onShowAccountRecords !== undefined ? (
        <Button
          size="sm"
          variant="secondary"
          onPress={() => actions.onShowAccountRecords?.(reference)}
        >
          <History size={14} aria-hidden="true" />
          このアカウントを名指した記録
        </Button>
      ) : null}
      {onAddNeighbours === undefined ? null : (
        <Button
          size="sm"
          variant="secondary"
          onPress={() => onAddNeighbours(reference)}
        >
          <Share2 size={14} aria-hidden="true" />
          隣接ノードを追加
        </Button>
      )}
      {pathStart === undefined ||
      isPathStart ||
      onShowPath === undefined ? null : (
        <Button
          size="sm"
          variant="secondary"
          aria-label={`影響の経路を表示: ${toVisibleRawText(pathStart.label)} から`}
          tooltip={`影響の経路を表示: ${toVisibleRawText(pathStart.label)} から`}
          onPress={() => onShowPath(reference)}
        >
          <Route size={14} aria-hidden="true" />
          影響の経路を表示
        </Button>
      )}
      {onMarkPathStart === undefined ? null : (
        <Button
          size="sm"
          variant="secondary"
          isDisabled={isPathStart}
          disabledReason={{
            title: "影響の経路の起点に設定済み",
            text: "別のノードで設定",
          }}
          onPress={() => onMarkPathStart(reference)}
        >
          <Flag size={14} aria-hidden="true" />
          影響の経路の起点に設定
        </Button>
      )}
      {detail.node.kind === "process" ? (
        <Button
          size="sm"
          variant="secondary"
          onPress={() => actions.onTraceLineage(reference)}
        >
          <GitFork size={14} aria-hidden="true" />
          プロセスの親子関係を表示
        </Button>
      ) : null}
      {/* 参照だけの端末には置いたレコードが無く、絞ると必ず 0 件になる。 */}
      {detail.node.kind === "terminal" &&
      detail.node.observation !== "referenced" ? (
        <Button
          size="sm"
          variant="secondary"
          onPress={() => actions.onNarrowToTerminal(reference)}
        >
          <Filter size={14} aria-hidden="true" />
          この端末でフィルタ
        </Button>
      ) : null}
    </div>
  );
}

/**
 * 名前の分からない端末に置いたファイルの印。端末を記録しない収集元は収集元ごとに別の端末に置かれ、
 * 同じパスでも収集元ごとに別のファイルのノードになる。収集元に端末を割り当てると、同じ端末の
 * 他の収集元の同じパスのファイルと 1 つのノードになる。
 */
function UnknownTerminalMark() {
  return (
    <StatusLabel
      status="idle"
      label="端末が不明"
      details={
        <KeyValueList
          stacked
          pairs={[
            { name: "次の操作", value: "収集元に端末を割り当て" },
            { name: "割り当てのビュー", value: "Time & Host" },
          ]}
        />
      }
    />
  );
}

/** 選んだノードの種別と表示名を、詳細の先頭に出す。 */
function NodeHeader({
  detail,
  actions,
  onCopy,
}: {
  detail: NodeDetailResponse;
  actions: ReactNode;
  onCopy?: ((text: string, what: string) => void) | undefined;
}) {
  const nodeLabel = readNodeLabel(detail.node.label);
  const copyableText =
    "text" in nodeLabel.value ? nodeLabel.value.text : undefined;
  return (
    <div className="node-header">
      <span className="flex items-center gap-1.5">
        <span className={`kind-badge kind-${detail.node.kind}`}>
          {nodeKindLabelOf(detail.node)}
        </span>
        {/* 参照だけで分かっている対象のときだけ印を付ける。記録がある対象は印を付けても判断を変えない。 */}
        {detail.node.observation === "referenced" ? (
          // 印は操作を持たない。意味は印の横の help で出し、pointer と keyboard の両方で開ける。
          <span className="inline-flex items-center gap-0.5">
            <span className="rounded-sm border border-dashed border-muted px-1.5 text-xs text-muted">
              {nodeReferencedOnlyMark}
            </span>
            <HelpPopover label={nodeReferencedOnlyMark}>
              <KeyValueList
                stacked
                pairs={[
                  ...referencedOnlyFacts,
                  ...(detail.node.creationRecord === "present"
                    ? [referencedCreationFact]
                    : []),
                ].map(([name, value]) => ({ name, value }))}
              />
            </HelpPopover>
          </span>
        ) : null}
        {actions}
      </span>
      <p className="node-title">
        <NodeLabelView label={nodeLabel} />
        {onCopy === undefined || copyableText === undefined ? null : (
          <span className="inline-flex items-center ml-1">
            <CopyButton
              text={copyableText}
              onCopy={(text) => onCopy(text, "ノードの名前")}
            />
          </span>
        )}
      </p>
      {detail.node.kind === "file" && detail.node.onUnknownTerminal === true ? (
        <UnknownTerminalMark />
      ) : null}
    </div>
  );
}

function NodeIdentityTable({
  detail,
  actions,
  onCopy,
}: {
  detail: NodeDetailResponse;
  actions: NodeDetailActions | undefined;
  onCopy?: ((text: string, what: string) => void) | undefined;
}) {
  const { node } = detail;
  return (
    <table>
      <caption>同一性</caption>
      <tbody>
        <tr>
          <th scope="row">同一性の基準</th>
          <td>{nodeKeyFormLabelOf(node)}</td>
        </tr>
        <tr>
          <th scope="row">作成レコード</th>
          <td>{nodeCreationRecordLabels[node.creationRecord]}</td>
        </tr>
        <tr>
          <th scope="row">同一性の値</th>
          <td>
            <ul>
              {node.identity.map((value, index) => (
                <NodeIdentityItem
                  key={renderKey([value.semantic ?? null, value.value])}
                  value={value}
                  time={
                    // 区間の始まりの時刻は鍵の最後の値であり、その時刻の正規化値と一致する。
                    index === node.identity.length - 1 &&
                    value.value === node.intervalStart?.normalized
                      ? node.intervalStart
                      : undefined
                  }
                  onAddTerm={actions?.onAddTerm}
                  onCopy={onCopy}
                />
              ))}
            </ul>
          </td>
        </tr>
      </tbody>
    </table>
  );
}

/**
 * 属性 1 つを指す名前。
 * 語彙に写していない欄は原資料の key の文字列で名前を持つ
 * (`backend/core/graph_response.go` の `NodeAttribute`)。
 */
function attributeKeyOf(attribute: NodeAttribute): string {
  return attribute.semantic ?? attribute.name ?? "";
}

function NodeAttributeRows({
  attribute,
  onAddTerm,
  onCopy,
  menuActions,
}: {
  attribute: NodeAttribute;
  onAddTerm: ((text: string) => void) | undefined;
  onCopy?: ((text: string, what: string) => void) | undefined;
  /** 原文の値を押したときに開く値のメニュー。 */
  menuActions: ValueMenuActions | undefined;
}) {
  return (
    <>
      {attribute.values.map((value) => {
        const rawText = readRecordFieldRawText(value.field);
        const normalized = readRecordFieldNormalized(value.field);
        return (
          <tr key={renderKey([attributeKeyOf(attribute), value.field])}>
            <td>
              {attribute.semantic === undefined ? (
                <MissingValue description={noVocabularyItemDescription} />
              ) : (
                <>
                  <RawText text={attribute.semantic} />
                  {onCopy === undefined ? null : (
                    <span className="inline-flex items-center ml-1">
                      <CopyButton
                        text={attribute.semantic}
                        onCopy={(text) => onCopy(text, "共通フィールド名")}
                      />
                    </span>
                  )}
                </>
              )}
            </td>
            <td>
              <RawText text={value.field.name} />
              {onCopy === undefined ? null : (
                <span className="inline-flex items-center ml-1">
                  <CopyButton
                    text={value.field.name}
                    onCopy={(text) => onCopy(text, "原資料のフィールド名")}
                  />
                </span>
              )}
            </td>
            <td>
              {"text" in rawText ? (
                <ValueMenuButton
                  target={{
                    key: renderKey([attributeKeyOf(attribute), value.field]),
                    name: value.field.name,
                    text: rawText.text,
                    conditions: fieldConditions(value.field),
                    copies: [{ what: "原文の文字列", text: rawText.text }],
                  }}
                  actions={menuActions}
                >
                  <FieldValueCell field={value.field} value={rawText} />
                </ValueMenuButton>
              ) : (
                <FieldValueCell field={value.field} value={rawText} />
              )}
              {!("text" in rawText) ? null : (
                <span className="inline-flex items-center ml-1 gap-0.5">
                  {onAddTerm === undefined ? null : (
                    <AddTermButton text={rawText.text} onAdd={onAddTerm} />
                  )}
                  {onCopy === undefined ? null : (
                    <CopyButton
                      text={rawText.text}
                      onCopy={(text) => onCopy(text, "原文の文字列")}
                    />
                  )}
                </span>
              )}
            </td>
            <td>
              <FieldValueCell field={value.field} value={normalized} />
              {!("text" in normalized) ? null : (
                <span className="inline-flex items-center ml-1">
                  {onCopy === undefined ? null : (
                    <CopyButton
                      text={normalized.text}
                      onCopy={(text) => onCopy(text, "正規化値")}
                    />
                  )}
                </span>
              )}
            </td>
            <td>{formatCount(value.observationCount)}</td>
            <td>{formatCount(attribute.valueCount)}</td>
            <td>
              <RawText text={value.firstRecordRef.sourceFileName} />{" "}
              {describeRecordPosition(value.firstRecordRef)}
            </td>
          </tr>
        );
      })}
    </>
  );
}

/**
 * 複数の値の名前を 1 つのノードへ束ねていることを明示する。
 * 図の識別鍵は既存の形を保ち、詳細で全数と各値を確認できる形を採る。
 */
function RegistryValueNameSummary({ detail }: { detail: NodeDetailResponse }) {
  if (detail.node.kind !== "registry_value") {
    return null;
  }
  const nameAttribute = detail.attributes.find(
    (attribute) => attribute.semantic === registryValueNameSemantic,
  );
  if (nameAttribute === undefined || nameAttribute.valueCount < 2) {
    return null;
  }
  return (
    <div role="status">
      <KeyValueList
        pairs={[
          {
            name: "まとめた値の名前",
            value: formatCount(nameAttribute.valueCount),
          },
        ]}
      />
    </div>
  );
}

function NodeAttributeTable({
  detail,
  onAddTerm,
  onCopy,
}: {
  detail: NodeDetailResponse;
  onAddTerm: ((text: string) => void) | undefined;
  onCopy?: ((text: string, what: string) => void) | undefined;
}) {
  const valueMenu = useValueMenu();
  return (
    <>
      <RegistryValueNameSummary detail={detail} />
      <KeyValueList
        pairs={[{ name: "属性", value: formatCount(detail.attributeCount) }]}
      />
      <table>
        <caption>属性</caption>
        <thead>
          <tr>
            <th scope="col">共通フィールド名</th>
            <th scope="col">原資料のフィールド名</th>
            <th scope="col">原文</th>
            <th scope="col">正規化した値</th>
            <th scope="col">レコード数</th>
            <th scope="col">値の種類</th>
            <th scope="col">最初のレコード</th>
          </tr>
        </thead>
        <tbody>
          {detail.attributes.map((attribute) => (
            <NodeAttributeRows
              key={attributeKeyOf(attribute)}
              attribute={attribute}
              onAddTerm={onAddTerm}
              onCopy={onCopy}
              menuActions={valueMenu.actions}
            />
          ))}
        </tbody>
      </table>
      {valueMenu.menu}
    </>
  );
}

/** 根拠のレコードを 1 件 1 行で並べる、開閉できる表。 */
function EvidenceFold({
  title,
  note,
  evidence,
}: {
  title: string;
  /** 見出しの横に添える件数の組。 */
  note: string;
  evidence: readonly GraphEvidence[];
}) {
  const termMenu = useValueMenu();
  return (
    <Fold summary={title} summaryNote={note} rowCount={evidence.length}>
      <table className="evidence-table">
        <caption>{title}</caption>
        <EvidenceTableHead />
        <tbody>
          {evidence.map((item) => (
            <EvidenceRecordRow
              key={recordLocatorKey(item.recordRef)}
              evidence={item}
              termActions={termMenu.actions}
            />
          ))}
        </tbody>
      </table>
      {termMenu.menu}
    </Fold>
  );
}

function NodeEvidenceTable({ detail }: { detail: NodeDetailResponse }) {
  const note =
    `件数: ${formatCount(detail.evidenceCount)}` +
    (detail.evidenceByCase === undefined
      ? ""
      : ` · 案件ごと: ${describeEvidenceByCase(detail.evidenceByCase)}`);
  return (
    <EvidenceFold
      title={nodeEvidenceTitleLabels[detail.node.observation]}
      note={note}
      evidence={detail.evidence}
    />
  );
}

/**
 * ノードの作成レコードを出す。
 *
 * **軸が該当しない種類ではセクションそのものを出さない。** `item_absent` のノードに「なし」と
 * 出すと、その種類が作成レコードを持ち得るという読みになる。
 */
function NodeCreationRecordTable({ detail }: { detail: NodeDetailResponse }) {
  if (detail.node.creationRecord === "item_absent") {
    return null;
  }
  if (detail.creationRecords.length === 0) {
    return (
      <KeyValueList
        pairs={[
          {
            name: "作成レコード",
            value: (
              <Hint text="取り込んだ収集元の中に作成レコードなし">なし</Hint>
            ),
          },
        ]}
      />
    );
  }
  return (
    <EvidenceFold
      title="作成レコード"
      note={`件数: ${formatCount(detail.creationRecordCount)}`}
      evidence={detail.creationRecords}
    />
  );
}

/**
 * このレコードを元にした接続先の推定の結果を出す。
 *
 * **述べるのは接続先の推定の元になったかだけである。** ログオンと操作の結び付けは
 * LogonSessionRejectionTable が別のセクションで出す。
 *
 * **レコードの種類のノードだけが持つ。** 他の種類のノードは推定の元にならないため、
 * セクションそのものを出さない。
 *
 * **判定の根拠の種類を添える。** 原資料の事実か、レコードにフィールドが無いことか、
 * Oraculum が処理できなかったことかで、分析者が次に採る手が違う。
 */
function RelationDerivationView({ detail }: { detail: NodeDetailResponse }) {
  const derivation = detail.relationDerivation;
  if (derivation === undefined) {
    return null;
  }
  const nextAction = relationDerivationNextActions[derivation.outcome];
  return (
    <>
      <h4>接続先の推定</h4>
      <KeyValueList
        stacked
        pairs={[
          {
            name: "結果",
            value: relationDerivationOutcomeLabels[derivation.outcome],
          },
          {
            name: "判定の根拠",
            value: relationDerivationBasisLabels[derivation.basis],
          },
          { name: "次の操作", value: nextAction },
        ]}
      />
    </>
  );
}

/**
 * このレコードの操作の Logon ID が一致しながら、エッジにしなかったログオンのレコードと理由を出す。
 * 該当するログオンを持たないノードではセクションを出さない。
 */
function LogonSessionRejectionTable({
  detail,
  onSelectRecord,
}: {
  detail: NodeDetailResponse;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  if (detail.logonSessionRejections.length === 0) {
    return null;
  }
  const title = "エッジにしなかったログオン";
  return (
    <>
      <h4>ログオンと操作の結び付け</h4>
      <Fold
        summary={title}
        summaryNote={`件数: ${formatCount(detail.logonSessionRejections.length)}`}
        rowCount={detail.logonSessionRejections.length}
      >
        <table>
          <caption>
            <Hint text="操作と Logon ID が一致したログオン">{title}</Hint>
          </caption>
          <thead>
            <tr>
              <th scope="col">収集元</th>
              <th scope="col">位置</th>
              <th scope="col">時刻の原文</th>
              <th scope="col">理由</th>
              <th scope="col">操作</th>
            </tr>
          </thead>
          <tbody>
            {detail.logonSessionRejections.map(({ reason, logon }) => {
              const position = describeRecordPosition(logon.recordRef);
              return (
                <tr key={recordLocatorKey(logon.recordRef)}>
                  <td>
                    <RawText text={logon.recordRef.sourceFileName} />
                  </td>
                  <td>{position}</td>
                  <td>
                    {logon.eventTime?.rawText === undefined ? (
                      <MissingValue description={eventTimeAbsent} />
                    ) : (
                      <PeriodMark timestamp={logon.eventTime}>
                        <RawText text={logon.eventTime.rawText} />
                      </PeriodMark>
                    )}
                    <LocalTimeNote timestamp={logon.eventTime} />
                  </td>
                  <td>
                    <Hint text={logonSessionRejectionReasonDetails[reason]}>
                      {logonSessionRejectionReasonLabels[reason]}
                    </Hint>
                  </td>
                  <td>
                    <IconButton
                      label={`ログオンのレコードを Record に表示: ${toVisibleRawText(describeRecordLocation(logon.recordRef))}`}
                      onPress={() => onSelectRecord(logon.recordRef)}
                    >
                      <FileText size={14} aria-hidden="true" />
                    </IconButton>
                  </td>
                </tr>
              );
            })}
          </tbody>
        </table>
      </Fold>
    </>
  );
}

/** 選んだノードから見たエッジの向きの icon。始点は右向き、終点は左向きの矢印にする。 */
function EdgeDirectionIcon({ direction }: { direction: EdgeDirection }) {
  const label = edgeDirectionLabels[direction];
  const Icon = direction === "outgoing" ? ArrowRight : ArrowLeft;
  return (
    <Hint text={label}>
      <span role="img" aria-label={label}>
        <Icon size={14} aria-hidden="true" />
      </span>
    </Hint>
  );
}

function NodeEdgeCountRow({ count }: { count: NodeEdgeCount }) {
  return (
    <tr>
      <td>{edgeKindLabels[count.edgeKind]}</td>
      <td>
        <EdgeDirectionIcon direction={count.direction} />
      </td>
      <td>{formatCount(count.edgeCount)}</td>
      <td>
        {formatEvidenceCountWithCases(
          count.evidenceCount,
          count.evidenceByCase,
        )}
      </td>
    </tr>
  );
}

function NodeEdgeCountTable({ detail }: { detail: NodeDetailResponse }) {
  return (
    <table>
      <caption>エッジの件数</caption>
      <thead>
        <tr>
          <th scope="col">エッジの種類</th>
          <th scope="col">向き</th>
          <th scope="col">エッジ数</th>
          <th scope="col">根拠のレコード数</th>
        </tr>
      </thead>
      <tbody>
        {detail.edgeCounts.map((count) => (
          <NodeEdgeCountRow
            key={renderKey([count.edgeKind, count.direction])}
            count={count}
          />
        ))}
      </tbody>
    </table>
  );
}

type NodeDetailProps = {
  state: NodeDetailState;
  onSelectRecord: (recordRef: RecordLocator) => void;
  /** ノードに付いた所見の表示と記録。ノードを選んでいるときだけ出す。 */
  assertions: (detail: NodeDetailResponse) => ReactNode;
  /** 関係の相手側の欄を数える部品。出ない場合は出さない。 */
  relatedValueCounts?: (detail: NodeDetailResponse) => ReactNode;
  /** 見出しに並べる操作。出ない場合は出さない。 */
  headerActions?: ((detail: NodeDetailResponse) => ReactNode) | undefined;
  /** 詳細の値から次の検索へ移る操作。出ない場合は操作の button を出さない。 */
  actions?: NodeDetailActions;
};

/** 選んだノードの識別と属性と根拠と相互参照の件数を出す (操作 10)。 */
export function NodeDetail({
  state,
  onSelectRecord,
  assertions,
  relatedValueCounts,
  headerActions,
  actions,
}: NodeDetailProps) {
  const { copy, notice } = useCopyText();
  switch (state.status) {
    case "unselected":
      return <p className="empty-hint">ノード未選択</p>;
    case "loading":
      return (
        <p role="status">
          <StatusLabel status="running" label="ノードの詳細の読み込み中" />
        </p>
      );
    case "failed":
      return <FetchFailureNotice failure={state.failure} />;
    case "empty":
      return <p role="status">{state.description}</p>;
    case "loaded":
      return (
        <div className="node-detail">
          {notice}
          <NodeHeader
            detail={state.value}
            actions={headerActions?.(state.value)}
            onCopy={copy}
          />
          {actions === undefined ? null : (
            <NodeActionButtons detail={state.value} actions={actions} />
          )}
          <NodeIdentityTable
            detail={state.value}
            actions={actions}
            onCopy={copy}
          />
          <NodeCreationRecordTable detail={state.value} />
          <NodeEvidenceTable detail={state.value} />
          <RelationDerivationView detail={state.value} />
          <LogonSessionRejectionTable
            detail={state.value}
            onSelectRecord={onSelectRecord}
          />
          <details className="more-detail">
            <summary>属性とエッジの件数</summary>
            <NodeAttributeTable
              detail={state.value}
              onAddTerm={actions?.onAddTerm}
              onCopy={copy}
            />
            <NodeEdgeCountTable detail={state.value} />
          </details>
          {relatedValueCounts?.(state.value)}
          {assertions(state.value)}
        </div>
      );
    default: {
      const exhaustive: never = state;
      throw new Error(
        `unknown node detail state: ${JSON.stringify(exhaustive)}`,
      );
    }
  }
}

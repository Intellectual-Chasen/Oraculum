import { Check, PencilLine, RefreshCw, X } from "lucide-react";
import { type ReactNode, useId } from "react";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { AssertionTarget } from "@/shared/contracts/assertions";
import type {
  AssistMatchCondition,
  AssistProposalItem,
  AssistProposalState,
} from "@/shared/contracts/assistProposals";
import { assertionTargetKey } from "@/shared/lib/assertionTargetKey";
import { formatCount } from "@/shared/lib/format";
import { listKey } from "@/shared/lib/listKey";
import { ConfirmIconButton } from "@/shared/ui/ConfirmIconButton";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import {
  AuthorField,
  useCanWrite,
  useSignedInAccount,
  viewerCannotRecord,
} from "@/shared/ui/SignedInAccount";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import {
  type AssistProposalsView,
  emptyDecisionDraft,
  type LoadedAssistProposals,
} from "./useAssistProposals";

const loadingDescription = "AI 提案の読み込み中";

const stateLabels: Record<AssistProposalState, string> = {
  proposed: "未決",
  adopted: "採用",
  rejected: "却下",
};

/** AI 提案が何であるか。一覧の見出しの help に「名前: 値」の組で出す。 */
const introPairs = [
  { name: "作成", value: "AI が挙げたメモの候補" },
  { name: "根拠の確認", value: "対象のノード・エッジ・レコードの Detail" },
  { name: "採用", value: "分析者を著者とするメモの記録" },
];

/**
 * 提案を作った発言の推定条件の選択が、今の画面の選択と同じかを返す。
 *
 * **幅を送らない条件は幅 0 として比べる。** backend は幅を持たない条件を幅 0 で記録する。
 */
export function sameMatchSelection(
  proposed: readonly AssistMatchCondition[],
  current: MatchConditionSelection,
): boolean {
  const keysOf = (conditions: { key: string; tolerance: number }[]) =>
    conditions
      .map((condition) => `${condition.key}~${condition.tolerance}`)
      .sort();
  const left = keysOf(
    proposed.map((condition) => ({
      key: condition.conditionKey,
      tolerance: condition.tolerance,
    })),
  );
  const right = keysOf(
    current.conditions.map((condition) => ({
      key: condition.conditionKey,
      tolerance: condition.toleranceSeconds ?? 0,
    })),
  );
  return (
    left.length === right.length &&
    left.every((key, index) => key === right[index])
  );
}

/** 提案の対象の「名前: 値」の組。 */
function targetPairs(target: AssertionTarget): KeyValuePair[] {
  switch (target.kind) {
    case "node":
      return [
        { name: "ノード", value: <RawText text={target.nodeId ?? ""} /> },
      ];
    case "edge":
      return [
        { name: "エッジ", value: <RawText text={target.edge?.kind ?? ""} /> },
        {
          name: "始点",
          value: <RawText text={target.edge?.sourceNodeId ?? ""} />,
        },
        {
          name: "終点",
          value: <RawText text={target.edge?.targetNodeId ?? ""} />,
        },
      ];
    case "record":
      return [{ name: "対象", value: "レコード" }];
    case "source":
      return [{ name: "対象", value: "収集元" }];
    default: {
      const exhaustive: never = target.kind;
      throw new Error(`unknown assertion target kind: ${String(exhaustive)}`);
    }
  }
}

type ProposalCardProps = {
  item: AssistProposalItem;
  view: AssistProposalsView;
  matchConditions: MatchConditionSelection;
};

type DecisionFormProps = Omit<ProposalCardProps, "matchConditions"> & {
  /** 提案の対象を書いた要素の id。操作がどの提案に対するものかを支援技術に伝える。 */
  targetLineId: string;
};

/**
 * 提案中の提案の採否の操作。採用、記述を修正して採用、却下を置く。採否は取り消せないため、
 * どの操作も確認の dialog を経て送る。
 *
 * **記述を修正している間は「提案を採用」を押せない。** 押すと修正した記述を捨てて元の記述で
 * 採用する。
 */
function DecisionForm({ item, view, targetLineId }: DecisionFormProps) {
  const fieldId = useId();
  const account = useSignedInAccount();
  const canWrite = useCanWrite();
  const id = item.proposal.id;
  const draft = view.drafts.get(id) ?? emptyDecisionDraft;
  const analyst = account === undefined ? draft.analyst : undefined;
  const isBusy = view.deciding.has(id);
  const isNoteEdited =
    draft.note !== undefined && draft.note !== item.proposal.note;
  const locked = !canWrite || isBusy;
  const lockedReason = canWrite
    ? { title: "記録中", text: "記録の完了を待機" }
    : { title: viewerCannotRecord, text: "編集者の役割が必要" };
  const details = (
    <KeyValueList stacked pairs={targetPairs(item.proposal.target)} />
  );
  return (
    <fieldset disabled={locked} aria-describedby={targetLineId}>
      <legend>採否</legend>
      {canWrite ? null : <p className="note">{viewerCannotRecord}</p>}
      <AuthorField
        id={`${fieldId}-analyst`}
        label="分析者の名前"
        value={draft.analyst}
        onChange={(value) => view.changeDraft(id, { ...draft, analyst: value })}
      />
      <p>
        <label htmlFor={`${fieldId}-note`}>修正した記述</label>
        <textarea
          id={`${fieldId}-note`}
          value={draft.note ?? item.proposal.note}
          onChange={(event) =>
            view.changeDraft(id, { ...draft, note: event.target.value })
          }
        />
      </p>
      <p>
        <label htmlFor={`${fieldId}-reason`}>却下の理由</label>
        <input
          id={`${fieldId}-reason`}
          value={draft.reason}
          onChange={(event) =>
            view.changeDraft(id, { ...draft, reason: event.target.value })
          }
        />
      </p>
      <div className="flex gap-1">
        <ConfirmIconButton
          label="提案を採用"
          details={details}
          aria-describedby={targetLineId}
          isDisabled={locked || isNoteEdited}
          disabledReason={
            locked
              ? lockedReason
              : { title: "記述の修正中", text: "修正した記述で採用を使用" }
          }
          onConfirm={() => view.adopt(id, { analyst })}
        >
          <Check size={14} aria-hidden="true" />
        </ConfirmIconButton>
        <ConfirmIconButton
          label="修正した記述で採用"
          details={details}
          aria-describedby={targetLineId}
          isDisabled={locked || !isNoteEdited}
          disabledReason={
            locked
              ? lockedReason
              : { title: "記述の修正なし", text: "記述の修正が必要" }
          }
          onConfirm={() => view.adopt(id, { analyst, note: draft.note })}
        >
          <PencilLine size={14} aria-hidden="true" />
        </ConfirmIconButton>
        <ConfirmIconButton
          label="提案を却下"
          details={details}
          aria-describedby={targetLineId}
          isDisabled={locked}
          disabledReason={lockedReason}
          onConfirm={() => view.reject(id, { analyst, reason: draft.reason })}
        >
          <X size={14} aria-hidden="true" />
        </ConfirmIconButton>
      </div>
    </fieldset>
  );
}

/**
 * 提案の注意の印。エッジを追加する提案、対象の欠落、推定条件の不一致を短いラベルで出し、判定に
 * 使った値を tooltip に入れる。
 *
 * **エッジを追加する提案は、両端のノードで欠落を判定する。** 追加するエッジは、採用するまで
 * グラフに無い。
 */
function ProposalMarks({
  item,
  matchConditions,
}: {
  item: AssistProposalItem;
  matchConditions: MatchConditionSelection;
}) {
  const { proposal } = item;
  const absent = proposal.addsRelation
    ? item.endpointsInGraph === false
    : item.targetOrigin === "absent";
  const marks: ReactNode[] = [];
  if (proposal.addsRelation) {
    marks.push(
      <StatusLabel
        key="adds"
        status="pending"
        label="エッジの追加"
        details={
          <KeyValueList
            stacked
            pairs={[
              ...targetPairs(proposal.target),
              { name: "グラフへの反映", value: "採用後" },
            ]}
          />
        }
      />,
    );
  }
  if (absent) {
    marks.push(
      <StatusLabel
        key="absent"
        status="failed"
        label="対象の欠落"
        details={
          <KeyValueList
            stacked
            pairs={[
              {
                name: proposal.addsRelation ? "両端のノード" : "対象",
                value: "今の取り込み結果に無い",
              },
            ]}
          />
        }
      />,
    );
  }
  if (!sameMatchSelection(proposal.matchConditions, matchConditions)) {
    marks.push(
      <StatusLabel
        key="conditions"
        status="pending"
        label="推定条件の不一致"
        details={
          <KeyValueList
            stacked
            pairs={[
              {
                name: "提案",
                value: proposal.matchConditions
                  .map((condition) => condition.conditionKey)
                  .join(", "),
              },
              {
                name: "今",
                value: matchConditions.conditions
                  .map((condition) => condition.conditionKey)
                  .join(", "),
              },
            ]}
          />
        }
      />,
    );
  }
  return marks.length === 0 ? null : (
    <div role="status" className="flex flex-wrap gap-2">
      {marks}
    </div>
  );
}

/**
 * AI 提案 1 件を出す。**分析者のメモと区別できるよう「AI 提案」の印を付ける。** AI の記述は
 * 文字列として描く。
 */
function ProposalCard({ item, view, matchConditions }: ProposalCardProps) {
  const { proposal } = item;
  const failure = view.failures.get(proposal.id);
  const targetLineId = useId();
  const { decision } = proposal;
  return (
    <li className="ai-proposal">
      <div id={targetLineId} className="flex flex-wrap items-center gap-2">
        <span className="ai-proposal-mark">AI 提案</span>
        <KeyValueList
          pairs={[
            { name: "状態", value: stateLabels[proposal.state] },
            ...targetPairs(proposal.target),
          ]}
        />
      </div>
      <KeyValueList
        stacked
        pairs={[{ name: "記述", value: <RawText text={proposal.note} /> }]}
      />
      <KeyValueList
        pairs={[
          { name: "根拠", value: formatCount(proposal.recordRefs.length) },
          { name: "提供者", value: <RawText text={proposal.provider} /> },
          {
            name: "モデル",
            value:
              proposal.model === undefined ? undefined : (
                <RawText text={proposal.model} />
              ),
          },
          { name: "会話", value: <RawText text={proposal.conversationId} /> },
          { name: "作成", value: <RawText text={proposal.createdAt} /> },
        ]}
      />
      <ProposalMarks item={item} matchConditions={matchConditions} />
      {decision === undefined ? null : (
        <KeyValueList
          pairs={[
            { name: "決定", value: stateLabels[proposal.state] },
            { name: "分析者", value: <RawText text={decision.analyst} /> },
            { name: "時刻", value: <RawText text={decision.decidedAt} /> },
            {
              name: "理由",
              value:
                decision.reason === undefined ? undefined : (
                  <RawText text={decision.reason} />
                ),
            },
          ]}
        />
      )}
      {failure === undefined ? null : <FetchFailureNotice failure={failure} />}
      {proposal.state === "proposed" ? (
        <DecisionForm item={item} view={view} targetLineId={targetLineId} />
      ) : null}
    </li>
  );
}

/** 提案の一覧の状態を出す。この起動で使えないことを失敗と分けて出す。 */
function ProposalsState({
  view,
  children,
}: {
  view: AssistProposalsView;
  children: (loaded: LoadedAssistProposals) => ReactNode;
}) {
  if (
    view.state.status === "failed" &&
    view.state.failure.failureCode === "assist_unavailable"
  ) {
    return (
      <KeyValueList
        pairs={[
          {
            name: "状態",
            value: (
              <StatusLabel
                status="idle"
                label="使用不可"
                details={
                  <KeyValueList
                    stacked
                    pairs={[
                      {
                        name: "次の操作",
                        value: view.state.failure.nextAction,
                      },
                    ]}
                  />
                }
              />
            ),
          },
        ]}
      />
    );
  }
  return (
    <FetchStateView state={view.state} loadingDescription={loadingDescription}>
      {children}
    </FetchStateView>
  );
}

type AssistProposalListProps = {
  view: AssistProposalsView;
  matchConditions: MatchConditionSelection;
};

/**
 * AI 提案の一覧。**分析者のメモの一覧と別に置く。** 中継の有無に関わらず出す。
 */
export function AssistProposalList({
  view,
  matchConditions,
}: AssistProposalListProps) {
  return (
    <section aria-labelledby="ai-proposals-heading" className="view-pane">
      <div className="flex items-center gap-1">
        <h2 id="ai-proposals-heading">AI 提案</h2>
        <HelpPopover label="AI 提案">
          <KeyValueList stacked pairs={introPairs} />
        </HelpPopover>
        <IconButton label="一覧を再読み込み" onPress={view.reload}>
          <RefreshCw size={14} aria-hidden="true" />
        </IconButton>
      </div>
      <ProposalsState view={view}>
        {(loaded) => {
          const pending = loaded.items.filter(
            (item) => item.proposal.state === "proposed",
          ).length;
          return loaded.items.length === 0 ? (
            <KeyValueList pairs={[{ name: "AI 提案", value: "0" }]} />
          ) : (
            <>
              <KeyValueList
                pairs={[
                  { name: "未決", value: formatCount(pending) },
                  { name: "すべて", value: formatCount(loaded.items.length) },
                ]}
              />
              <ul>
                {loaded.items.map((item) => (
                  <ProposalCard
                    key={listKey([item.proposal.id])}
                    item={item}
                    view={view}
                    matchConditions={matchConditions}
                  />
                ))}
              </ul>
            </>
          );
        }}
      </ProposalsState>
    </section>
  );
}

type AssistProposalsForTargetProps = AssistProposalListProps & {
  target: AssertionTarget;
};

/**
 * 対象 1 つへの AI 提案と、未決の件数を出す。この起動で使えないとき、読み込みの途中、対象への
 * 提案が 0 件のときは何も出さない。前の 2 つは AI 提案の一覧が出す。
 */
export function AssistProposalsForTarget({
  target,
  view,
  matchConditions,
}: AssistProposalsForTargetProps) {
  if (
    view.state.status === "failed" &&
    view.state.failure.failureCode !== "assist_unavailable"
  ) {
    return (
      <section>
        <h3>AI 提案</h3>
        <FetchFailureNotice failure={view.state.failure} />
      </section>
    );
  }
  if (view.state.status !== "loaded") {
    return null;
  }
  const key = assertionTargetKey(target);
  const matched =
    key === undefined ? [] : (view.state.value.byTarget.get(key) ?? []);
  if (matched.length === 0) {
    return null;
  }
  const pending = matched.filter((item) => item.proposal.state === "proposed");
  return (
    <section aria-label="AI 提案">
      <h3>AI 提案</h3>
      <KeyValueList
        pairs={[{ name: "未決", value: formatCount(pending.length) }]}
      />
      <ul>
        {matched.map((item) => (
          <ProposalCard
            key={listKey([item.proposal.id])}
            item={item}
            view={view}
            matchConditions={matchConditions}
          />
        ))}
      </ul>
    </section>
  );
}

import { Save } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";
import type { TerminalAssignmentDraft } from "@/shared/api/terminalAssignments";
import { assertionRecordRefOf } from "@/shared/contracts/assertions";
import type {
  RecordLocator,
  TimeRange,
  Timestamp,
  TimestampInterpretation,
} from "@/shared/contracts/common";
import type { SourceIdentity } from "@/shared/contracts/sources";
import type { TerminalAssignment } from "@/shared/contracts/terminalAssignments";
import { recordPositionParts } from "@/shared/lib/recordPosition";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import {
  AuthorField,
  useCanWrite,
  useSignedInAccount,
  viewerCannotRecord,
} from "@/shared/ui/SignedInAccount";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TerminalAssignmentTable } from "@/shared/ui/TerminalAssignmentTable";
import { formFieldsClass, TextField } from "@/shared/ui/TextField";
import { TimestampField } from "@/shared/ui/TimestampField";
import { interpretationOriginLabel } from "@/shared/ui/TimestampOffsetNote";
import type { TerminalAssignmentsView } from "./useTerminalAssignments";

const loadingDescription = "端末の割り当てを読み込み中";
/** 割り当ての記録が端末をどう決めるかの説明。 */
const introPairs = [
  { name: "対象", value: "自機の識別子を持たない収集元" },
  {
    name: "入力",
    value: "端末の識別子・端末の表示名・接続元 IP のうち分かるもの",
  },
  { name: "端末の識別子なし", value: "選択した収集元を記録した端末" },
  {
    name: "同じ収集元に 2 台以上",
    value: "収集元のレコードを名前不明の端末に結び付け",
  },
];
/** ホスト名の入力欄の説明。 */
const hostnamesPairs = [
  { name: "区切り", value: "空白かカンマ" },
  { name: "形", value: "短い名前・FQDN" },
  {
    name: "端末に置くレコード",
    value: "同じホスト名を記録した適用期間の中のイベントログのレコード",
  },
  {
    name: "推定のエッジ",
    value: "コマンド行の引数が指す同じホスト名とこの端末",
  },
  {
    name: "一緒に記録する値",
    value: "端末の識別子・端末の表示名・接続元 IP のどれか",
  },
];
/** 収集元の全体への適用の説明。 */
const appliesPairs = [
  {
    name: "選択",
    value:
      "端末の識別子を持たない収集元のレコードすべてをこの端末の記録として扱う",
  },
  {
    name: "この端末のものになる値",
    value:
      "レコードが指すプロセス・ファイル・アカウント・ループバックとリンクローカルのアドレス",
  },
  { name: "未選択", value: "接続元 IP と端末の対応だけを記録" },
];
/** タイムゾーンを当てた記録期間を適用期間にする割り当ての説明。 */
const interpretedPeriodPairs = [
  { name: "適用期間の記録", value: "タイムゾーンを持たない時刻の文字列" },
  {
    name: "UTC 時刻への変換",
    value: "グラフの構築時に収集元のタイムゾーンで変換",
  },
  { name: "タイムゾーンの記録の取り消し", value: "割り当てを適用しない" },
  {
    name: "タイムゾーンの変更",
    value: "適用期間とレコードの時刻を新しいタイムゾーンで UTC 時刻に変換",
  },
];

/** ホスト名の入力欄の文字列を、空白とカンマで区切ったホスト名の並びにする。 */
function hostnamesOf(text: string): string[] {
  return text.split(/[\s,]+/).filter((name) => name !== "");
}

/**
 * タイムゾーンを当てた記録期間を確かめられたか。
 *
 * - `loading`: 収集元の一覧を読み込んでいるか、タイムゾーンを記録した後に再読み込みしている。
 * - `failed`: 収集元の一覧を読み込めていない。
 * - `conflicted`: 収集元に適用中の解釈が 2 件以上あり、backend はどのタイムゾーンも使わない。
 * - `loaded`: 一覧を読み込めた。`range` はタイムゾーンを当てた記録期間であり、両端は
 *   タイムゾーンを持たない時刻の文字列と、当てたタイムゾーン (`interpretation`) の組である。
 *   タイムゾーンを持たない収集元では出ない。
 */
export type InterpretedRangeState =
  | { status: "loading" }
  | { status: "failed" }
  | { status: "conflicted" }
  | { status: "loaded"; range: TimeRange | undefined };

const noInterpretedRange: InterpretedRangeState = {
  status: "loaded",
  range: undefined,
};

type TerminalAssignmentsProps = {
  view: TerminalAssignmentsView;
  /** 適用期間を読み取る収集元。選んでいないときは入力欄を出さない。 */
  selectedSource: SourceIdentity | undefined;
  /**
   * 記録期間を持たない収集元に、分析者が記録したタイムゾーンを当てた記録期間の状態。収集元が
   * 記録期間を持つときは使わない。渡さないときは、タイムゾーンを当てた期間を持たない状態として
   * 扱う。
   */
  interpretedRange?: InterpretedRangeState;
  /** 根拠に挙げるレコード。原文を開いているレコードを使う。 */
  basisRecordRef: RecordLocator | undefined;
  /**
   * 収集元の sourceId から file 名を探す表。一覧の「適用する収集元」を file 名で出す。
   * 表に無い収集元は sourceId をそのまま出す。
   */
  sourceFileNames?: ReadonlyMap<string, string>;
  /** 収集元の sourceId から今のタイムゾーンを求める表。一覧の期間をそのタイムゾーンで UTC に直す。 */
  sourceInterpretations?: ReadonlyMap<string, TimestampInterpretation>;
};

/**
 * 分析者が端末の割り当てを記録し、分析者が記録した割り当てを一覧する。
 *
 * **自機の識別子を持たない収集元へ端末を与える経路である。** Linux の監査ログは接続元
 * IP もホスト名も記録しないため、レコードから端末を求められない。
 *
 * **記録した割り当ては 1 件で端末を確定させる** (`backend/core/assignment.go` の
 * `ResolveTerminal`)。取り消せない処理の結果であるため、「?」を開かずに読めるラベルで出す。
 */
export function TerminalAssignments({
  view,
  selectedSource,
  interpretedRange,
  basisRecordRef,
  sourceFileNames,
  sourceInterpretations,
}: TerminalAssignmentsProps) {
  return (
    <section aria-label="端末の割り当て">
      <h2 className="section-title">
        端末の割り当て
        <HelpPopover label="端末の割り当て">
          <KeyValueList stacked pairs={introPairs} />
        </HelpPopover>
      </h2>
      <KeyValueList
        pairs={[{ name: "確定", value: "記録 1 件で端末を確定" }]}
      />
      <TerminalAssignmentForm
        view={view}
        selectedSource={selectedSource}
        interpretedRange={interpretedRange}
        basisRecordRef={basisRecordRef}
      />
      <FetchStateView
        state={view.state}
        loadingDescription={loadingDescription}
      >
        {(assignments) => (
          <AssignmentList
            assignments={assignments}
            sourceFileNames={sourceFileNames}
            sourceInterpretations={sourceInterpretations}
          />
        )}
      </FetchStateView>
    </section>
  );
}

/** 記録済みの割り当ての一覧。 */
function AssignmentList({
  assignments,
  sourceFileNames,
  sourceInterpretations,
}: {
  assignments: TerminalAssignment[];
  sourceFileNames: ReadonlyMap<string, string> | undefined;
  sourceInterpretations:
    | ReadonlyMap<string, TimestampInterpretation>
    | undefined;
}) {
  if (assignments.length === 0) {
    return <StatusLabel status="idle" label="分析者が記録した割り当てなし" />;
  }
  return (
    <TerminalAssignmentTable
      assignments={assignments}
      caption="分析者が記録した割り当て"
      sourceFileNames={sourceFileNames}
      sourceInterpretations={sourceInterpretations}
    />
  );
}

/** 適用期間を確かめられないときの状態のラベル。 */
const unreadPeriodStatus: Record<
  Exclude<InterpretedRangeState["status"], "loaded">,
  ReactNode
> = {
  loading: <StatusLabel status="running" label="収集元の一覧を読み込み中" />,
  failed: (
    <StatusLabel
      status="failed"
      label="収集元の一覧の読み込みに失敗"
      details="適用期間を確認不可"
    />
  ),
  conflicted: (
    <StatusLabel
      status="failed"
      label="収集元のタイムゾーンの記録が競合"
      details="適用期間が未確定"
    />
  ),
};

/** 記録期間もタイムゾーンを当てた記録期間も持たない収集元の状態のラベル。 */
const noPeriodStatus = (
  <StatusLabel
    status="idle"
    label="記録期間なし"
    details={
      <KeyValueList
        stacked
        pairs={[{ name: "次の操作", value: "収集元のタイムゾーンを記録" }]}
      />
    }
  />
);

/**
 * 分析者が入力した適用期間の端を返す。入力が空か端の文字列と同じときは `base` をそのまま返す。
 * 入力した端は、`base` と同じ形の文字列 (端末時刻・タイムゾーンの状態) として、原文の文字列と
 * 正規化値に入れる。
 */
function editedEnd(base: Timestamp, text: string | undefined): Timestamp {
  const trimmed = text?.trim() ?? "";
  if (trimmed === "" || trimmed === base.normalized) {
    return base;
  }
  return { ...base, rawText: trimmed, normalized: trimmed };
}

/** 当てたタイムゾーンを外した時刻を返す。割り当ての期間はタイムゾーンを持たない文字列で送る。 */
function withoutInterpretation(timestamp: Timestamp): Timestamp {
  const { interpretation: _interpretation, ...local } = timestamp;
  return local;
}

/**
 * 記録期間を持たない収集元の、適用期間と、期間の表示を返す。`range` は、タイムゾーンを
 * 当てた記録期間があるときだけ出る。
 *
 * **割り当ての期間は、タイムゾーンを外した時刻の文字列で送る。** backend はグラフを構築する
 * ときに、その収集元の今のタイムゾーンで期間を UTC に直す
 * (`backend/pipeline/time_interpretation.go` の `withInterpretedRange`)。
 */
function interpretedPeriodOf(
  source: SourceIdentity,
  state: InterpretedRangeState,
): { range: TimeRange | undefined; note: ReactNode } {
  if (state.status !== "loaded") {
    return { range: undefined, note: unreadPeriodStatus[state.status] };
  }
  const { range } = state;
  const interpretation = range?.from.interpretation;
  if (range === undefined || interpretation === undefined) {
    return { range: undefined, note: noPeriodStatus };
  }
  return {
    range: {
      from: withoutInterpretation(range.from),
      to: withoutInterpretation(range.to),
    },
    note: (
      <div className="flex items-center gap-1">
        <KeyValueList
          pairs={[
            { name: "収集元", value: <RawText text={source.fileName} /> },
            {
              name: "適用期間",
              value: (
                <>
                  <RawText text={range.from.normalized ?? "—"} />
                  {" – "}
                  <RawText text={range.to.normalized ?? "—"} />
                </>
              ),
            },
            {
              name: "タイムゾーン",
              value: (
                <>
                  UTC
                  <RawText text={interpretation.offset} />
                </>
              ),
            },
            {
              name: "出どころ",
              value: interpretationOriginLabel(interpretation),
            },
          ]}
        />
        <HelpPopover label="適用期間のタイムゾーン">
          <KeyValueList stacked pairs={interpretedPeriodPairs} />
        </HelpPopover>
      </div>
    ),
  };
}

/** 割り当てを記録する入力欄。 */
function TerminalAssignmentForm({
  view,
  selectedSource,
  interpretedRange,
  basisRecordRef,
}: TerminalAssignmentsProps) {
  const clientIpId = useId();
  const terminalIdId = useId();
  const hostnameId = useId();
  const hostnamesId = useId();
  const authorId = useId();
  const derivationId = useId();
  const appliesId = useId();
  const appliesNoteId = useId();
  const periodFromId = useId();
  const periodToId = useId();
  // 入力していない端は undefined であり、収集元の期間の端を使う。
  const [periodFrom, setPeriodFrom] = useState<string | undefined>(undefined);
  const [periodTo, setPeriodTo] = useState<string | undefined>(undefined);
  const [clientIp, setClientIp] = useState("");
  const [terminalId, setTerminalId] = useState("");
  const [terminalHostname, setTerminalHostname] = useState("");
  const [hostnames, setHostnames] = useState("");
  const account = useSignedInAccount();
  const canWrite = useCanWrite();
  const [author, setAuthor] = useState("");
  const [derivation, setDerivation] = useState("");
  // **初期値は切にする。** 入にすると、収集元に現れるリンクローカルのアドレスなど、端末の
  // 範囲で識別する対象がすべてこの端末のものになる。分析者が適用範囲を読んでから選ぶ。
  const [appliesToSource, setAppliesToSource] = useState(false);
  const recordedCount = view.recordedCount;

  // **記録できた入力だけを消す。** 失敗した入力を消すと、分析者が書いた理由が消える。
  useEffect(() => {
    if (recordedCount === 0) {
      return;
    }
    setClientIp("");
    setTerminalId("");
    setTerminalHostname("");
    setHostnames("");
    setDerivation("");
    setAppliesToSource(false);
    setPeriodFrom(undefined);
    setPeriodTo(undefined);
  }, [recordedCount]);

  // 収集元を選び直したときは、入力した期間を新しい収集元の期間に戻す。
  const selectedSourceId = selectedSource?.sourceId;
  useEffect(() => {
    void selectedSourceId;
    setPeriodFrom(undefined);
    setPeriodTo(undefined);
  }, [selectedSourceId]);

  if (selectedSource === undefined) {
    return (
      <StatusLabel
        status="idle"
        label="収集元の選択なし"
        details="Artifacts で収集元を選択"
      />
    );
  }
  const { observedRangeFirst, observedRangeLast } = selectedSource;
  const observedRange: TimeRange | undefined =
    observedRangeFirst !== undefined && observedRangeLast !== undefined
      ? { from: observedRangeFirst, to: observedRangeLast }
      : undefined;
  const period =
    observedRange !== undefined
      ? {
          range: observedRange,
          note: (
            <KeyValueList
              pairs={[
                {
                  name: "収集元",
                  value: <RawText text={selectedSource.fileName} />,
                },
                { name: "適用期間の初期値", value: "記録期間" },
              ]}
            />
          ),
        }
      : interpretedPeriodOf(
          selectedSource,
          interpretedRange ?? noInterpretedRange,
        );
  if (period.range === undefined) {
    return period.note;
  }
  const validRange: TimeRange = {
    from: editedEnd(period.range.from, periodFrom),
    to: editedEnd(period.range.to, periodTo),
  };
  if (basisRecordRef === undefined) {
    return (
      <>
        {period.note}
        <StatusLabel
          status="idle"
          label="根拠のレコードなし"
          details="Timeline か Graph でレコードを開く"
        />
      </>
    );
  }

  // 端末の識別子を入力しないときは、選んだ収集元を記録した端末を指す。割り当ては収集元の
  // 全体に必ず適用するため、選択を固定する。
  const isTerminalIdBlank = terminalId.trim() === "";
  const isAppliedToSource = isTerminalIdBlank || appliesToSource;

  // 空にした入力欄は、画面の表示と送る値が食い違うため送らない。
  const isFromBlank = periodFrom?.trim() === "";
  const isToBlank = periodTo?.trim() === "";
  const isPeriodBlank = isFromBlank || isToBlank;
  const blankPeriodError = "入力が必要";

  const draft: TerminalAssignmentDraft = {
    clientIp,
    terminalId,
    terminalHostname,
    terminalHostnames: hostnamesOf(hostnames),
    sourceId: selectedSource.sourceId,
    sourceContentSha256: selectedSource.contentSha256,
    assignmentValidRange: validRange,
    derivation,
    basisRecordRefs: [assertionRecordRefOf(basisRecordRef)],
    author: account === undefined ? author : undefined,
    appliesToSourceId: isAppliedToSource ? selectedSource.sourceId : undefined,
  };

  return (
    <form
      className={formFieldsClass}
      onSubmit={(event) => {
        event.preventDefault();
        if (canWrite && !view.recording && !isPeriodBlank) {
          void view.record(draft);
        }
      }}
    >
      {period.note}
      <KeyValueList
        pairs={[
          {
            name: "根拠のレコード",
            value: <RawText text={basisRecordRef.sourceFileName} />,
          },
          ...recordPositionParts(basisRecordRef),
        ]}
      />
      <TimestampField
        id={periodFromId}
        label="適用期間の始まり"
        defaultOffset=""
        description="初期値: 記録期間の始まり"
        errorMessage={isFromBlank ? blankPeriodError : undefined}
        mono
        value={periodFrom ?? period.range.from.normalized ?? ""}
        onChange={setPeriodFrom}
      />
      <TimestampField
        id={periodToId}
        label="適用期間の終わり"
        defaultOffset=""
        description="初期値: 記録期間の終わり"
        errorMessage={isToBlank ? blankPeriodError : undefined}
        mono
        value={periodTo ?? period.range.to.normalized ?? ""}
        onChange={setPeriodTo}
      />
      <TextField
        id={clientIpId}
        label="接続元 IP"
        mono
        value={clientIp}
        onChange={setClientIp}
      />
      <TextField
        id={terminalIdId}
        label="端末の識別子"
        mono
        value={terminalId}
        onChange={setTerminalId}
      />
      <TextField
        id={hostnameId}
        label="端末の表示名"
        value={terminalHostname}
        onChange={setTerminalHostname}
      />
      <div className="flex items-end gap-1">
        <TextField
          id={hostnamesId}
          label="ホスト名"
          className="grow"
          value={hostnames}
          onChange={setHostnames}
        />
        <HelpPopover label="ホスト名">
          <KeyValueList stacked pairs={hostnamesPairs} />
        </HelpPopover>
      </div>
      <AuthorField
        id={authorId}
        label="分析者"
        value={author}
        onChange={setAuthor}
      />
      <TextField
        id={derivationId}
        label="理由"
        multiline
        value={derivation}
        onChange={setDerivation}
      />
      <div className="flex flex-wrap items-center gap-2">
        <label htmlFor={appliesId}>
          <input
            id={appliesId}
            type="checkbox"
            aria-describedby={appliesNoteId}
            checked={isAppliedToSource}
            disabled={isTerminalIdBlank}
            onChange={(event) => setAppliesToSource(event.target.checked)}
          />
          収集元の全体に適用
        </label>
        <HelpPopover label="収集元の全体への適用">
          <KeyValueList stacked pairs={appliesPairs} />
        </HelpPopover>
        <span id={appliesNoteId}>
          <KeyValueList
            pairs={[
              {
                name: "適用範囲",
                value: isAppliedToSource
                  ? "収集元の全体"
                  : "接続元 IP と端末の対応",
              },
            ]}
          />
        </span>
      </div>
      <IconButton
        type="submit"
        variant="primary"
        label="割り当てを記録"
        isDisabled={!canWrite || view.recording}
        disabledReason={
          canWrite
            ? { title: "記録中", text: "記録の結果を待機中" }
            : { title: viewerCannotRecord, text: "記録できる役割でログイン" }
        }
      >
        <Save size={14} aria-hidden="true" />
      </IconButton>
      {canWrite ? null : <p className="note">{viewerCannotRecord}</p>}
      {view.recordFailure !== undefined && (
        <FetchFailureNotice failure={view.recordFailure} />
      )}
    </form>
  );
}

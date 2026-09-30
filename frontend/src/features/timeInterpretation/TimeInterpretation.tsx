import { Download, Plus, Save, Undo2, Upload, X } from "lucide-react";
import { type ReactNode, useEffect, useId, useState } from "react";
import type { AssertionRevisionDraft } from "@/shared/api/assertions";
import {
  type Assertion,
  type AssertionRecordRef,
  type AssertionRevision,
  type AssertionState,
  assertionRecordRefOf,
} from "@/shared/contracts/assertions";
import type { RecordLocator } from "@/shared/contracts/common";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { formatCount } from "@/shared/lib/format";
import { listKey } from "@/shared/lib/listKey";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { positionKindLabels } from "@/shared/lib/recordLabels";
import { recordPositionParts, recordRefKey } from "@/shared/lib/recordPosition";
import type { DisabledReason } from "@/shared/ui/Button";
import { DataTable } from "@/shared/ui/DataTable";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import {
  AuthorField,
  useCanWrite,
  useSignedInAccount,
  viewerCannotRecord,
} from "@/shared/ui/SignedInAccount";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { formFieldsClass, TextField } from "@/shared/ui/TextField";
import {
  interpretationOriginLabel,
  offsetUnknownLabel,
} from "@/shared/ui/TimestampOffsetNote";
import type { TimeInterpretationsView } from "./useTimeInterpretations";

/** 記録を送っている間の button の押せない理由。 */
const recordingReason: DisabledReason = {
  title: "記録中",
  text: "記録の結果を待機中",
};

const stateLabels: Record<AssertionState, string> = {
  active: "適用中",
  withdrawn: "取り消し",
};

/** 起動時の指定の出どころの名前。 */
const importOriginLabel = interpretationOriginLabel({ offset: "" });
/** 分析者の記録の出どころの名前。 */
const assertionOriginLabel = interpretationOriginLabel({
  offset: "",
  assertionId: "-",
});

/** UTC からのずれを「UTC+09:00」で書く。 */
function zoneText(offset: string): ReactNode {
  return (
    <>
      UTC
      <RawText text={offset} />
    </>
  );
}

function basisRefPosition(basisRef: AssertionRecordRef): number | undefined {
  return basisRef.positionKind === "sequence_number"
    ? basisRef.sequenceNumber
    : basisRef.positionKind === "line_number"
      ? basisRef.lineNumber
      : basisRef.byteOffset;
}

/** 保存した根拠のレコード 1 件を読み上げで指す名前。収集元と位置を挙げる。 */
function basisRefName(
  basisRef: AssertionRecordRef,
  fileNames: ReadonlyMap<string, string>,
): string {
  const fileName =
    fileNames.get(basisRef.sourceContentSha256) ?? basisRef.sourceContentSha256;
  return [
    toVisibleRawText(fileName),
    positionKindLabels[basisRef.positionKind],
    basisRefPosition(basisRef),
  ]
    .filter((part) => part !== undefined)
    .join(" ");
}

/**
 * 保存した根拠のレコード 1 件の「収集元」と「位置」の組。収集元は file 名で書き、表に無い
 * 収集元は sha256 で書く。
 *
 * **画面が組んだ文字列を原資料の文字列と同じ部品で出さない。** 原資料から来るのは file 名だけで
 * ある。
 */
function basisRefPairs(
  basisRef: AssertionRecordRef,
  fileNames: ReadonlyMap<string, string>,
): KeyValuePair[] {
  const position = basisRefPosition(basisRef);
  const fileName = fileNames.get(basisRef.sourceContentSha256);
  return [
    {
      name: "収集元",
      value: <RawText text={fileName ?? basisRef.sourceContentSha256} />,
    },
    {
      name: positionKindLabels[basisRef.positionKind],
      value:
        position === undefined ? (
          <MissingValue description="値なし" />
        ) : (
          `${position}`
        ),
    },
  ];
}

/** 保存した根拠のレコード 1 件の一覧の key。 */
function assertionRefKey(ref: AssertionRecordRef): string {
  return listKey([
    ref.sourceContentSha256,
    ref.positionKind,
    ref.sequenceNumber,
    ref.lineNumber,
    ref.byteOffset,
  ]);
}

/** 保存した根拠のレコードの並び。1 件を 1 行の値の組で書く。 */
function BasisRefList({
  refs,
  fileNames,
}: {
  refs: AssertionRecordRef[];
  fileNames: ReadonlyMap<string, string>;
}) {
  if (refs.length === 0) {
    return <MissingValue description="なし" />;
  }
  return (
    <ul>
      {refs.map((ref) => (
        <li key={assertionRefKey(ref)}>
          <KeyValueList pairs={basisRefPairs(ref, fileNames)} />
        </li>
      ))}
    </ul>
  );
}

/** 解釈の改訂を、古い順に現在の改訂まで並べる。 */
function revisionsOf(assertion: Assertion): AssertionRevision[] {
  return [
    ...assertion.history,
    {
      revisionNumber: assertion.revisionNumber,
      state: assertion.state,
      author: assertion.author,
      recordedAt: assertion.recordedAt,
      basis: assertion.basis,
      timeOffset: assertion.timeOffset,
    },
  ];
}

/**
 * 起動時の指定のタイムゾーンの組。指定が無い収集元の時刻は UTC に直せない。
 * (`backend/pipeline/time_interpretation.go` の `withTimeInterpretations`)
 */
function importZonePairs(importTimeOffset: string | undefined): KeyValuePair[] {
  return importTimeOffset === undefined
    ? [{ name: "タイムゾーン", value: offsetUnknownLabel }]
    : [
        { name: "タイムゾーン", value: zoneText(importTimeOffset) },
        { name: "出どころ", value: importOriginLabel },
      ];
}

/**
 * 収集元の時刻に今あてているタイムゾーンと、その出どころを出す。
 *
 * 適用中の解釈があれば、その解釈を使う。無ければ、取り込みの起動で指定したタイムゾーンを使う。
 */
function CurrentInterpretation({
  source,
  assertion,
  importTimeOffset,
}: {
  source: SourceIdentity;
  assertion: Assertion | undefined;
  importTimeOffset: string | undefined;
}) {
  const sourcePair: KeyValuePair = {
    name: "収集元",
    value: <RawText text={source.fileName} />,
  };
  if (assertion === undefined || assertion.state !== "active") {
    return (
      <KeyValueList
        pairs={[
          sourcePair,
          {
            name: "記録",
            value:
              assertion === undefined ? "なし" : stateLabels[assertion.state],
          },
          ...importZonePairs(importTimeOffset),
        ]}
      />
    );
  }
  return (
    <KeyValueList
      pairs={[
        sourcePair,
        { name: "タイムゾーン", value: zoneText(assertion.timeOffset ?? "") },
        { name: "出どころ", value: assertionOriginLabel },
        { name: "分析者", value: <RawText text={assertion.author} /> },
        {
          name: "記録した時刻",
          value: <RawText text={assertion.recordedAt} />,
        },
        {
          name: importOriginLabel,
          value:
            importTimeOffset === undefined
              ? undefined
              : zoneText(importTimeOffset),
        },
      ]}
    />
  );
}

function RevisionHistory({
  assertion,
  fileNames,
}: {
  assertion: Assertion;
  fileNames: ReadonlyMap<string, string>;
}) {
  return (
    <DataTable
      label="変更履歴"
      showCaption
      rows={revisionsOf(assertion)}
      rowKey={(revision) => `${revision.revisionNumber}`}
      columns={[
        {
          key: "revision",
          header: "改訂の番号",
          numeric: true,
          cell: (revision) => formatCount(revision.revisionNumber),
        },
        {
          key: "state",
          header: "状態",
          cell: (revision) => stateLabels[revision.state],
        },
        {
          key: "zone",
          header: "タイムゾーン",
          mono: true,
          cell: (revision) =>
            revision.timeOffset === undefined ? (
              <MissingValue description="なし" />
            ) : (
              zoneText(revision.timeOffset)
            ),
        },
        {
          key: "author",
          header: "分析者",
          cell: (revision) => <RawText text={revision.author} />,
        },
        {
          key: "recordedAt",
          header: "記録した時刻",
          mono: true,
          cell: (revision) => <RawText text={revision.recordedAt} />,
        },
        {
          key: "note",
          header: "メモ",
          cell: (revision) => <RawText text={revision.basis.note} />,
        },
        {
          key: "refs",
          header: "根拠のレコード",
          cell: (revision) => (
            <BasisRefList
              refs={revision.basis.recordRefs}
              fileNames={fileNames}
            />
          ),
        },
      ]}
    />
  );
}

/**
 * 適用中の解釈が 2 件以上ある収集元を出す。
 *
 * **どちらのタイムゾーンも使わないことを出す。** backend はこの収集元に解釈を適用せず、時刻は
 * 取り込みの起動で指定したタイムゾーンで UTC に直す。指定が無ければ UTC に直せない。どちらを
 * 記録し直すかを画面が決めないため、入力欄を出さない。
 */
function ConflictedInterpretations({
  source,
  assertions,
  fileNames,
  importTimeOffset,
}: {
  source: SourceIdentity;
  assertions: Assertion[];
  fileNames: ReadonlyMap<string, string>;
  importTimeOffset: string | undefined;
}) {
  return (
    <>
      <div role="status">
        <KeyValueList
          pairs={[
            { name: "収集元", value: <RawText text={source.fileName} /> },
            {
              name: "記録",
              value: (
                <StatusLabel
                  status="failed"
                  label="競合"
                  details={
                    <KeyValueList
                      stacked
                      pairs={[{ name: "適用", value: "なし" }]}
                    />
                  }
                />
              ),
            },
            {
              name: "適用中の記録",
              value: formatCount(assertions.length),
            },
            ...importZonePairs(importTimeOffset),
          ]}
        />
      </div>
      {assertions.map((assertion) => (
        <RevisionHistory
          key={assertion.id}
          assertion={assertion}
          fileNames={fileNames}
        />
      ))}
    </>
  );
}

/** 先に保存された値と入力した値の表の 1 行。 */
type ConflictRow = { name: string; theirs: ReactNode; mine: ReactNode };

/**
 * 別の分析者が先に記録した解釈と、退けられた自分の入力を並べ、採る値を分析者に選ばせる。
 *
 * **どちらの値も通知せずに捨てない。** 「入力した値で上書き」は先に保存された改訂を元にして
 * 送り直し、先に保存された改訂は履歴に残る。「先に保存された値を採用」は入力欄を先に保存された
 * 値にし、何も送らない。
 */
function InterpretationConflict({
  theirs,
  mine,
  fileNames,
  recording,
  onReviseWithMine,
  onTakeTheirs,
}: {
  theirs: Assertion;
  mine: AssertionRevisionDraft;
  fileNames: ReadonlyMap<string, string>;
  recording: boolean;
  onReviseWithMine: () => void;
  onTakeTheirs: () => void;
}) {
  const refsOf = (refs: AssertionRecordRef[]) => (
    <>
      {formatCount(refs.length)}
      {refs.length === 0 ? null : (
        <BasisRefList refs={refs} fileNames={fileNames} />
      )}
    </>
  );
  const text = (value: string) => <RawText text={value} />;
  const zone = (offset: string | undefined) =>
    offset === undefined ? (
      <MissingValue description="なし" />
    ) : (
      zoneText(offset)
    );
  const rows: ConflictRow[] = [
    {
      name: "状態",
      theirs: stateLabels[theirs.state],
      mine: stateLabels[mine.state],
    },
    {
      name: "タイムゾーン",
      theirs: zone(theirs.timeOffset),
      mine: zone(mine.timeOffset),
    },
    { name: "メモ", theirs: text(theirs.basis.note), mine: text(mine.note) },
    {
      name: "根拠のレコード",
      theirs: refsOf(theirs.basis.recordRefs),
      mine: refsOf(mine.recordRefs),
    },
  ];
  // 取り消しを送り直す意味は、相手も取り消していれば無い。
  const alreadyWithdrawn =
    mine.state === "withdrawn" && theirs.state === "withdrawn";
  return (
    <div role="alert">
      <StatusLabel status="failed" label="別の分析者の改訂と競合" />
      <KeyValueList
        pairs={[
          { name: "改訂した分析者", value: text(theirs.author) },
          { name: "改訂した時刻", value: text(theirs.recordedAt) },
        ]}
      />
      <DataTable
        label="先に保存された値と入力した値"
        showCaption
        rows={rows}
        rowKey={(row) => row.name}
        columns={[
          {
            key: "name",
            header: "フィールド",
            rowHeader: true,
            cell: (row) => row.name,
          },
          {
            key: "theirs",
            header: "先に保存された値",
            cell: (row) => row.theirs,
          },
          { key: "mine", header: "入力した値", cell: (row) => row.mine },
        ]}
      />
      {alreadyWithdrawn ? (
        <StatusLabel status="idle" label="取り消し済み" />
      ) : (
        <IconButton
          label="入力した値で上書き"
          isDisabled={recording}
          disabledReason={recordingReason}
          onPress={onReviseWithMine}
        >
          <Upload size={14} aria-hidden="true" />
        </IconButton>
      )}
      <IconButton
        label="先に保存された値を採用"
        isDisabled={recording}
        disabledReason={recordingReason}
        onPress={onTakeTheirs}
      >
        <Download size={14} aria-hidden="true" />
      </IconButton>
    </div>
  );
}

/** 根拠のレコードの表の 1 行。 */
type BasisRow = {
  key: string;
  /** 行のレコードを読み上げで指す名前。収集元と位置を挙げる。 */
  name: string;
  pairs: KeyValuePair[];
  origin: string;
  remove: () => void;
};

type InterpretationFormProps = {
  source: SourceIdentity;
  assertion: Assertion | undefined;
  openedRecordRef: RecordLocator | undefined;
  view: TimeInterpretationsView;
  /**
   * 最後に成功した記録がこの収集元であるときの、記録の成功の通算の回数。別の収集元である
   * ときと、成功がまだ無いときは 0 である。値の変化だけを入力欄を戻す引き金にする。
   */
  recordedHere: number;
  /** 収集元の内容の識別から file 名を探す表。 */
  fileNames: ReadonlyMap<string, string>;
};

/**
 * 解釈を記録・変更・取り消す入力欄。
 *
 * **根拠のレコードは分析者が開いたレコードから 1 件ずつ追加する。** 解釈の根拠は、この収集元の
 * レコードと、同じイベントを UTC で記録した別の収集元のレコードを比べた結果である。両側を
 * 開いては追加する操作で、比べた 2 件を根拠に残せる。
 */
function InterpretationForm({
  source,
  assertion,
  openedRecordRef,
  view,
  recordedHere,
  fileNames,
}: InterpretationFormProps) {
  const fieldId = useId();
  const applied =
    assertion?.state === "active" ? assertion.timeOffset : undefined;
  const [offset, setOffset] = useState(applied ?? "");
  const account = useSignedInAccount();
  const canWrite = useCanWrite();
  const [author, setAuthor] = useState("");
  const [note, setNote] = useState("");
  const [basis, setBasis] = useState<RecordLocator[]>([]);
  // 「先に保存された値を採用」で引き継いだ根拠のレコード。file 名を持たないため、開いたレコードと
  // 分けて持つ。
  const [adoptedRefs, setAdoptedRefs] = useState<AssertionRecordRef[]>([]);

  // この収集元への記録が成功したら根拠の記述とレコードを空へ戻す。前の改訂の根拠を次の改訂に
  // 付けない。**分析者の名前とタイムゾーンは残す。** 同じ分析者が続けて記録する。
  useEffect(() => {
    if (recordedHere === 0) {
      return;
    }
    setNote("");
    setBasis([]);
    setAdoptedRefs([]);
  }, [recordedHere]);

  // 取り消しの改訂は、入力欄の値でなく適用中のずれを持つ。取り消すのは適用中の解釈であり、
  // 入力欄を書き換えた値を取り消した改訂として履歴に残さない。
  // 改訂は画面が読んだ改訂の番号を元にする。別の分析者が先に改訂していれば server が退ける。
  const draft = (state: AssertionState, timeOffset: string) => ({
    target: {
      kind: "source" as const,
      sourceContentSha256: source.contentSha256,
    },
    author: account === undefined ? author : undefined,
    note,
    recordRefs: [...adoptedRefs, ...basis.map(assertionRecordRefOf)],
    timeOffset,
    state,
  });
  const submit = () => {
    // 記録中の送信を退ける。入力欄の Enter でも form は送られ、同じ解釈を 2 度記録する。
    if (view.recording) {
      return;
    }
    if (assertion === undefined) {
      view.create(draft("active", offset));
      return;
    }
    view.revise(assertion.id, {
      ...draft("active", offset),
      baseRevision: assertion.revisionNumber,
    });
  };
  const withdraw = (current: Assertion, timeOffset: string) =>
    view.revise(current.id, {
      ...draft("withdrawn", timeOffset),
      baseRevision: current.revisionNumber,
    });
  const conflict =
    view.conflict?.sourceContentSha256 === source.contentSha256
      ? view.conflict
      : undefined;
  const takeTheirs = (theirs: Assertion) => {
    setOffset(theirs.state === "active" ? (theirs.timeOffset ?? "") : "");
    setNote(theirs.basis.note);
    setBasis([]);
    setAdoptedRefs(theirs.basis.recordRefs);
    view.dismissConflict();
  };
  // 取り消しを送り直すときは、相手の現在のずれを取り消す。退けられた時点のずれは古い。
  const reviseWithMine = (theirs: Assertion, mine: AssertionRevisionDraft) =>
    view.revise(theirs.id, {
      ...mine,
      timeOffset:
        mine.state === "withdrawn" ? theirs.timeOffset : mine.timeOffset,
      baseRevision: theirs.revisionNumber,
    });
  const opened = openedRecordRef;
  const alreadyAdded =
    opened !== undefined &&
    basis.some((item) => recordRefKey(item) === recordRefKey(opened));
  const basisRows: BasisRow[] = [
    ...adoptedRefs.map((ref) => ({
      key: `adopted:${assertionRefKey(ref)}`,
      name: basisRefName(ref, fileNames),
      pairs: basisRefPairs(ref, fileNames),
      origin: "先に保存された値",
      remove: () =>
        setAdoptedRefs((current) =>
          current.filter(
            (other) => assertionRefKey(other) !== assertionRefKey(ref),
          ),
        ),
    })),
    ...basis.map((item) => ({
      key: `opened:${recordRefKey(item)}`,
      name: [
        toVisibleRawText(item.sourceFileName),
        ...recordPositionParts(item).map(
          (part) => `${part.name} ${part.value}`,
        ),
      ].join(" "),
      pairs: [
        { name: "収集元", value: <RawText text={item.sourceFileName} /> },
        ...recordPositionParts(item),
      ],
      origin: "開いたレコード",
      remove: () =>
        setBasis((current) =>
          current.filter((other) => recordRefKey(other) !== recordRefKey(item)),
        ),
    })),
  ];

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <fieldset disabled={!canWrite} className={formFieldsClass}>
        {canWrite ? null : <p className="note">{viewerCannotRecord}</p>}
        {conflict === undefined ? null : (
          <InterpretationConflict
            theirs={conflict.theirs}
            mine={conflict.mine}
            fileNames={fileNames}
            recording={view.recording}
            onReviseWithMine={() =>
              reviseWithMine(conflict.theirs, conflict.mine)
            }
            onTakeTheirs={() => takeTheirs(conflict.theirs)}
          />
        )}
        <TextField
          id={`${fieldId}-offset`}
          label="タイムゾーン"
          placeholder="+09:00"
          mono
          value={offset}
          onChange={setOffset}
        />
        <AuthorField
          id={`${fieldId}-author`}
          label="分析者"
          value={author}
          onChange={setAuthor}
        />
        <TextField
          id={`${fieldId}-note`}
          label="メモ"
          multiline
          value={note}
          onChange={setNote}
        />
        <div className="flex items-center gap-2">
          <KeyValueList
            pairs={[
              { name: "根拠のレコード", value: formatCount(basisRows.length) },
            ]}
          />
          <IconButton
            label="開いているレコードを根拠に追加"
            isDisabled={opened === undefined || alreadyAdded}
            disabledReason={
              opened === undefined
                ? { title: "開いているレコードなし", text: "Record で開く" }
                : { title: "追加済み", text: "未追加のレコードを開く" }
            }
            onPress={() => {
              if (opened !== undefined) {
                setBasis((current) => [...current, opened]);
              }
            }}
          >
            <Plus size={14} aria-hidden="true" />
          </IconButton>
        </div>
        {basisRows.length === 0 ? null : (
          <DataTable
            label="根拠のレコード"
            rows={basisRows}
            rowKey={(row) => row.key}
            columns={[
              {
                key: "record",
                header: "レコード",
                cell: (row) => <KeyValueList pairs={row.pairs} />,
              },
              { key: "origin", header: "出どころ", cell: (row) => row.origin },
              {
                key: "remove",
                header: "除外",
                cell: (row) => (
                  <IconButton
                    label="根拠から除外"
                    accessibleName={`${row.name} を根拠から除外`}
                    onPress={row.remove}
                  >
                    <X size={14} aria-hidden="true" />
                  </IconButton>
                ),
              },
            ]}
          />
        )}
        <div className="flex items-center gap-2">
          <IconButton
            type="submit"
            variant="primary"
            label={
              assertion === undefined
                ? "タイムゾーンを記録"
                : "タイムゾーンを変更"
            }
            isDisabled={view.recording}
            disabledReason={recordingReason}
          >
            <Save size={14} aria-hidden="true" />
          </IconButton>
          {assertion !== undefined && applied !== undefined ? (
            <IconButton
              label="記録を取り消す"
              isDisabled={view.recording || note.trim() === ""}
              disabledReason={
                view.recording
                  ? recordingReason
                  : { title: "取り消す理由が未入力", text: "メモに理由を入力" }
              }
              onPress={() => withdraw(assertion, applied)}
            >
              <Undo2 size={14} aria-hidden="true" />
            </IconButton>
          ) : null}
        </div>
      </fieldset>
    </form>
  );
}

type TimeInterpretationProps = {
  view: TimeInterpretationsView;
  /** 解釈を記録する収集元。選んでいないときは入力欄を出さない。 */
  selectedSource: SourceIdentity | undefined;
  /** 根拠に追加できるレコード。分析者が原文を開いているレコードである。 */
  openedRecordRef: RecordLocator | undefined;
  /** 収集元の内容の識別から file 名を探す表。根拠のレコードを file 名で出す。 */
  sourceFileNamesByContent: ReadonlyMap<string, string>;
  /** 選んだ収集元に取り込みの起動で指定したずれ。指定していないときは `undefined`。 */
  importTimeOffset?: string;
};

/**
 * 収集元の時刻のタイムゾーンを記録し、変更履歴を出す。
 *
 * **原資料の時刻の文字列を書き換えない。** 解釈は Timeline の並びとエッジの推定の時刻の範囲に
 * 使う UTC 時刻だけを変える。取り消すと、その収集元の時刻は UTC に直せなくなる。
 */
export function TimeInterpretation({
  view,
  selectedSource,
  openedRecordRef,
  sourceFileNamesByContent,
  importTimeOffset,
}: TimeInterpretationProps) {
  return (
    <section aria-label="収集元のタイムゾーン">
      <h2 className="section-title">収集元のタイムゾーン</h2>
      {selectedSource === undefined ? (
        <StatusLabel
          status="idle"
          label="収集元の選択なし"
          details="Artifacts で収集元を選択"
        />
      ) : (
        <>
          {view.recordFailure === undefined ||
          view.recordFailure.sourceContentSha256 !==
            selectedSource.contentSha256 ? null : (
            <FetchFailureNotice failure={view.recordFailure.failure} />
          )}
          <FetchStateView
            state={view.state}
            loadingDescription="タイムゾーンを読み込み中"
          >
            {(bySource) => {
              const found = bySource.get(selectedSource.contentSha256);
              if (found?.kind === "conflicted") {
                return (
                  <ConflictedInterpretations
                    source={selectedSource}
                    assertions={found.assertions}
                    fileNames={sourceFileNamesByContent}
                    importTimeOffset={importTimeOffset}
                  />
                );
              }
              const assertion = found?.assertion;
              return (
                <>
                  <CurrentInterpretation
                    source={selectedSource}
                    assertion={assertion}
                    importTimeOffset={importTimeOffset}
                  />
                  <InterpretationForm
                    key={selectedSource.contentSha256}
                    source={selectedSource}
                    assertion={assertion}
                    openedRecordRef={openedRecordRef}
                    view={view}
                    fileNames={sourceFileNamesByContent}
                    recordedHere={
                      view.lastRecordedSource === selectedSource.contentSha256
                        ? view.recordedCount
                        : 0
                    }
                  />
                  {assertion === undefined ? null : (
                    <RevisionHistory
                      assertion={assertion}
                      fileNames={sourceFileNamesByContent}
                    />
                  )}
                </>
              );
            }}
          </FetchStateView>
        </>
      )}
    </section>
  );
}

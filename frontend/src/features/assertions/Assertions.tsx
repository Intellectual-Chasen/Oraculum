import { Save } from "lucide-react";
import { useEffect, useId, useState } from "react";
import type { AssertionDraft } from "@/shared/api/assertions";
import type {
  Assertion,
  AssertionRecordRef,
  AssertionTarget,
} from "@/shared/contracts/assertions";
import { assertionTargetKey } from "@/shared/lib/assertionTargetKey";
import { formatCount } from "@/shared/lib/format";
import { listKey } from "@/shared/lib/listKey";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
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
import { TextField } from "@/shared/ui/TextField";
import type { AssertionsView } from "./useAssertions";

const loadingDescription = "メモの読み込み中";

/** 2 つのレコードの参照が同じレコードを指すかを返す。 */
function sameRecordRef(
  left: AssertionRecordRef | undefined,
  right: AssertionRecordRef | undefined,
): boolean {
  if (left === undefined || right === undefined) {
    return false;
  }
  return (
    left.sourceContentSha256 === right.sourceContentSha256 &&
    left.positionKind === right.positionKind &&
    left.sequenceNumber === right.sequenceNumber &&
    left.lineNumber === right.lineNumber &&
    left.byteOffset === right.byteOffset
  );
}

/**
 * 2 つの対象が同じ対象を指すかを返す。
 *
 * **両辺が値を持つことを先に確かめる。** 値を持たない項目どうしの一致で、対象を欠いた
 * 組が任意の対象へ一致する形にしない。位置の値は収集元が持つものをすべて比べる
 * (`backend/core/assertion.go` の `AssertionRecordRef`)。
 */
export function sameAssertionTarget(
  left: AssertionTarget,
  right: AssertionTarget,
): boolean {
  if (left.kind !== right.kind) {
    return false;
  }
  switch (left.kind) {
    case "node":
      return (
        left.nodeId !== undefined &&
        right.nodeId !== undefined &&
        left.nodeId === right.nodeId
      );
    case "edge":
      return (
        left.edge !== undefined &&
        right.edge !== undefined &&
        left.edge.kind === right.edge.kind &&
        left.edge.sourceNodeId === right.edge.sourceNodeId &&
        left.edge.targetNodeId === right.edge.targetNodeId
      );
    case "record":
      return sameRecordRef(left.record, right.record);
    case "source":
      return (
        left.sourceContentSha256 !== undefined &&
        left.sourceContentSha256 === right.sourceContentSha256
      );
    default: {
      const exhaustive: never = left.kind;
      throw new Error(`unknown assertion target kind: ${String(exhaustive)}`);
    }
  }
}

type NoteFormProps = {
  target: AssertionTarget;
  /** 所見の根拠に添えるレコード。分析者が画面で開いたレコードを渡す。 */
  recordRefs: AssertionRecordRef[];
  recording: boolean;
  /**
   * 最後に成功した記録がこの欄の対象であるときの、全対象を通した記録の成功の通算の
   * 回数。最後に成功した記録が別の対象であるときと、記録がまだ 1 件も成功していない
   * ときは 0 である。
   *
   * 欄 A で 1 回、欄 B で 1 回、欄 A でもう 1 回記録すると、3 回目の後に欄 A が読む値は
   * 3 になる。**値の大きさを読まず、変化したことだけを入力欄を戻す引き金にする。**
   *
   * **この欄の対象への記録が成功したときだけ値が変わる。** 2 つの欄が 1 つの一覧を
   * 読むため、一覧の件数を引き金にすると、片方の記録の成功がもう片方の未保存の入力を
   * 消す。
   */
  recordedCount: number;
  onRecord: (draft: AssertionDraft) => void;
};

/**
 * 分析者のメモを記録する。
 *
 * **入力は自由記述である。** 決まった欄に分けず、分析者が書いた記述をそのまま送る。
 */
function NoteForm({
  target,
  recordRefs,
  recording,
  recordedCount,
  onRecord,
}: NoteFormProps) {
  const fieldId = useId();
  const account = useSignedInAccount();
  const canWrite = useCanWrite();
  const [author, setAuthor] = useState("");
  const [note, setNote] = useState("");

  // この欄の記録が成功したら、メモの入力欄を空へ戻す。同じ内容を 2 回記録する操作と、
  // 前の所見の根拠が次の所見に付く状態を避ける。
  //
  // **分析者の名前は残す。** 同じ分析者が続けて記録する。
  useEffect(() => {
    if (recordedCount === 0) {
      return;
    }
    setNote("");
  }, [recordedCount]);

  const submit = () => {
    // 記録中の送信を退ける。入力欄の Enter でも form は送られ、同じメモを 2 度記録する。
    if (recording) {
      return;
    }
    onRecord({
      target,
      author: account === undefined ? author : undefined,
      note,
      recordRefs,
    });
  };

  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        submit();
      }}
    >
      <fieldset
        disabled={!canWrite}
        className="m-0 flex min-w-0 flex-col items-start gap-2 border-0 p-0"
      >
        <legend className="mb-1 p-0 text-md font-semibold">新規メモ</legend>
        {canWrite ? null : <p className="note">{viewerCannotRecord}</p>}
        <KeyValueList
          className="text-sm text-muted"
          pairs={[
            {
              name: "根拠のレコード",
              value: (
                <>
                  {formatCount(recordRefs.length)}
                  <HelpPopover label="根拠のレコード">
                    表示中の根拠と開いているレコード
                  </HelpPopover>
                </>
              ),
            },
          ]}
        />
        <AuthorField
          id={`${fieldId}-author`}
          label="分析者の名前"
          value={author}
          onChange={setAuthor}
        />
        <TextField
          id={`${fieldId}-note`}
          label="メモ"
          multiline
          value={note}
          onChange={setNote}
          isDisabled={!canWrite}
          className="w-full"
        />
        <IconButton
          type="submit"
          variant="primary"
          label="メモを記録"
          isDisabled={recording}
          disabledReason={{ title: "記録中", text: "記録の完了を待機" }}
        >
          <Save size={14} aria-hidden="true" />
        </IconButton>
      </fieldset>
    </form>
  );
}

/** 対象に付いたメモの表の列。1 件を 1 行にする。 */
const assertionColumns: DataTableColumn<Assertion>[] = [
  {
    key: "note",
    header: "メモ",
    rowHeader: true,
    cell: (assertion) => <RawText text={assertion.basis.note} />,
  },
  {
    key: "author",
    header: "分析者",
    cell: (assertion) => <RawText text={assertion.author} />,
  },
  {
    key: "recordedAt",
    header: "時刻",
    mono: true,
    cell: (assertion) => <RawText text={assertion.recordedAt} />,
  },
  {
    key: "records",
    header: "根拠",
    numeric: true,
    cell: (assertion) => formatCount(assertion.basis.recordRefs.length),
  },
  {
    key: "origin",
    header: "作成",
    cell: (assertion) =>
      assertion.proposalId === undefined ? "分析者" : "AI 提案の採用",
  },
];

type AssertionsProps = {
  /** 所見を付ける対象。ノード 1 つ、関係 1 本、原資料のレコード 1 件のいずれかを指す。 */
  target: AssertionTarget;
  recordRefs: AssertionRecordRef[];
  view: AssertionsView;
};

/**
 * 対象に付いた分析者のメモを表にし、新しいメモを記録する。
 *
 * **メモが 0 件のときに空の表を出さない。** 件数 0 を値の組で出す。
 * **記録の失敗は、記録しようとした対象の欄だけに出す。**
 */
export function Assertions({ target, recordRefs, view }: AssertionsProps) {
  const failure =
    view.recordFailure !== undefined &&
    sameAssertionTarget(view.recordFailure.target, target)
      ? view.recordFailure.failure
      : undefined;
  // この対象への記録が成功した回数。他の対象の成功では増えない。
  const recordedCount =
    view.recordSuccess !== undefined &&
    sameAssertionTarget(view.recordSuccess.target, target)
      ? view.recordSuccess.count
      : 0;
  return (
    <section>
      <h3>メモ</h3>
      {failure === undefined ? null : <FetchFailureNotice failure={failure} />}
      <FetchStateView
        state={view.state}
        loadingDescription={loadingDescription}
      >
        {(loaded) => {
          const key = assertionTargetKey(target);
          const matched =
            key === undefined ? [] : (loaded.byTarget.get(key) ?? []);
          return matched.length === 0 ? (
            <KeyValueList pairs={[{ name: "メモ", value: "0" }]} />
          ) : (
            <DataTable
              label="メモ"
              columns={assertionColumns}
              rows={matched.map((item) => item.assertion)}
              rowKey={(assertion) => listKey([assertion.id])}
            />
          );
        }}
      </FetchStateView>
      <NoteForm
        target={target}
        recordRefs={recordRefs}
        recording={view.recording}
        recordedCount={recordedCount}
        onRecord={view.record}
      />
    </section>
  );
}

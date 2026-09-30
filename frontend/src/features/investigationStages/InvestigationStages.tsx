import { Play, RefreshCw } from "lucide-react";
import { type ReactNode, useCallback, useId, useMemo, useState } from "react";
import type { SourceFileEntry } from "@/shared/contracts/sourceFiles";
import { importCategories } from "@/shared/contracts/sources";
import type {
  LoadedSourceStatus,
  LoadingSource,
  LoadingStage,
  ProcessingStage,
  StageFailure,
  StageState,
  InvestigationStages as Stages,
} from "@/shared/contracts/stages";
import { formatCount } from "@/shared/lib/format";
import {
  diagnosisClassLabels,
  formatKeyLabel,
  importCategoryLabels,
  publicationStateLabels,
} from "@/shared/lib/sourceLabels";
import { Button, type DisabledReason } from "@/shared/ui/Button";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { useCanWrite } from "@/shared/ui/SignedInAccount";
import type { Status } from "@/shared/ui/StatusDot";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { LoadingForm } from "./LoadingForm";
import {
  processingStepLabels,
  stageFailureReasonLabels,
  stageStateLabels,
} from "./labels";
import {
  type LoadingFormRow,
  loadingDraftOf,
  withAddedRows,
} from "./loadingRows";
import {
  SourceBrowser,
  type SourceDragSelection,
  sourceDragType,
} from "./SourceBrowser";
import type { InvestigationStagesView } from "./useInvestigationStages";
import { useSourceUpload } from "./useSourceUpload";

/** 段階を始められる状態か。未開始の段階と、失敗した段階を始められる。 */
function canStart(state: StageState): boolean {
  switch (state) {
    case "not_started":
    case "failed":
      return true;
    case "running":
    case "completed":
      return false;
    default: {
      const exhaustive: never = state;
      throw new Error(`unknown stage state: ${String(exhaustive)}`);
    }
  }
}

/** 処理を始められる状態か。読み込みを終え、処理が未開始か失敗しているときに真である。 */
function canStartProcessing(stages: Stages): boolean {
  return (
    stages.loading.state === "completed" && canStart(stages.processing.state)
  );
}

/** 読み込みを始める入力欄を出す状態か。 */
function canStartLoading(stages: Stages): boolean {
  return stages.acceptsRequestedLoading && canStart(stages.loading.state);
}

const statusAbsent = "読み込みの段階の完了後に表示";
const categoryAbsent = "未集計";

/** 段階の状態を、状態の icon と短いラベルにする。 */
const stageStatuses: Record<StageState, Status> = {
  not_started: "idle",
  running: "running",
  completed: "done",
  failed: "failed",
};

function StageStateLabel({ state }: { state: StageState }) {
  return (
    <KeyValueList
      pairs={[
        {
          name: "状態",
          value: (
            <StatusLabel
              status={stageStatuses[state]}
              label={stageStateLabels[state]}
            />
          ),
        },
      ]}
    />
  );
}

/** 読み込みの段階を終える前の収集元の値。 */
function whenLoaded(
  status: LoadedSourceStatus | undefined,
  value: (status: LoadedSourceStatus) => ReactNode,
): ReactNode {
  return status === undefined ? (
    <MissingValue description={statusAbsent} />
  ) : (
    value(status)
  );
}

/**
 * 収集元ごとの読み込みの表の列。同じ収集元を 2 回読む要求もあり、収集元だけでは行を区別
 * できないため、行は読む順の位置で指す。並びは server が読む順で、応答の間で変わらない。
 */
const sourceColumns: DataTableColumn<LoadingSource>[] = [
  {
    key: "origin",
    header: "収集元",
    rowHeader: true,
    cell: (source) => <RawText text={source.originPath} />,
  },
  {
    key: "format",
    header: "入力形式",
    cell: (source) => <RawText text={formatKeyLabel(source.formatKey)} />,
  },
  {
    key: "readBytes",
    header: "読み込んだ byte",
    numeric: true,
    cell: (source) => formatCount(source.readBytes),
  },
  {
    key: "sizeBytes",
    header: "大きさ",
    numeric: true,
    cell: (source) =>
      source.sizeBytes === undefined ? (
        <MissingValue description="大きさ不明" />
      ) : (
        formatCount(source.sizeBytes)
      ),
  },
  {
    key: "publication",
    header: "公開の状態",
    cell: (source) =>
      whenLoaded(
        source.status,
        (status) => publicationStateLabels[status.publicationState],
      ),
  },
  ...importCategories.map(
    (category): DataTableColumn<LoadingSource> => ({
      key: category,
      header: importCategoryLabels[category],
      numeric: true,
      cell: (source) =>
        whenLoaded(source.status, (status) => {
          const count = status.counts.find(
            (item) => item.category === category,
          );
          return count === undefined ? (
            <MissingValue description={categoryAbsent} />
          ) : (
            formatCount(count.count)
          );
        }),
    }),
  ),
  {
    key: "diagnosis",
    header: "失敗原因の分類",
    cell: (source) =>
      whenLoaded(source.status, (status) =>
        status.diagnosisCounts.length === 0 ? (
          "なし"
        ) : (
          <KeyValueList
            stacked
            pairs={status.diagnosisCounts.map((item) => ({
              name: diagnosisClassLabels[item.diagnosisClass],
              value: formatCount(item.count),
            }))}
          />
        ),
      ),
  },
  {
    key: "failures",
    header: "失敗の件数",
    numeric: true,
    cell: (source) =>
      whenLoaded(source.status, (status) => formatCount(status.failureCount)),
  },
];

function LoadingSourceTable({ loading }: { loading: LoadingStage }) {
  if (loading.sources.length === 0) {
    return <KeyValueList pairs={[{ name: "収集元", value: "0" }]} />;
  }
  return (
    <DataTable
      label="収集元の読み込み"
      columns={sourceColumns}
      rows={loading.sources}
      rowKey={(_source, index) => String(index)}
    />
  );
}

function StageFailureText({ failure }: { failure: StageFailure }) {
  return (
    <div role="alert">
      <KeyValueList
        stacked
        pairs={[
          {
            name: "失敗の理由",
            value: stageFailureReasonLabels[failure.reason],
          },
          {
            name: "失敗した収集元",
            value:
              failure.originPath === undefined ? undefined : (
                <RawText text={failure.originPath} />
              ),
          },
          { name: "詳細", value: "サーバーの運用者の log" },
        ]}
      />
    </div>
  );
}

function ProcessingSteps({ processing }: { processing: ProcessingStage }) {
  return (
    <ol aria-label="処理の手順">
      {processing.steps.map((progress) => (
        <li key={progress.step}>
          {processingStepLabels[progress.step]}:{" "}
          {stageStateLabels[progress.state]}
        </li>
      ))}
    </ol>
  );
}

type InvestigationStagesProps = {
  view: InvestigationStagesView;
};

/**
 * 収集元の読み込みと処理の 2 つの段階の状態を出し、段階を始める操作を受け付ける。
 */
export function InvestigationStages({ view }: InvestigationStagesProps) {
  // 選んだ収集元と指定を本 component が持つ。読み込みを実行する間に入力欄を隠しても、
  // 読み込みが失敗した後に同じ選択から直せる。
  const [rows, setRows] = useState<LoadingFormRow[]>([]);
  const [caseId, setCaseId] = useState("");
  const [thenProcess, setThenProcess] = useState(true);
  // まとめて選ぶ取得の応答を待つ間は、読み込みを始めない。送らなかった file が後から選択に入る。
  const [isBulkAdding, setIsBulkAdding] = useState(false);
  const [dragSelection, setDragSelection] = useState<SourceDragSelection>();
  // **選んだ path の集合は、path の並びが変わったときだけ作り直す。** 収集元の欄の入力のたびに、
  // directory の一覧の表を描き直さない。
  const selectedKey = rows.map((row) => row.originPath).join("\n");
  // biome-ignore lint/correctness/useExhaustiveDependencies: selectedKey は rows の path の並びだけを表す。
  const selectedPaths = useMemo(
    () => new Set(rows.map((row) => row.originPath)),
    [selectedKey],
  );
  const addRows = useCallback(
    (entries: SourceFileEntry[]) =>
      setRows((current) => withAddedRows(current, entries)),
    [],
  );
  const removeRow = useCallback(
    (originPath: string) =>
      setRows((current) =>
        current.filter((row) => row.originPath !== originPath),
      ),
    [],
  );
  const upload = useSourceUpload(addRows, removeRow);
  const { startFailure } = view;
  // 取り直しの応答と段階を始める要求の応答を待つ間は、段階の操作を止める。段階の状態を含む
  // 要求を一度に 1 つだけ送り、応答を送った順に反映する。
  const isAwaitingReply = view.isStarting || view.isRefreshing;
  const canWrite = useCanWrite();
  const viewerNoteId = useId();
  const viewerDescribedBy = canWrite ? undefined : viewerNoteId;
  const reloadLabel = "段階の状態を再読み込み";

  return (
    <section aria-labelledby="investigation-stages-heading" className="stages">
      <h2 id="investigation-stages-heading">調査の段階</h2>
      {canWrite ? null : (
        <p id={viewerNoteId} className="note">
          {viewerCannotStart}
        </p>
      )}
      {view.refreshFailure === undefined ? null : (
        <div className="flex items-center gap-1">
          <FetchFailureNotice failure={view.refreshFailure} />
          {/* 取り直しの予約があるあいだは、予約した取り直しが状態を取り直す。 */}
          {view.isPolling ? null : (
            <IconButton
              label={reloadLabel}
              isDisabled={isAwaitingReply}
              onPress={() => {
                void view.refresh();
              }}
            >
              <RefreshCw size={18} aria-hidden="true" />
            </IconButton>
          )}
        </div>
      )}
      <FetchStateView
        state={view.state}
        loadingDescription="段階の状態の読み込み中"
      >
        {(stages) => (
          <>
            <section
              aria-labelledby="loading-stage-heading"
              className="loading-stage"
            >
              <div className="import-stage-heading">
                <h3 id="loading-stage-heading">収集元の読み込み</h3>
                <StageStateLabel state={stages.loading.state} />
              </div>
              {stages.loading.failure === undefined ? null : (
                <StageFailureText failure={stages.loading.failure} />
              )}
              <LoadingSourceTable loading={stages.loading} />
              {canStartLoading(stages) && canWrite ? (
                <div className="import-workspace">
                  <SourceBrowser
                    selectedPaths={selectedPaths}
                    onAdd={addRows}
                    onRemove={removeRow}
                    onBusyChange={setIsBulkAdding}
                    onDragSelection={setDragSelection}
                  />
                  <LoadingForm
                    formatKeys={stages.formatKeys}
                    rows={rows}
                    onChangeRows={setRows}
                    caseId={caseId}
                    onChangeCaseId={setCaseId}
                    thenProcess={thenProcess}
                    onChangeThenProcess={setThenProcess}
                    onSubmit={(submitted) => {
                      void view.startLoading(
                        submitted.map((row) => loadingDraftOf(row, caseId)),
                        thenProcess,
                      );
                    }}
                    isSubmitting={
                      isAwaitingReply || isBulkAdding || upload.isUploading
                    }
                    onUploadFiles={upload.uploadFiles}
                    uploadNotice={
                      <>
                        {upload.status === "" ? null : (
                          <p role="status">{upload.status}</p>
                        )}
                        {upload.failure === undefined ? null : (
                          <FetchFailureNotice failure={upload.failure} />
                        )}
                        {upload.isUploading ? (
                          <Button variant="secondary" onPress={upload.cancel}>
                            アップロードを中止
                          </Button>
                        ) : null}
                      </>
                    }
                    dragName={dragSelection?.name}
                    onDragOver={(event) => {
                      if (
                        ((dragSelection !== undefined &&
                          event.dataTransfer.types.includes(sourceDragType)) ||
                          event.dataTransfer.types.includes("Files")) &&
                        !isAwaitingReply &&
                        !isBulkAdding &&
                        !upload.isUploading
                      ) {
                        event.preventDefault();
                        event.dataTransfer.dropEffect = "copy";
                      }
                    }}
                    onDrop={(event) => {
                      event.preventDefault();
                      if (
                        !isAwaitingReply &&
                        !isBulkAdding &&
                        !upload.isUploading &&
                        event.dataTransfer.types.includes("Files")
                      ) {
                        upload.uploadTransfer(event.dataTransfer);
                      }
                      if (
                        event.dataTransfer.types.includes(sourceDragType) &&
                        !isAwaitingReply &&
                        !isBulkAdding &&
                        !upload.isUploading
                      ) {
                        dragSelection?.add();
                      }
                      setDragSelection(undefined);
                    }}
                  />
                </div>
              ) : null}
              {!stages.acceptsRequestedLoading &&
              canStart(stages.loading.state) ? (
                <div className="flex items-center gap-1">
                  <KeyValueList
                    pairs={[{ name: "画面からの読み込み", value: "使用不可" }]}
                  />
                  <HelpPopover label="画面からの読み込み">
                    <KeyValueList
                      stacked
                      pairs={[
                        {
                          name: "起動の指定",
                          value: <code>--source-root</code>,
                        },
                        {
                          name: "値",
                          value: "読み込む file の基準の directory",
                        },
                      ]}
                    />
                  </HelpPopover>
                </div>
              ) : null}
              {startFailure?.kind === "loading" ? (
                <FetchFailureNotice failure={startFailure.failure} />
              ) : null}
            </section>
            <section
              aria-labelledby="processing-stage-heading"
              className="processing-stage import-panel"
            >
              <h3 id="processing-stage-heading">処理</h3>
              <StageStateLabel state={stages.processing.state} />
              {stages.processing.failure === undefined ? null : (
                <StageFailureText failure={stages.processing.failure} />
              )}
              <ProcessingSteps processing={stages.processing} />
              <IconButton
                variant="primary"
                label="処理を開始"
                isDisabled={
                  !canWrite || !canStartProcessing(stages) || isAwaitingReply
                }
                disabledReason={processingDisabledReason(
                  canWrite,
                  stages,
                  isAwaitingReply,
                )}
                aria-describedby={viewerDescribedBy}
                onPress={() => {
                  void view.startProcessing();
                }}
              >
                <Play size={18} aria-hidden="true" />
              </IconButton>
              {startFailure?.kind === "processing" ? (
                <FetchFailureNotice failure={startFailure.failure} />
              ) : null}
            </section>
          </>
        )}
      </FetchStateView>
      {view.state.status === "failed" ? (
        <IconButton label={reloadLabel} onPress={view.reload}>
          <RefreshCw size={18} aria-hidden="true" />
        </IconButton>
      ) : null}
    </section>
  );
}

/** 閲覧者の役割で段階の開始を止めた理由の短いラベル。 */
const viewerCannotStart = "閲覧者は開始不可";

/** 処理を始める button を止めた理由。 */
function processingDisabledReason(
  canWrite: boolean,
  stages: Stages,
  isAwaitingReply: boolean,
): DisabledReason {
  if (!canWrite) {
    return { title: viewerCannotStart, text: "編集者の役割が必要" };
  }
  if (isAwaitingReply) {
    return { title: "応答の待機中", text: "応答の受信後に操作可" };
  }
  return stages.loading.state === "completed"
    ? {
        title: `処理: ${stageStateLabels[stages.processing.state]}`,
        text: "未開始か失敗の処理だけ開始可",
      }
    : { title: "読み込み未完了", text: "収集元の読み込みの完了が必要" };
}

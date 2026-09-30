import { File, FolderOpen, FolderPlus, RefreshCw, Search } from "lucide-react";
import { memo, useEffect, useRef, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { fetchSourceFiles } from "@/shared/api/sourceFiles";
import {
  type SourceFileEntry,
  type SourceFileListing,
  type SourceFileUndetectedReason,
  sourceFileUndetectedReasons,
} from "@/shared/contracts/sourceFiles";
import type { FetchFailure } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { formatKeyLabel } from "@/shared/lib/sourceLabels";
import { Button } from "@/shared/ui/Button";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { RawText } from "@/shared/ui/RawText";
import {
  detectedKindLabel,
  otherEntryLabel,
  undetectedReasonLabels,
} from "./labels";
import { useSourceFileListing } from "./useSourceFileListing";

/** 基準の directory そのものを指す path。 */
const basePath = ".";
const baseLabel = "ログフォルダ";

/** 画面内の収集元をドラッグして追加する操作。外部のドロップデータから生成しない。 */
export type SourceDragSelection = { name: string; add: () => void };

/** 画面内の収集元のドラッグを区別する形式。 */
export const sourceDragType = "application/x-oraculum-source";

/** path を directory の名前の並びに分ける。server は path を `/` で区切る。基準の directory は要素数 0 である。 */
function pathSegments(path: string): string[] {
  return path === basePath ? [] : path.split("/").filter((s) => s !== "");
}

/** directory の path の表示。基準の directory は名前で出す。 */
function DirectoryName({ path }: { path: string }) {
  return path === basePath ? baseLabel : <RawText text={path} />;
}

/** 分析者が選べる file か。付属の file、空の file、読めない file は選べない。 */
function isSelectable(entry: SourceFileEntry): boolean {
  if (entry.kind !== "file") {
    return false;
  }
  const reason = entry.undetected?.reason;
  return reason === undefined || reason === "unsupported_format";
}

/**
 * 判定の結果の表示。**候補が無い理由を文字で出す。** 分析者は理由を見て、形式を選ぶか外すかを
 * 決める。
 */
function DetectionText({ entry }: { entry: SourceFileEntry }) {
  switch (entry.kind) {
    case "directory":
      return null;
    case "other":
      return <span className="text-muted">{otherEntryLabel}</span>;
    case "file":
      break;
    default: {
      const exhaustive: never = entry.kind;
      throw new Error(`unknown source file kind: ${String(exhaustive)}`);
    }
  }
  if (entry.undetected !== undefined) {
    const { reason, detectedKind } = entry.undetected;
    const kind =
      detectedKind === undefined ? "" : ` (${detectedKindLabel(detectedKind)})`;
    return (
      <span className="text-muted">
        {undetectedReasonLabels[reason]}
        {kind}
      </span>
    );
  }
  const labels = entry.formatCandidates.map(formatKeyLabel).join(" / ");
  return entry.formatCandidates.length === 1 ? (
    <span>{labels}</span>
  ) : (
    <span>
      候補 {entry.formatCandidates.length} 件: {labels}
    </span>
  );
}

/** まとめて選ぶ操作が選択に含めなかった項目の理由。`other` は file と directory 以外の項目である。 */
type SkippedReason = SourceFileUndetectedReason | "other";

/** 含めなかった理由の表示の順。 */
const skippedReasons: readonly SkippedReason[] = [
  ...sourceFileUndetectedReasons,
  "other",
];

function skippedReasonLabel(reason: SkippedReason): string {
  return reason === "other" ? otherEntryLabel : undetectedReasonLabels[reason];
}

/** 項目をまとめて選ぶときに含めない理由を返す。含める項目には `undefined` を返す。 */
function skippedReasonOf(entry: SourceFileEntry): SkippedReason | undefined {
  if (entry.kind !== "file") {
    return "other";
  }
  return entry.undetected?.reason;
}

/** directory の下の file をまとめて選んだ結果。 */
type BulkAddition =
  | { status: "adding"; path: string }
  | {
      status: "added";
      path: string;
      /** 形式を判定できて、選択に含めた file の件数。 */
      included: number;
      /** 含めなかった項目の、理由ごとの件数。件数 0 の理由は持たない。 */
      skipped: { reason: SkippedReason; count: number }[];
      truncated: boolean;
    }
  | { status: "failed"; path: string; failure: FetchFailure };

type SourceBrowserProps = {
  /** 読み込む収集元に選んだ file の path。 */
  selectedPaths: ReadonlySet<string>;
  /** file を読み込む収集元に足す。 */
  onAdd: (entries: SourceFileEntry[]) => void;
  /** file を読み込む収集元から外す。 */
  onRemove: (originPath: string) => void;
  /** まとめて選ぶ取得の応答を待っているかを知らせる。画面を離れるときは偽を知らせる。 */
  onBusyChange: (isBusy: boolean) => void;
  onDragSelection: (selection: SourceDragSelection | undefined) => void;
};

// 既知の制限: directory の一覧の表は全行を描く, 3000 件の file を持つ directory で測ると、開くまで
// 2.1 秒、1 件の選択に 285 ms かかった。一覧は開くたびに 1 度描き、選んだ収集元の欄の入力では
// 描き直さない (memo),
// 1 つの directory の file が上限 (5000 件) に近い収集を扱うときに、見えている行だけを描く表を見直す

/**
 * server の基準の directory の下を辿り、読み込む file を選ぶ。file ごとに server が判定した
 * 入力形式を出す。directory は、下の file のうち形式を判定できた file をまとめて選べる。
 * 選択に含めなかった項目は、理由ごとの件数で出す。
 */
export const SourceBrowser = memo(function SourceBrowser({
  selectedPaths,
  onAdd,
  onRemove,
  onBusyChange,
  onDragSelection,
}: SourceBrowserProps) {
  const [path, setPath] = useState(basePath);
  const [search, setSearch] = useState("");
  const [bulk, setBulk] = useState<BulkAddition | undefined>(undefined);
  const listing = useSourceFileListing(path);
  const segments = pathSegments(path);
  const isAdding = bulk?.status === "adding";
  useEffect(() => {
    onBusyChange(isAdding);
  }, [isAdding, onBusyChange]);
  // まとめて選ぶ取得の打ち切り。画面を離れた後に届いた応答を選択に足さない。
  const bulkController = useRef<AbortController | undefined>(undefined);
  useEffect(
    () => () => {
      bulkController.current?.abort();
      onBusyChange(false);
    },
    [onBusyChange],
  );

  const addDirectory = async (directory: string) => {
    const controller = new AbortController();
    bulkController.current = controller;
    setBulk({ status: "adding", path: directory });
    let result: Awaited<ReturnType<typeof fetchSourceFiles>>;
    try {
      result = await fetchSourceFiles(directory, {
        recursive: true,
        signal: controller.signal,
      });
    } catch {
      result = {
        ok: false,
        failure: buildFetchFailure("unexpected", "directory の file の取得"),
      };
    }
    if (controller.signal.aborted) {
      return;
    }
    if (!result.ok) {
      setBulk({ status: "failed", path: directory, failure: result.failure });
      return;
    }
    const included: SourceFileEntry[] = [];
    const counts = new Map<SkippedReason, number>();
    for (const entry of result.value.entries) {
      const reason = skippedReasonOf(entry);
      if (reason === undefined) {
        included.push(entry);
      } else {
        counts.set(reason, (counts.get(reason) ?? 0) + 1);
      }
    }
    onAdd(included);
    setBulk({
      status: "added",
      path: directory,
      included: included.length,
      skipped: skippedReasons.flatMap((reason) => {
        const count = counts.get(reason) ?? 0;
        return count === 0 ? [] : [{ reason, count }];
      }),
      truncated: result.value.truncated,
    });
  };

  const columns: DataTableColumn<SourceFileEntry>[] = [
    {
      key: "select",
      header: "選択",
      className: "short-cell",
      cell: (entry) =>
        entry.kind === "directory" ? (
          <IconButton
            label="フォルダ内のログを追加"
            accessibleName={`${entry.name} の下の、形式を判定できた file をすべて選ぶ`}
            isDisabled={isAdding}
            onPress={() => {
              void addDirectory(entry.originPath);
            }}
          >
            <FolderPlus size={18} aria-hidden="true" />
          </IconButton>
        ) : (
          <label className="source-choice">
            <input
              type="checkbox"
              aria-label={`${entry.name} を選ぶ`}
              // 選べない項目の表示用の path は、選んだ file の path と比べない。
              checked={
                entry.kind === "file" && selectedPaths.has(entry.originPath)
              }
              disabled={!isSelectable(entry)}
              onChange={(event) => {
                if (event.target.checked) {
                  onAdd([entry]);
                } else {
                  onRemove(entry.originPath);
                }
              }}
            />
          </label>
        ),
    },
    {
      key: "name",
      header: "名前",
      rowHeader: true,
      cell: (entry) =>
        entry.kind === "directory" ? (
          <Button
            size="sm"
            variant="ghost"
            aria-label={`${entry.name} を開く`}
            onPress={() => {
              setPath(entry.originPath);
              setSearch("");
            }}
          >
            <FolderOpen size={18} aria-hidden="true" />
            <RawText text={entry.name} />
          </Button>
        ) : (
          <span className="source-file-name">
            <File size={18} aria-hidden="true" />
            <RawText text={entry.name} />
          </span>
        ),
    },
    {
      key: "size",
      header: "大きさ (byte)",
      numeric: true,
      cell: (entry) =>
        entry.sizeBytes === undefined ? null : formatCount(entry.sizeBytes),
    },
    {
      key: "format",
      header: "判定した形式",
      cell: (entry) => <DetectionText entry={entry} />,
    },
  ];

  return (
    <section
      aria-labelledby="source-browser-heading"
      className="source-browser import-panel"
    >
      <div className="import-panel-heading">
        <h4 id="source-browser-heading">収集元の選択</h4>
        <HelpPopover label="収集元の選択">
          サーバーのログフォルダからファイルを選択。チェックボックスかドラッグで追加。フォルダ名で移動。フォルダの追加ボタンは形式を判定できたファイルをまとめて追加
        </HelpPopover>
      </div>
      <nav aria-label="開いている directory" className="source-breadcrumbs">
        <Button
          size="sm"
          variant="ghost"
          isDisabled={segments.length === 0}
          onPress={() => {
            setPath(basePath);
            setSearch("");
          }}
        >
          {baseLabel}
        </Button>
        {segments.map((segment, index) => {
          const target = segments.slice(0, index + 1).join("/");
          const isCurrent = index === segments.length - 1;
          return (
            <Button
              key={target}
              size="sm"
              variant="ghost"
              isDisabled={isCurrent}
              aria-current={isCurrent ? "location" : undefined}
              onPress={() => {
                setPath(target);
                setSearch("");
              }}
            >
              / <RawText text={segment} />
            </Button>
          );
        })}
      </nav>
      <div className="source-browser-tools">
        <label className="source-search">
          <Search size={18} aria-hidden="true" />
          <input
            type="search"
            aria-label="ファイル名で絞り込み"
            placeholder="ファイル名で絞り込み"
            value={search}
            onChange={(event) => setSearch(event.target.value)}
          />
        </label>
        <Button
          size="sm"
          isDisabled={isAdding || listing.state.status !== "loaded"}
          onPress={() => {
            void addDirectory(path);
          }}
        >
          <FolderPlus size={18} aria-hidden="true" />
          フォルダ内をすべて追加
        </Button>
        <IconButton label="一覧を再読み込み" onPress={listing.reload}>
          <RefreshCw size={18} aria-hidden="true" />
        </IconButton>
      </div>
      <BulkAdditionText bulk={bulk} />
      <FetchStateView
        state={listing.state}
        loadingDescription="directory の一覧の読み込み中"
      >
        {(value: SourceFileListing) =>
          value.entries.length === 0 ? (
            <p role="status">項目なし</p>
          ) : (
            <>
              {value.truncated ? (
                <p role="status">
                  項目の数が上限に達したため、上限を超えた項目を表示していない
                </p>
              ) : null}
              <div className="source-list-scroll">
                <DataTable
                  label="directory の項目"
                  columns={columns}
                  rows={value.entries.filter((entry) =>
                    entry.name
                      .toLocaleLowerCase()
                      .includes(search.toLocaleLowerCase()),
                  )}
                  rowProps={(entry) => ({
                    draggable:
                      !isAdding &&
                      (entry.kind === "directory" || isSelectable(entry)),
                    "aria-selected":
                      entry.kind === "file" &&
                      selectedPaths.has(entry.originPath),
                    onDragStart: (event) => {
                      if (
                        isAdding ||
                        (entry.kind !== "directory" && !isSelectable(entry))
                      ) {
                        event.preventDefault();
                        return;
                      }
                      event.dataTransfer.setData(
                        sourceDragType,
                        entry.originPath,
                      );
                      event.dataTransfer.effectAllowed = "copy";
                      onDragSelection({
                        name: entry.name,
                        add: () => {
                          if (entry.kind === "directory") {
                            void addDirectory(entry.originPath);
                          } else {
                            onAdd([entry]);
                          }
                        },
                      });
                    },
                    onDragEnd: () => onDragSelection(undefined),
                  })}
                  // 選べない項目の表示用の path は、ほかの項目の path と同じ文字列になりうる。一覧は
                  // 取り直すたびに差し替わるため、並びの位置で区別する。
                  rowKey={(entry, index) => `${index}:${entry.originPath}`}
                />
              </div>
              {value.entries.some((entry) =>
                entry.name
                  .toLocaleLowerCase()
                  .includes(search.toLocaleLowerCase()),
              ) ? null : (
                <p role="status" className="import-empty">
                  一致するファイルなし
                </p>
              )}
            </>
          )
        }
      </FetchStateView>
    </section>
  );
});

/** directory の下の file をまとめて選んだ結果の表示。 */
function BulkAdditionText({ bulk }: { bulk: BulkAddition | undefined }) {
  if (bulk === undefined) {
    return null;
  }
  switch (bulk.status) {
    case "adding":
      return (
        <p role="status">
          <DirectoryName path={bulk.path} /> の下の file の判定中
        </p>
      );
    case "failed":
      return <FetchFailureNotice failure={bulk.failure} />;
    case "added":
      return (
        <p role="status">
          <DirectoryName path={bulk.path} /> の下の、形式を判定できた{" "}
          {formatCount(bulk.included)} 件を選択に含めた。
          {bulk.skipped.length === 0
            ? ""
            : ` 含めていない項目: ${bulk.skipped
                .map(
                  ({ reason, count }) =>
                    `${skippedReasonLabel(reason)} ${formatCount(count)} 件`,
                )
                .join("、")}。`}
          {bulk.truncated
            ? " 一覧の上限を超えた項目は、確かめておらず、選択に含めていない。"
            : ""}
        </p>
      );
    default: {
      const exhaustive: never = bulk;
      throw new Error(`unknown bulk addition: ${JSON.stringify(exhaustive)}`);
    }
  }
}

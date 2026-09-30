import { useEffect, useRef, useState } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import { uploadSourceFile } from "@/shared/api/sourceFiles";
import type { SourceFileEntry } from "@/shared/contracts/sourceFiles";
import type { FetchFailure } from "@/shared/lib/fetchState";

type UploadFile = { file: File; path: string };

/** directory reader は一覧を分割して返すので、空の組まで読み進める。 */
async function collectEntry(
  entry: FileSystemEntry,
  files: UploadFile[],
  signal: AbortSignal,
): Promise<void> {
  signal.throwIfAborted();
  if (entry.isFile) {
    const file = await new Promise<File>((resolve, reject) =>
      (entry as FileSystemFileEntry).file(resolve, reject),
    );
    files.push({ file, path: entry.fullPath.replace(/^\//, "") });
    return;
  }
  if (entry.isDirectory) {
    const reader = (entry as FileSystemDirectoryEntry).createReader();
    for (;;) {
      const children = await new Promise<FileSystemEntry[]>((resolve, reject) =>
        reader.readEntries(resolve, reject),
      );
      if (children.length === 0) return;
      for (const child of children) await collectEntry(child, files, signal);
    }
  }
}

/** アップロードは順に送り、完了した資料だけを追加する。中断・失敗後も追加済みの資料を保つ。 */
export function useSourceUpload(
  onAdd: (entries: SourceFileEntry[]) => void,
  onRemove: (path: string) => void,
) {
  const [isUploading, setIsUploading] = useState(false);
  const [status, setStatus] = useState("");
  const [failure, setFailure] = useState<FetchFailure>();
  const controllerRef = useRef<AbortController | undefined>(undefined);
  useEffect(() => () => controllerRef.current?.abort(), []);
  const upload = async (
    collect: (signal: AbortSignal) => Promise<UploadFile[]>,
  ) => {
    if (controllerRef.current) return;
    const controller = new AbortController();
    controllerRef.current = controller;
    setIsUploading(true);
    setFailure(undefined);
    setStatus("ファイルを確認中");
    try {
      const files = await collect(controller.signal);
      if (files.length === 0) {
        setStatus("ファイルがありません");
        return;
      }
      const batch = crypto.randomUUID();
      const added = new Set<string>();
      for (let index = 0; index < files.length; index++) {
        if (controller.signal.aborted) return;
        const { file, path } = files[index];
        setStatus(`アップロード中: ${index + 1} / ${files.length}`);
        const result = await uploadSourceFile(
          file,
          path,
          controller.signal,
          batch,
        );
        if (controller.signal.aborted) return;
        if (!result.ok) {
          setStatus(`中断: ${index} / ${files.length}件を送信済み`);
          setFailure(result.failure);
          return;
        }
        const selected = result.value.entries.filter(
          (entry) =>
            entry.kind === "file" &&
            (entry.undetected === undefined ||
              entry.undetected.reason === "unsupported_format"),
        );
        onAdd(selected);
        for (const entry of selected) added.add(entry.originPath);
        for (const entry of result.value.entries) {
          if (!selected.includes(entry)) {
            onRemove(entry.originPath);
            added.delete(entry.originPath);
          }
        }
      }
      setStatus(
        `追加: ${added.size}件 / 選択対象外: ${files.length - added.size}件`,
      );
    } catch (cause) {
      if (!controller.signal.aborted) {
        setStatus(
          cause instanceof Error
            ? cause.message
            : "ファイルを取得できませんでした",
        );
        setFailure(buildFetchFailure("unexpected", "ログのアップロード"));
      }
    } finally {
      if (controllerRef.current === controller) {
        controllerRef.current = undefined;
        setIsUploading(false);
      }
    }
  };
  return {
    isUploading,
    status,
    failure,
    cancel: () => {
      controllerRef.current?.abort();
      setStatus("中止しました。追加済みの資料は保持しています");
    },
    uploadFiles: (files: File[]) =>
      void upload(async () =>
        files.map((file) => ({
          file,
          path: file.webkitRelativePath || file.name,
        })),
      ),
    uploadTransfer: (transfer: DataTransfer) => {
      // DataTransfer は drop handler を抜けると読めないので、参照を同期的に取り出す。
      const entries = Array.from(transfer.items ?? [])
        .map((item) => item.webkitGetAsEntry?.())
        .filter((entry): entry is FileSystemEntry => entry != null);
      const files = Array.from(transfer.files);
      void upload(async (signal) => {
        if (entries.length === 0)
          return files.map((file) => ({ file, path: file.name }));
        const collected: UploadFile[] = [];
        for (const entry of entries)
          await collectEntry(entry, collected, signal);
        return collected;
      });
    },
  };
}

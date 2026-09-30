// @vitest-environment jsdom
import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import * as api from "@/shared/api/sourceFiles";
import { decodeSourceFileListing } from "@/shared/contracts/sourceFiles";
import { proxyListingJson } from "@/testdata/stages/sourceFilesResponse";
import { useSourceUpload } from "./useSourceUpload";

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
});
const listing = () => decodeSourceFileListing(proxyListingJson(), "response");

test("フォルダのドロップは分割された一覧を最後まで読み、同じ識別子と相対pathで送る", async () => {
  const added = vi.fn();
  const removed = vi.fn();
  const upload = vi
    .spyOn(api, "uploadSourceFile")
    .mockResolvedValue({ ok: true, value: listing() });
  const { result } = renderHook(() => useSourceUpload(added, removed));
  const entry = (name: string) => ({
    isFile: true,
    isDirectory: false,
    fullPath: `/folder/${name}`,
    file: (done: (file: File) => void) => done(new File(["log"], name)),
  });
  const chunks = [[entry("one.log")], [entry("two.log")], []];
  const directory = {
    isDirectory: true,
    isFile: false,
    createReader: () => ({
      readEntries: (done: (files: unknown[]) => void) =>
        done(chunks.shift() ?? []),
    }),
  };
  act(() =>
    result.current.uploadTransfer({
      items: [{ webkitGetAsEntry: () => directory }],
      files: [],
    } as unknown as DataTransfer),
  );
  await waitFor(() => expect(result.current.isUploading).toBe(false));
  expect(upload).toHaveBeenCalledTimes(2);
  expect(upload.mock.calls.map((call) => call[1])).toEqual([
    "folder/one.log",
    "folder/two.log",
  ]);
  expect(upload.mock.calls[0][3]).toBe(upload.mock.calls[1][3]);
  expect(added).toHaveBeenCalledTimes(2);
});

test("通信失敗は完了した資料を保持し、後続の資料を送らない", async () => {
  const added = vi.fn();
  const removed = vi.fn();
  const upload = vi
    .spyOn(api, "uploadSourceFile")
    .mockResolvedValueOnce({ ok: true, value: listing() })
    .mockResolvedValueOnce({
      ok: false,
      failure: buildFetchFailure("network", "ログのアップロード"),
    });
  const { result } = renderHook(() => useSourceUpload(added, removed));
  act(() =>
    result.current.uploadFiles(
      ["one.log", "two.log", "three.log"].map(
        (name) => new File(["log"], name),
      ),
    ),
  );
  await waitFor(() => expect(result.current.failure?.kind).toBe("network"));
  expect(upload).toHaveBeenCalledTimes(2);
  expect(added).toHaveBeenCalledTimes(1);
  expect(result.current.status).toContain("1 / 3件を送信済み");
});

test("中止は進行中の要求を取り消し、後続の資料を送らない", async () => {
  const upload = vi.spyOn(api, "uploadSourceFile").mockImplementation(
    (_file, _path, signal) =>
      new Promise((resolve) =>
        signal?.addEventListener("abort", () =>
          resolve({
            ok: false,
            failure: buildFetchFailure("network", "ログのアップロード"),
          }),
        ),
      ),
  );
  const { result } = renderHook(() => useSourceUpload(vi.fn(), vi.fn()));
  act(() =>
    result.current.uploadFiles([
      new File(["log"], "one.log"),
      new File(["log"], "two.log"),
    ]),
  );
  await waitFor(() => expect(upload).toHaveBeenCalledTimes(1));
  act(() => result.current.cancel());
  await waitFor(() => expect(result.current.isUploading).toBe(false));
  expect(upload.mock.calls[0][2]?.aborted).toBe(true);
  expect(upload).toHaveBeenCalledTimes(1);
  expect(result.current.failure).toBeUndefined();
});

test("空のフォルダは資料が無いことを知らせて要求を送らない", async () => {
  const upload = vi.spyOn(api, "uploadSourceFile");
  const { result } = renderHook(() => useSourceUpload(vi.fn(), vi.fn()));
  act(() => result.current.uploadFiles([]));
  await waitFor(() =>
    expect(result.current.status).toBe("ファイルがありません"),
  );
  expect(upload).not.toHaveBeenCalled();
});

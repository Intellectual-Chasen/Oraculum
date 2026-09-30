import { proxyLogPath } from "./stagesResponse";

/** 基準の directory の一覧の要求の path。 */
export const sourceFilesPath = "/api/v0/stages/source-files";

/** 基準の directory の下の directory `path` の一覧の要求の method と path。 */
export function sourceFilesRequest(path: string, recursive = false): string {
  const query = new URLSearchParams({ path });
  if (recursive) {
    query.set("recursive", "true");
  }
  return `GET ${sourceFilesPath}?${query.toString()}`;
}

/** 形式を判定できない file の path。 */
export const unknownFilePath = "logs/proxy/notes.bin";

/** 候補を 2 つ持つ file の path。 */
export const ambiguousFilePath = "logs/proxy/other.log";

/** 基準の directory の一覧。directory を 1 つだけ持つ。 */
export function baseListingJson() {
  return {
    path: ".",
    recursive: false,
    entries: [{ originPath: "logs", name: "logs", kind: "directory" }],
    truncated: false,
  };
}

/** `logs` の一覧。directory を 1 つだけ持つ。 */
export function logsListingJson() {
  return {
    path: "logs",
    recursive: false,
    entries: [{ originPath: "logs/proxy", name: "proxy", kind: "directory" }],
    truncated: false,
  };
}

/** `logs/proxy` の一覧。判定できた file、候補が 2 つの file、判定できない file、付属の file を持つ。 */
export function proxyListingJson() {
  return {
    path: "logs/proxy",
    recursive: false,
    entries: [
      {
        originPath: proxyLogPath,
        name: "access.log",
        kind: "file",
        sizeBytes: 8192,
        formatCandidates: ["squid_combined"],
      },
      {
        originPath: ambiguousFilePath,
        name: "other.log",
        kind: "file",
        sizeBytes: 64,
        formatCandidates: ["infotrace_mark_ii", "squid_combined"],
      },
      {
        originPath: unknownFilePath,
        name: "notes.bin",
        kind: "file",
        sizeBytes: 16,
        undetected: { reason: "unsupported_format", detectedKind: "zip" },
      },
      {
        originPath: "logs/proxy/SYSTEM.LOG1",
        name: "SYSTEM.LOG1",
        kind: "file",
        sizeBytes: 4,
        undetected: { reason: "companion_file" },
      },
    ],
    truncated: false,
  };
}

/**
 * `logs/proxy` の下の再帰の一覧。`logs/proxy` の file に加えて、空の file、読めない file、選べない
 * 項目を持つ。
 */
export function proxyRecursiveListingJson() {
  const listing = proxyListingJson();
  return {
    ...listing,
    recursive: true,
    entries: [
      ...listing.entries,
      {
        originPath: "logs/proxy/sub/empty.log",
        name: "empty.log",
        kind: "file",
        sizeBytes: 0,
        undetected: { reason: "empty_file" },
      },
      {
        originPath: "logs/proxy/sub/locked.log",
        name: "locked.log",
        kind: "file",
        sizeBytes: 32,
        undetected: { reason: "unreadable" },
      },
      { originPath: "logs/proxy/linked", name: "linked", kind: "other" },
    ],
  };
}

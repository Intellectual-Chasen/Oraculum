import { expect, test } from "vitest";
import { proxyListingJson } from "@/testdata/stages/sourceFilesResponse";
import { DecodeFailure } from "./decoding";
import { decodeSourceFileListing } from "./sourceFiles";

/** 一覧の JSON の最初の項目を `change` で書き換えた一覧を返す。 */
function withFirstEntry(
  change: (entry: Record<string, unknown>) => Record<string, unknown>,
  listing: Record<string, unknown> = proxyListingJson(),
) {
  // 境界の検証を試すため、型の付かない JSON として項目を書き換える。
  const entries = listing.entries as Record<string, unknown>[];
  const [first, ...rest] = entries;
  if (first === undefined) {
    throw new Error("the listing fixture has no entry");
  }
  return { ...listing, entries: [change(first), ...rest] };
}

test("一覧の項目を読み、候補を持たない項目の候補を要素数 0 にする", () => {
  const listing = decodeSourceFileListing(proxyListingJson(), "listing");
  expect(listing.entries.map((entry) => entry.formatCandidates)).toEqual([
    ["squid_combined"],
    ["infotrace_mark_ii", "squid_combined"],
    [],
    [],
  ]);
  expect(listing.entries[2]?.undetected).toEqual({
    reason: "unsupported_format",
    detectedKind: "zip",
  });
});

test.each([
  [
    "候補と候補が無い理由の両方を持つ file",
    withFirstEntry((entry) => ({
      ...entry,
      undetected: { reason: "empty_file" },
    })),
  ],
  [
    "候補も候補が無い理由も持たない file",
    withFirstEntry((entry) => ({ ...entry, formatCandidates: undefined })),
  ],
  [
    "大きさを持たない file",
    withFirstEntry((entry) => ({ ...entry, sizeBytes: undefined })),
  ],
  [
    "file の項目を持つ directory",
    withFirstEntry((entry) => ({ ...entry, kind: "directory" })),
  ],
  [
    "大きさだけを持つ選べない項目",
    withFirstEntry((entry) => ({
      originPath: entry.originPath,
      name: entry.name,
      kind: "other",
      sizeBytes: 1,
    })),
  ],
  [
    "空の path を持つ項目",
    withFirstEntry((entry) => ({ ...entry, originPath: "" })),
  ],
  [
    "directory を持つ再帰の一覧",
    withFirstEntry(
      (entry) => ({
        originPath: entry.originPath,
        name: entry.name,
        kind: "directory",
      }),
      { ...proxyListingJson(), recursive: true },
    ),
  ],
])("%s を退ける", (_name, input) => {
  expect(() => decodeSourceFileListing(input, "listing")).toThrow(
    DecodeFailure,
  );
});

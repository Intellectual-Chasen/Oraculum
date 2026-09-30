import { decodeRecordLocator, type RecordLocator } from "./common";
import {
  DecodeFailure,
  type Decoder,
  optionalBoolean,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireBoolean,
  requireCount,
  requireMember,
  requireString,
} from "./decoding";

export const urlFragmentEncodings = ["base64url", "base64"] as const;
export type UrlFragmentEncoding = (typeof urlFragmentEncodings)[number];

export const urlFragmentDecodeFailures = [
  "missing_numbers",
  "conflicting_duplicates",
  "alphabet_undetermined",
  "invalid_encoding",
] as const;
export type UrlFragmentDecodeFailure =
  (typeof urlFragmentDecodeFailures)[number];

export const zipIntegrities = [
  "passed",
  "failed",
  "not_checked",
  "unsupported",
] as const;
export type ZipIntegrity = (typeof zipIntegrities)[number];

/**
 * URL の断片をつないだ結果 (`GET /api/v0/edges/{id}/url-fragments`) の応答。
 * 定義元は `backend/core/url_fragment_join.go` の `UrlFragmentJoin` である。
 */
export type UrlFragmentJoin = {
  edgeId: string;
  unnumberedRecordCount: number;
  segments: UrlFragmentSegment[];
};

export type UrlFragmentSegment = {
  fragmentCount: number;
  lastNumber: number;
  duplicateCount: number;
  conflictingDuplicateCount: number;
  missingNumberCount: number;
  fragments: UrlFragment[];
  encoding?: UrlFragmentEncoding;
  decoded?: DecodedBytes;
  decodeFailure?: UrlFragmentDecodeFailure;
};

export type UrlFragment = {
  number: number;
  adopted: boolean;
  /** 行が記録した HTTP の状態の文字列。状態の欄を持たない行では出ない。 */
  httpStatusCode?: string;
  /** 原資料の行が途中で切れ、URL の文字列が後ろを持たないか。 */
  truncated?: boolean;
  recordRef: RecordLocator;
};

export type DecodedBytes = {
  byteCount: number;
  sha256: string;
  leadingBytesHex: string;
  contentType: string;
  zip?: ZipListing;
};

export type ZipListing = {
  readable: boolean;
  integrity?: ZipIntegrity;
  entryCount: number;
  entries: ZipEntryName[];
};

export type ZipEntryName = { name: string; nameHex?: string };

const decodeUrlFragment: Decoder<UrlFragment> = (input, path) => {
  const source = readObject(input, path);
  return {
    number: requireCount(source, "number", path),
    adopted: requireBoolean(source, "adopted", path),
    httpStatusCode: optionalString(source, "httpStatusCode", path),
    truncated: optionalBoolean(source, "truncated", path),
    recordRef: requireMember(source, "recordRef", path, decodeRecordLocator),
  };
};

const decodeZipEntryName: Decoder<ZipEntryName> = (input, path) => {
  const source = readObject(input, path);
  const nameHex = optionalString(source, "nameHex", path);
  return {
    name: requireString(source, "name", path),
    ...(nameHex === undefined ? {} : { nameHex }),
  };
};

const decodeZipListing: Decoder<ZipListing> = (input, path) => {
  const source = readObject(input, path);
  const integrity = optionalEnum(source, "integrity", path, zipIntegrities);
  return {
    readable: requireBoolean(source, "readable", path),
    ...(integrity === undefined ? {} : { integrity }),
    entryCount: requireCount(source, "entryCount", path),
    entries: requireArray(source, "entries", path, decodeZipEntryName),
  };
};

const decodeDecodedBytes: Decoder<DecodedBytes> = (input, path) => {
  const source = readObject(input, path);
  const zip = optionalMember(source, "zip", path, decodeZipListing);
  return {
    byteCount: requireCount(source, "byteCount", path),
    sha256: requireString(source, "sha256", path),
    leadingBytesHex: requireString(source, "leadingBytesHex", path),
    contentType: requireString(source, "contentType", path),
    ...(zip === undefined ? {} : { zip }),
  };
};

const decodeUrlFragmentSegment: Decoder<UrlFragmentSegment> = (input, path) => {
  const source = readObject(input, path);
  const encoding = optionalEnum(source, "encoding", path, urlFragmentEncodings);
  const decoded = optionalMember(source, "decoded", path, decodeDecodedBytes);
  const decodeFailure = optionalEnum(
    source,
    "decodeFailure",
    path,
    urlFragmentDecodeFailures,
  );
  if ((decoded === undefined) === (decodeFailure === undefined)) {
    throw new DecodeFailure(
      `${path}.decoded`,
      "expected exactly one of decoded and decodeFailure",
    );
  }
  return {
    fragmentCount: requireCount(source, "fragmentCount", path),
    lastNumber: requireCount(source, "lastNumber", path),
    duplicateCount: requireCount(source, "duplicateCount", path),
    conflictingDuplicateCount: requireCount(
      source,
      "conflictingDuplicateCount",
      path,
    ),
    missingNumberCount: requireCount(source, "missingNumberCount", path),
    fragments: requireArray(source, "fragments", path, decodeUrlFragment),
    ...(encoding === undefined ? {} : { encoding }),
    ...(decoded === undefined ? {} : { decoded }),
    ...(decodeFailure === undefined ? {} : { decodeFailure }),
  };
};

/** URL の断片をつないだ結果の応答を検証する。 */
export const decodeUrlFragmentJoin: Decoder<UrlFragmentJoin> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    edgeId: requireString(source, "edgeId", path),
    unnumberedRecordCount: requireCount(source, "unnumberedRecordCount", path),
    segments: requireArray(source, "segments", path, decodeUrlFragmentSegment),
  };
};

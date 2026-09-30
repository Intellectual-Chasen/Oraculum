import type { AssertionRecordRef } from "./assertions";
import { decodeAssertionRecordRef } from "./assertions";
import type { TimeRange } from "./common";
import { decodeTimeRange } from "./common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalArray,
  optionalString,
  readObject,
  requireArray,
  requireCount,
  requireEnum,
  requireString,
} from "./decoding";

/**
 * 割当が何から出たか。定義元は `backend/core/assignment.go` の
 * `TerminalAssignmentOrigin` である。
 */
export const terminalAssignmentOrigins = [
  "observed_in_source",
  "import_specified",
  "analyst_supplied",
] as const;
export type TerminalAssignmentOrigin =
  (typeof terminalAssignmentOrigins)[number];

/**
 * 接続元 IP から端末への割当 1 件。定義元は `backend/core/assignment.go` の
 * `TerminalAssignment` と `Validate` である。
 *
 * 接続元 IP、端末の識別子、端末の表示名は、それぞれ省略できる。1 件の割当は 3 項目の
 * うち 1 つ以上を持つ。
 */
export type TerminalAssignment = {
  clientIp?: string;
  /** 端末の外部識別子。持たない割当は `appliesToSourceId` の収集元を記録した端末を指す。 */
  terminalId?: string;
  terminalHostname?: string;
  /**
   * 端末が名乗るホスト名 (短い名前と FQDN)。分析者が記録した割当だけが持つ。
   * レコードが名乗ったホスト名と、引数が指すホスト名をこの端末と比べる。
   */
  terminalHostnames?: string[];
  /** 適用期間を読み取った収集元。 */
  sourceId: string;
  sourceContentSha256: string;
  assignmentValidRange: TimeRange;
  origin: TerminalAssignmentOrigin;
  /** 分析者が割当を導いた筋道。画面から記録した割当が持つ。 */
  derivation?: string;
  /** 分析者が根拠に挙げたレコード。画面から記録した割当が持つ。 */
  basisRecordRefs?: AssertionRecordRef[];
  /** 割当を記録した分析者。画面から記録した割当が持つ。 */
  author?: string;
  /** レコードの全体がこの端末のものである収集元。 */
  appliesToSourceId?: string;
};

/**
 * `TerminalAssignment` を検証する。
 *
 * 接続元 IP、端末の識別子、端末の表示名のいずれも持たない割当を退ける
 * (`backend/core/assignment.go` の `Validate`)。
 */
export const decodeTerminalAssignment: Decoder<TerminalAssignment> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const clientIp = optionalString(source, "clientIp", path);
  const terminalId = optionalString(source, "terminalId", path);
  const terminalHostname = optionalString(source, "terminalHostname", path);
  if (
    clientIp === undefined &&
    terminalId === undefined &&
    terminalHostname === undefined
  ) {
    throw new DecodeFailure(
      path,
      "expected at least one of clientIp, terminalId and terminalHostname",
    );
  }
  return {
    clientIp,
    terminalId,
    terminalHostname,
    terminalHostnames: optionalArray(
      source,
      "terminalHostnames",
      path,
      decodeString,
    ),
    sourceId: requireString(source, "sourceId", path),
    sourceContentSha256: requireString(source, "sourceContentSha256", path),
    assignmentValidRange: decodeTimeRange(
      source.assignmentValidRange,
      `${path}.assignmentValidRange`,
    ),
    origin: requireEnum(source, "origin", path, terminalAssignmentOrigins),
    derivation: optionalString(source, "derivation", path),
    basisRecordRefs: optionalArray(
      source,
      "basisRecordRefs",
      path,
      decodeAssertionRecordRef,
    ),
    author: optionalString(source, "author", path),
    appliesToSourceId: optionalString(source, "appliesToSourceId", path),
  };
};

/** 割当の一覧の応答。 */
export type TerminalAssignmentsResponse = {
  assignments: TerminalAssignment[];
  assignmentCount: number;
};

/** `TerminalAssignmentsResponse` を検証する。 */
export const decodeTerminalAssignmentsResponse: Decoder<
  TerminalAssignmentsResponse
> = (input, path) => {
  const source = readObject(input, path);
  return {
    assignments: requireArray(
      source,
      "assignments",
      path,
      decodeTerminalAssignment,
    ),
    assignmentCount: requireCount(source, "assignmentCount", path),
  };
};

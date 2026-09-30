import { type CaseId, decodeCaseId } from "./cases";
import {
  decodeRequestedTime,
  decodeTimestamp,
  type RequestedTime,
  type Timestamp,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
  optionalArray,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  rejectMember,
  requireArray,
  requireCount,
  requireEnum,
  requireString,
} from "./decoding";
import {
  decodeEdgeKind,
  decodeGraphEvidence,
  decodeGraphNode,
  type EdgeKind,
  type FilterUnit,
  filterUnits,
  type GraphEmptyReason,
  type GraphEvidence,
  type GraphNode,
  graphEmptyReasons,
} from "./graph";

/**
 * 要求が与えた期間と収集元の収録範囲の重なり方。
 * 定義元は `backend/core/timeline.go` の `CoverageState` である。
 */
export const coverageStates = [
  "covered",
  "partially_covered",
  "outside_recording",
  "recording_range_unknown",
  "range_not_requested",
  "undetermined",
] as const;
export type CoverageState = (typeof coverageStates)[number];

/** 時系列の 1 行。原資料の値を持たず、レコードの位置を持つ。 */
export type TimelineEntry = GraphEvidence & {
  /** 出ない場合は、レコードが端末の識別鍵に入る値を持たない。 */
  terminal?: GraphNode;
  /** 出ない場合は、レコードがアカウントの識別鍵に入る値を持たない。 */
  account?: GraphNode;
  /**
   * 行の時刻が、レコードの事象の時刻のほかに記録した時刻であるときの、その時刻の欄の名前。
   * 1 つのレコードは時刻ごとに行を持つ (定義元は `backend/core/timeline.go` の `TimelineEntry`)。
   */
  timeFieldName?: string;
  accountRoles?: EdgeKind[];
  otherAccounts?: { role: EdgeKind; node: GraphNode }[];
  sourceAddress?: string;
};

/** `TimelineEntry` を検証する。 */
export const decodeTimelineEntry: Decoder<TimelineEntry> = (input, path) => {
  const source = readObject(input, path);
  return {
    ...decodeGraphEvidence(input, path),
    terminal: optionalMember(source, "terminal", path, decodeGraphNode),
    account: optionalMember(source, "account", path, decodeGraphNode),
    timeFieldName: optionalString(source, "timeFieldName", path),
    accountRoles: optionalArray(source, "accountRoles", path, decodeEdgeKind),
    otherAccounts: optionalArray(source, "otherAccounts", path, (item, at) => {
      const account = readObject(item, at);
      return {
        role: decodeEdgeKind(account.role, `${at}.role`),
        node: decodeGraphNode(account.node, `${at}.node`),
      };
    }),
    sourceAddress: optionalString(source, "sourceAddress", path),
  };
};

/** 時系列の行を区別する鍵。同じレコードの別の時刻の行は、別の鍵を持つ。 */
export function timelineEntryKey(entry: TimelineEntry): string {
  return JSON.stringify([
    entry.recordRef.recordRawTextRef,
    entry.timeFieldName ?? null,
  ]);
}

/**
 * 収集元 1 件の収録範囲と、要求の期間との重なり方。
 *
 * **`matchedRecordCount` が 0 であることを「記録が無い」と読まない。** 記録の不在は
 * `state` が `outside_recording` または `recording_range_unknown` であることが示す。
 */
export type SourceCoverage = {
  sourceId: string;
  /** 分析者が読む収集元の file 名。 */
  sourceFileName: string;
  /** 出ない場合は、収集元が絶対時刻として読める根拠を 1 件も持たない。 */
  observedRangeFirst?: Timestamp;
  observedRangeLast?: Timestamp;
  state: CoverageState;
  /** 絞り込みを通ったレコードの件数。母集団は取り込みに成功し公開されたレコードである。 */
  matchedRecordCount: number;
  /** 絞り込みを通った時系列の行の数。レコードは事象の時刻ごとに行を持ちうる。 */
  matchedRowCount: number;
};

/** `SourceCoverage` を検証する。 */
export const decodeSourceCoverage: Decoder<SourceCoverage> = (input, path) => {
  const source = readObject(input, path);
  return {
    sourceId: requireString(source, "sourceId", path),
    sourceFileName: requireString(source, "sourceFileName", path),
    observedRangeFirst: optionalMember(
      source,
      "observedRangeFirst",
      path,
      decodeTimestamp,
    ),
    observedRangeLast: optionalMember(
      source,
      "observedRangeLast",
      path,
      decodeTimestamp,
    ),
    state: requireEnum(source, "state", path, coverageStates),
    matchedRecordCount: requireCount(source, "matchedRecordCount", path),
    matchedRowCount: requireCount(source, "matchedRowCount", path),
  };
};

/** `backend/api/timeline.go` の `periodUnjudged`。期間の判定から外れたレコードの件数。 */
export type PeriodUnjudged = {
  /** UTC からのずれが決まらない地方時の文字列を持つレコードの件数。 */
  localRecordCount: number;
  /** 時刻を持たないレコードの件数。 */
  undatedRecordCount: number;
};

const decodePeriodUnjudged: Decoder<PeriodUnjudged> = (input, path) => {
  const source = readObject(input, path);
  return {
    localRecordCount: requireCount(source, "localRecordCount", path),
    undatedRecordCount: requireCount(source, "undatedRecordCount", path),
  };
};

/** 時系列の操作の応答。上限も続きを取る位置も持たない。 */
export type TimelineResponse = {
  /**
   * 事象の時刻の昇順に並んだ行。UTC からのずれの決まらない地方時の行は、時点を持つ行の
   * 後ろに収集元ごとにまとまって並ぶ (定義元は `backend/api/timeline.go` の `timelineResponse`)。
   */
  entries: TimelineEntry[];
  /** 行の件数。`entries.length` と等しい。 */
  entryCount: number;
  /**
   * 絞り込みを通りながら比較に用いる時刻も地方時の文字列も持たないレコードの件数。
   * **この件数のレコードは `entries` に入らない。** ずれの決まらない地方時のレコードは
   * この件数に入らず、`entries` の末尾に並ぶ。
   */
  undatedRecordCount: number;
  /**
   * 期間を指定した要求で、時点を持たないため期間の判定から外れ、`entries` に入らなかった
   * レコードの件数。期間を指定しない要求では出ない。
   */
  periodUnjudged?: PeriodUnjudged;
  /** 収集元ごとの収録範囲。並びは取り込みの入力順である。 */
  sourceCoverages: SourceCoverage[];
  accountNodeId?: string;
  /** 応答が用いた絞り込みの条件。要求が省略した項目は出ない。 */
  eventCategory?: string;
  eventAction?: string;
  /** 応答が根拠のレコードを絞った案件。 */
  case?: CaseId;
  /** 応答が根拠のレコードを絞った端末のノードの識別子。 */
  terminal?: string;
  /** 応答が根拠のレコードを絞った検索式。要求が与えた文字列のままである。 */
  searchExpression?: string;
  timeFrom?: RequestedTime;
  timeTo?: RequestedTime;
  filterUnit?: FilterUnit;
  /**
   * 要求の文字列を原文に含む行の、`entries` での位置。昇順に並ぶ。文字列を与えない要求では出ない。
   * 定義元は backend の `timelineResponse.FindMatches` である。
   */
  findMatches?: number[];
  /** 行が 0 件になった理由。出る条件は `entryCount` が 0 のときである。 */
  emptyReason?: GraphEmptyReason;
};

/** `TimelineResponse` を検証する。 */
export const decodeTimelineResponse: Decoder<TimelineResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const entries = requireArray(source, "entries", path, decodeTimelineEntry);
  const entryCount = requireCount(source, "entryCount", path);
  if (entryCount !== entries.length) {
    throw new DecodeFailure(
      `${path}.entryCount`,
      `expected ${entries.length} to match the number of entries`,
    );
  }
  // **0 件の応答は理由を必ず持つ。** 理由の無い 0 件は、絞り込みの結果なのか応答の
  // 欠落なのかを画面が読み分けられない (`backend/api/timeline.go` の `EmptyReason`)。
  let emptyReason: GraphEmptyReason | undefined;
  if (entryCount === 0) {
    emptyReason = requireEnum(source, "emptyReason", path, graphEmptyReasons);
  } else {
    rejectMember(
      source,
      "emptyReason",
      path,
      "expected no reason while entryCount is 1 or more",
    );
  }
  const findMatches = optionalArray(source, "findMatches", path, (item, at) => {
    if (
      typeof item !== "number" ||
      !Number.isInteger(item) ||
      item < 0 ||
      item >= entries.length
    ) {
      throw new DecodeFailure(at, "expected the position of an entry");
    }
    return item;
  });
  // 次の一致・前の一致は昇順を前提に探すため、並びを境界で確かめる。
  findMatches?.forEach((item, index) => {
    if (index > 0 && item <= (findMatches[index - 1] ?? -1)) {
      throw new DecodeFailure(
        `${path}.findMatches[${index}]`,
        "expected positions in ascending order",
      );
    }
  });
  return {
    entries,
    entryCount,
    undatedRecordCount: requireCount(source, "undatedRecordCount", path),
    periodUnjudged: optionalMember(
      source,
      "periodUnjudged",
      path,
      decodePeriodUnjudged,
    ),
    sourceCoverages: requireArray(
      source,
      "sourceCoverages",
      path,
      decodeSourceCoverage,
    ),
    accountNodeId: optionalString(source, "accountNodeId", path),
    eventCategory: optionalString(source, "eventCategory", path),
    eventAction: optionalString(source, "eventAction", path),
    case: optionalMember(source, "case", path, decodeCaseId),
    terminal: optionalString(source, "terminal", path),
    searchExpression: optionalString(source, "searchExpression", path),
    timeFrom: optionalMember(source, "timeFrom", path, decodeRequestedTime),
    timeTo: optionalMember(source, "timeTo", path, decodeRequestedTime),
    filterUnit: optionalEnum(source, "filterUnit", path, filterUnits),
    findMatches,
    emptyReason,
  };
};

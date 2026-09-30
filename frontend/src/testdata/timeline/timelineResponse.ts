/**
 * 時系列の操作 (`GET /api/v0/timeline`) の応答の JSON。
 * 端末と時刻の文字列、レコードの通番と行番号は本 fixture が決める値である。
 */

import {
  accessLogSha256,
  accessLogSourceId,
  hostALogSha256,
  hostALogSourceId,
} from "../sources/sourcesResponse";

/** 行が指す端末のノード。 */
export const timelineTerminalNodeId = "n:terminal:1f0c";
/** 行が指すアカウントのノード。 */
export const timelineAccountNodeId = "n:account:3b91";

function recordRef(sequenceNumber: number, lineNumber: number) {
  return {
    sourceId: hostALogSourceId,
    sourceContentSha256: hostALogSha256,
    sourceFileName: "host-a.log",
    positionKind: "sequence_number",
    sequenceNumber,
    lineNumber,
    recordRawTextRef: `/api/v0/records?sequenceNumber=${sequenceNumber}`,
  };
}

function eventTime(rawText: string, normalized: string) {
  return {
    rawText,
    normalized,
    normalizedForm: "rfc3339_absolute",
    precision: "millisecond",
    offsetState: "in_value",
    offsetText: "+0900",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
}

function observationKind(category: string, action: string) {
  return {
    raw: [
      {
        name: "evt",
        semantic: "event.category",
        kind: "text",
        text: { rawText: category, valueState: "present" },
      },
      {
        name: "subEvt",
        semantic: "event.action",
        kind: "text",
        text: { rawText: action, valueState: "present" },
      },
    ],
    status: "determined",
  };
}

function terminalNode() {
  return {
    id: timelineTerminalNodeId,
    kind: "terminal",
    keyForm: "terminal_id",
    identity: [{ semantic: "terminal.id", value: "HOST-C-TMID" }],
    label: { rawText: "HOST-C", valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
  };
}

function accountNode() {
  return {
    id: timelineAccountNodeId,
    kind: "account",
    keyForm: "account_sid",
    identity: [{ semantic: "account.sid", value: "S-1-5-21-1001" }],
    label: { rawText: "user01", valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
  };
}

/** 収録範囲を持ち、要求した期間を覆う収集元。 */
function coveredSource(matchedRecordCount: number) {
  return {
    sourceId: hostALogSourceId,
    sourceFileName: "host-a.log",
    observedRangeFirst: eventTime(
      "2031/10/08 09:40:02.300",
      "2031-10-08T09:40:02.300+09:00",
    ),
    observedRangeLast: eventTime(
      "2031/10/08 11:05:48.500",
      "2031-10-08T11:05:48.500+09:00",
    ),
    state: "covered",
    matchedRecordCount,
    matchedRowCount: matchedRecordCount,
  };
}

/**
 * 要求した期間に記録を持たない収集元。
 * **記録が無い状態と、事象が無い状態を分ける fixture である。**
 */
function outsideRecordingSource() {
  return {
    sourceId: "source:access-log",
    sourceFileName: "access.log",
    observedRangeFirst: eventTime(
      "2031/10/08 10:40:00.000",
      "2031-10-08T10:40:00.000+09:00",
    ),
    observedRangeLast: eventTime(
      "2031/10/08 11:02:30.000",
      "2031-10-08T11:02:30.000+09:00",
    ),
    state: "outside_recording",
    matchedRecordCount: 0,
    matchedRowCount: 0,
  };
}

/** 収録範囲を読めない収集元。 */
function unknownRangeSource() {
  return {
    sourceId: "source:audit-log",
    sourceFileName: "audit.log",
    state: "recording_range_unknown",
    matchedRecordCount: 0,
    matchedRowCount: 0,
  };
}

/** 2 件の行と、状態の違う 3 件の収集元を持つ応答。 */
export function timelineResponseJson() {
  return {
    entries: [
      {
        recordRef: recordRef(2204, 1022),
        eventTime: eventTime(
          "2031/10/08 10:20:35.100",
          "2031-10-08T10:20:35.100+09:00",
        ),
        observationKind: observationKind("net", "con"),
        terminal: terminalNode(),
        account: accountNode(),
      },
      {
        recordRef: recordRef(2207, 1025),
        eventTime: eventTime(
          "2031/10/08 10:20:36.200",
          "2031-10-08T10:20:36.200+09:00",
        ),
        observationKind: observationKind("net", "dcon"),
        terminal: terminalNode(),
      },
    ],
    entryCount: 2,
    undatedRecordCount: 3,
    sourceCoverages: [
      coveredSource(2),
      outsideRecordingSource(),
      unknownRangeSource(),
    ],
  };
}

/**
 * 生成する行の収集元。`hostA` と `hostACopy` は file 名が同じで、内容と識別子が違う。
 * `hostACopy` の識別子と内容の識別は本 fixture が決める値である。
 */
export const generatedSources = {
  hostA: {
    sourceId: hostALogSourceId,
    contentSha256: hostALogSha256,
    fileName: "host-a.log",
  },
  hostACopy: {
    sourceId: "ingest-host-a-log-2",
    contentSha256:
      "5f0a1c2e3d4b5a69788796a5b4c3d2e1f00112233445566778899aabbccddeef",
    fileName: "host-a.log",
  },
  access: {
    sourceId: accessLogSourceId,
    contentSha256: accessLogSha256,
    fileName: "access.log",
  },
} as const;

export type GeneratedSourceName = keyof typeof generatedSources;

/** 生成する行 1 件の仕様。 */
export type GeneratedEntrySpec = {
  /** 事象の時刻の正規化値。`offsetState` が `in_value` のとき RFC 3339 の文字列を渡す。 */
  normalized: string;
  precision?: string;
  offsetState?: string;
  /** 分析者が与えた UTC からのずれ。ずれを持たない地方時にだけ渡す。 */
  interpretationOffset?: string;
  source?: GeneratedSourceName;
};

/** 仕様の並びから時系列の行の JSON を作る。通番は並びの位置から振る。 */
export function generatedTimelineEntriesJson(specs: GeneratedEntrySpec[]) {
  return specs.map((spec, index) => {
    const source = generatedSources[spec.source ?? "hostA"];
    const sequenceNumber = 1000 + index;
    const offsetState = spec.offsetState ?? "in_value";
    const carriesOffset = offsetState === "in_value" || offsetState === "epoch";
    return {
      recordRef: {
        sourceId: source.sourceId,
        sourceContentSha256: source.contentSha256,
        sourceFileName: source.fileName,
        positionKind: "sequence_number",
        sequenceNumber,
        lineNumber: sequenceNumber,
        recordRawTextRef: `/api/v0/records?sequenceNumber=${sequenceNumber}`,
      },
      eventTime: {
        rawText: spec.normalized,
        normalized: spec.normalized,
        normalizedForm: carriesOffset
          ? "rfc3339_absolute"
          : "local_without_offset",
        precision: spec.precision ?? "millisecond",
        offsetState,
        clock: "terminal_local",
        meaning: "event",
        valueState: "present",
        ...(spec.interpretationOffset === undefined
          ? {}
          : {
              interpretation: {
                offset: spec.interpretationOffset,
                assertionId: "as:generated",
              },
            }),
      },
      observationKind: observationKind("net", "con"),
      terminal: terminalNode(),
    };
  });
}

/**
 * 開始の時刻から一定の間隔で並ぶ行の仕様を作る。収集元は並びの位置で順に回す。
 * 時刻は UTC の文字列で書く。
 */
export function evenlySpacedEntrySpecs(
  count: number,
  startUtc: string,
  stepMs: number,
  sources: GeneratedSourceName[] = ["hostA"],
): GeneratedEntrySpec[] {
  const startMs = Date.parse(startUtc);
  return Array.from({ length: count }, (_, index) => ({
    normalized: new Date(startMs + index * stepMs).toISOString(),
    source: sources[index % sources.length],
  }));
}

/** 生成した行を含む応答。収録範囲は行を持つ収集元を、`generatedSources` の順で並べる。 */
export function generatedTimelineResponseJson(specs: GeneratedEntrySpec[]) {
  const entries = generatedTimelineEntriesJson(specs);
  const coverages = Object.entries(generatedSources).flatMap(
    ([name, source]) => {
      const matchedRecordCount = specs.filter(
        (spec) => (spec.source ?? "hostA") === name,
      ).length;
      return matchedRecordCount === 0
        ? []
        : [
            {
              sourceId: source.sourceId,
              sourceFileName: source.fileName,
              state: "range_not_requested",
              matchedRecordCount,
              matchedRowCount: matchedRecordCount,
            },
          ];
    },
  );
  return {
    entries,
    entryCount: entries.length,
    undatedRecordCount: 0,
    sourceCoverages: coverages,
  };
}

/** 行が 0 件の応答。収集元の収録範囲は含む。 */
export function emptyTimelineResponseJson() {
  return {
    entries: [],
    entryCount: 0,
    undatedRecordCount: 0,
    sourceCoverages: [coveredSource(0), outsideRecordingSource()],
    eventCategory: "no_such_category",
    emptyReason: "no_record_in_filter",
  };
}

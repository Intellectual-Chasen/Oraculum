/**
 * 操作 3 と操作 7 (`GET /api/v0/records`) の応答の JSON。
 * `fields` の `name` の集合は、返すレコードが持つ項目が決める。
 * `name` と `kind` と `valueState` と値は `backend/api` の応答を写す。
 * レコードの原文と正規化値と到達した経路の各段階、`sourceId` と原文への参照、
 * 各収集元の内容の識別と byte 数は、本 fixture が決める値である。
 */

import {
  accessLogSha256,
  accessLogSource,
  accessLogSourceId,
  hostALogSha256,
  hostALogSource,
  hostALogSourceId,
} from "../sources/sourcesResponse";

/** `access.log` の 37 行の原文。 */
export const accessLogRecordRawText =
  '192.0.2.101 - - [08/Oct/2031:10:20:35 +0900] "GET http://203.0.113.21/example HTTP/1.1" 200 1234 "-" "-" TCP_MEM_HIT:HIER_NONE';

/**
 * `host-a.log` の `sn=112` の 1 行。Mark II のレコードの key の並びと文字列の形を持ち、
 * `loc` / `type` / `lv` / `os` / `domain` / `profile` / `mac` / `sessionID` を持たない
 * 部分集合である。
 */
export const hostALogRecordRawText =
  '10/08/2031 10:20:35.100 +0900 sn=112 evt=net subEvt=con com="HOST-A" ' +
  "tmid=00000000-0000-4000-8000-00000000000a " +
  "csid=S-1-5-21-1000000001-1000000002-1000000003 " +
  "ip=192.0.2.101,2001:db8::101 " +
  "psGUID={00000000-0000-4000-8000-000000000001} " +
  'psPath="C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe" ' +
  "srcIP=192.0.2.101 srcPort=50000 dstIP=203.0.113.21 dstPort=80";

/**
 * `clientTerminal` が持つ端末の外部識別子と、`clientTerminalName` が持つ表示名。
 * 値は `host-a.log` の行が持つ `tmid` と `com` である。
 */
export const clientTerminalId = "00000000-0000-4000-8000-00000000000a";
export const clientTerminalName = "HOST-A";

/**
 * `clientTerminal` と `clientTerminalName` の導き方。
 * 2 項目は同じ割当 1 件から出るため、同じ文字列を持つ。
 * 項目の名前と区切りと並びは `backend/pipeline/fields_builder.go` の
 * `derivedTerminalDerivation` が組み立てる文字列に合わせる。適用期間の両端は `HOST-A` の
 * 観測期間の正規化値である。
 */
export const clientTerminalDerivation =
  "clientIp=192.0.2.101; " +
  "rule=terminal_ip_assignment; " +
  `sourceId=${hostALogSourceId}; ` +
  "assignmentValidRange=" +
  "2031-10-08T09:40:02.300+09:00/2031-10-08T11:05:48.500+09:00";

/**
 * 要求先の値。`backend/pipeline/binding_squid.go` の `requestTargetHost` と
 * `requestTargetPort` が返す原資料の文字列と正規化値と導き方である。
 * 2 項目は同じ `%ru` の文字列を原資料の文字列に持つ。
 */
export const requestTargetRawText = "http://203.0.113.21/example";
export const requestTargetHost = "203.0.113.21";
export const requestTargetHostDerivation =
  "要求先の URI の authority から userinfo と port と IPv6 の角括弧を外した host";
export const requestTargetPort = "80";
export const requestTargetPortDerivation = "要求先の scheme の既定の port";

/**
 * レコードの原文への参照。画面は `raw:` に続く文字列を解釈せずに server へ返すので、
 * 値はレコードごとに異なる文字列である。
 */
const accessLogLine37RawTextRef = "raw:access-log-37";
const hostARawTextRefBySequenceNumber: Record<number, string> = {
  104: "raw:host-a-104",
  107: "raw:host-a-107",
  108: "raw:host-a-108",
  111: "raw:host-a-111",
  112: "raw:host-a-112",
};

function hostARawTextRef(sequenceNumber: number): string {
  const ref = hostARawTextRefBySequenceNumber[sequenceNumber];
  if (ref === undefined) {
    throw new Error(`no raw text ref for sequence number ${sequenceNumber}`);
  }
  return ref;
}

function presentText(rawText: string): unknown {
  return { rawText, valueState: "present" };
}

function absentText(rawText: string): unknown {
  return { rawText, valueState: "absent" };
}

function itemAbsentText(): unknown {
  return { valueState: "item_absent" };
}

function normalizedText(
  rawText: string,
  normalized: string,
  derivation: string,
): unknown {
  return { rawText, normalized, derivation, valueState: "present" };
}

function textField(name: string, text: unknown): unknown {
  return { name, kind: "text", text };
}

/**
 * `access.log` の 37 行の項目。
 * combined の欄、同じレコードの `%ru` から導く項目、段階 2 が補う項目の順に並ぶ。
 */
function accessLogFields(): unknown[] {
  return [
    textField("clientIp", presentText("192.0.2.101")),
    textField("ident", absentText("-")),
    textField("user", absentText("-")),
    {
      name: "requestTime",
      kind: "timestamp",
      timestamp: {
        rawText: "[08/Oct/2031:10:20:35 +0900]",
        normalized: "2031-10-08T10:20:35+09:00",
        normalizedForm: "rfc3339_absolute",
        precision: "second",
        offsetState: "in_value",
        offsetText: "+0900",
        clock: "observer_local",
        meaning: "event",
        valueState: "present",
      },
    },
    textField(
      "requestLine",
      normalizedText(
        '"GET http://203.0.113.21/example HTTP/1.1"',
        "GET http://203.0.113.21/example HTTP/1.1",
        "引用符を外した文字列",
      ),
    ),
    textField("statusCode", presentText("200")),
    textField("replyBytes", presentText("1234")),
    textField("referer", absentText('"-"')),
    textField("userAgent", absentText('"-"')),
    textField("squidStatus", presentText("TCP_MEM_HIT:HIER_NONE")),
    textField("requestMethod", presentText("GET")),
    textField(
      "requestTargetHost",
      normalizedText(
        requestTargetRawText,
        requestTargetHost,
        requestTargetHostDerivation,
      ),
    ),
    textField(
      "requestTargetPort",
      normalizedText(
        requestTargetRawText,
        requestTargetPort,
        requestTargetPortDerivation,
      ),
    ),
    textField("clientTerminal", {
      normalized: clientTerminalId,
      derivation: clientTerminalDerivation,
      valueState: "derived",
    }),
    textField("clientTerminalName", {
      normalized: clientTerminalName,
      derivation: clientTerminalDerivation,
      valueState: "derived",
    }),
    textField("clientPort", itemAbsentText()),
    textField("process", itemAbsentText()),
  ];
}

/**
 * `host-a.log` の `sn=112` の項目。並びは原文の key の並びで、ヘッダーの時刻を
 * 先頭に置き、当該レコードに出ない `recv` と `send` を末尾に置く。
 *
 * **段階 2 の補いが 1 件も無い。** レコードが `tmid` と `com` と `srcIP` と `srcPort` と
 * `psGUID` を持つため、接続元と実行主体の項目を補う対象にならない。
 */
function hostALogFields(): unknown[] {
  return [
    {
      name: "headerTime",
      kind: "timestamp",
      timestamp: {
        rawText: "10/08/2031 10:20:35.100 +0900",
        normalized: "2031-10-08T10:20:35.100+09:00",
        normalizedForm: "rfc3339_absolute",
        precision: "millisecond",
        offsetState: "in_value",
        offsetText: "+0900",
        clock: "terminal_local",
        meaning: "event",
        valueState: "present",
      },
    },
    textField("sn", presentText("112")),
    textField("evt", presentText("net")),
    textField("subEvt", presentText("con")),
    textField(
      "com",
      normalizedText('"HOST-A"', "HOST-A", "引用符を外した文字列"),
    ),
    textField("tmid", presentText(clientTerminalId)),
    textField("csid", presentText("S-1-5-21-1000000001-1000000002-1000000003")),
    textField("ip", presentText("192.0.2.101,2001:db8::101")),
    textField("psGUID", presentText("{00000000-0000-4000-8000-000000000001}")),
    textField(
      "psPath",
      normalizedText(
        '"C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe"',
        "C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe",
        "引用符を外した文字列",
      ),
    ),
    textField("srcIP", presentText("192.0.2.101")),
    textField("srcPort", presentText("50000")),
    textField("dstIP", presentText("203.0.113.21")),
    textField("dstPort", presentText("80")),
    textField("recv", itemAbsentText()),
    textField("send", itemAbsentText()),
  ];
}

function accessLogRecordRef(): unknown {
  return {
    sourceId: accessLogSourceId,
    sourceContentSha256: accessLogSha256,
    sourceFileName: "access.log",
    positionKind: "line_number",
    lineNumber: 37,
    recordRawTextRef: accessLogLine37RawTextRef,
  };
}

function hostALogRecordRef(
  sequenceNumber: number,
  lineNumber: number,
): unknown {
  return {
    sourceId: hostALogSourceId,
    sourceContentSha256: hostALogSha256,
    sourceFileName: "host-a.log",
    positionKind: "sequence_number",
    sequenceNumber,
    lineNumber,
    recordRawTextRef: hostARawTextRef(sequenceNumber),
  };
}

/** 操作 3 の応答。`access.log` の 37 行を返し、到達した経路を持たない。 */
export function accessLogRecordResponseJson(): unknown {
  return {
    recordRef: accessLogRecordRef(),
    rawText: accessLogRecordRawText,
    sourceIdentity: accessLogSource(),
    fields: accessLogFields(),
    observationKind: { raw: [] },
  };
}

/** 段階 A3 が参照する端末のログのうち、`host-a.log` 以外の 4 件。 */
const otherEndpointLogNames = [
  "host-b.log",
  "host-c.log",
  "host-d.log",
  "host-e.log",
] as const;

function endpointLogSource(fileName: string, index: number): unknown {
  const suffix = String(index + 1).padStart(2, "0");
  return {
    sourceId: `ingest-${fileName.replace(".log", "")}-log-1`,
    contentSha256: suffix.repeat(32),
    originPath: `/data/example/endpoint/${fileName}`,
    fileName,
    sizeBytes: 9000000,
    newlineCount: 4000,
    endsWithNewline: true,
    lineEnding: "crlf",
    formatKey: "infotrace_mark_ii",
  };
}

function recordInput(record: unknown): unknown {
  return { kind: "record", record };
}

function sourceInput(source: unknown): unknown {
  return { kind: "source", source };
}

/** Proxy のレコードから候補のプロセスへ到達した経路の段階 A1 から A8。 */
function trailSteps(): unknown[] {
  return [
    {
      stepKey: "A1",
      usedIdentifiers: ["端末名 HOST-A", "URL http://203.0.113.21/example"],
      output: "探す対象の URL と端末",
    },
    {
      stepKey: "A2",
      inputRefs: [sourceInput(accessLogSource())],
      usedIdentifiers: ["接続元 IP 192.0.2.101", "%ru の完全一致"],
      output: "37 行 (1 件)",
    },
    {
      stepKey: "A3",
      inputRefs: [
        sourceInput(hostALogSource()),
        ...otherEndpointLogNames.map((fileName, index) =>
          sourceInput(endpointLogSource(fileName, index)),
        ),
      ],
      usedIdentifiers: ["IP から端末への割当", "適用範囲は HOST-A の観測期間"],
      output: "接続元の端末は HOST-A、収集元 file は host-a.log",
    },
    {
      stepKey: "A4",
      inputRefs: [recordInput(accessLogRecordRef())],
      usedIdentifiers: [
        "%ru の authority 部分 203.0.113.21",
        "scheme http (port の指定無し)",
      ],
      output: "比較する値 dstIP=203.0.113.21、dstPort=80",
    },
    {
      stepKey: "A5",
      inputRefs: [sourceInput(hostALogSource())],
      usedIdentifiers: [
        "evt=net",
        "subEvt が con または est",
        "dstIP",
        "dstPort",
      ],
      output: "候補 4 件 (sn 111 / 112 / 131 / 132)、プロセス 2 個",
    },
    {
      stepKey: "A6",
      inputRefs: [recordInput(accessLogRecordRef())],
      usedIdentifiers: ["ヘッダー時刻の秒部分の一致"],
      output: "候補 2 件 (sn 111 / 112)、プロセス 1 個",
    },
    {
      stepKey: "A7",
      inputRefs: [
        recordInput(hostALogRecordRef(111, 1021)),
        recordInput(hostALogRecordRef(112, 1022)),
      ],
      usedIdentifiers: [
        "psGUID={00000000-0000-4000-8000-000000000001}",
        "tmid",
        "com",
        "csid",
      ],
      output: "候補のプロセス 1 個",
    },
    {
      stepKey: "A8",
      inputRefs: [sourceInput(hostALogSource())],
      usedIdentifiers: ["evt=ps subEvt=start", "psGUID の一致"],
      output: "sn=107 (1 件)。起動時刻は同レコードのヘッダー時刻 10:20:34.800",
    },
  ];
}

/** 操作 7 の応答。`host-a.log` の `sn=112` を返し、段階 A1 から A8 の到達した経路を持つ。 */
export function hostALogRecordResponseJson(): unknown {
  return {
    recordRef: hostALogRecordRef(112, 1022),
    rawText: hostALogRecordRawText,
    sourceIdentity: hostALogSource(),
    fields: hostALogFields(),
    observationKind: {
      raw: [
        textField("evt", presentText("net")),
        textField("subEvt", presentText("con")),
      ],
      status: "determined",
    },
    derivationTrail: {
      originRef: accessLogRecordRef(),
      steps: trailSteps(),
    },
  };
}

/**
 * 進めなかった段階を持つ応答。
 * 段階 C5 は、候補のプロセスの親を同じ `psGUID` の `ps` / `start` で探し、0 件で止まる。
 */
export function stoppedRecordResponseJson(): unknown {
  const stoppedStep = {
    stepKey: "C5",
    inputRefs: [recordInput(hostALogRecordRef(104, 1014))],
    usedIdentifiers: ["parentGUID={00000000-0000-4000-8000-000000000002}"],
    output: "同じ psGUID の ps / start が 0 件",
  };
  return {
    recordRef: hostALogRecordRef(112, 1022),
    rawText: hostALogRecordRawText,
    sourceIdentity: hostALogSource(),
    fields: hostALogFields(),
    observationKind: {
      raw: [
        textField("evt", presentText("net")),
        textField("subEvt", presentText("con")),
      ],
      status: "determined",
    },
    derivationTrail: {
      originRef: hostALogRecordRef(107, 1017),
      steps: [stoppedStep],
      stoppedAt: stoppedStep,
    },
  };
}

/**
 * 同じ key を 2 回書いたレコードの原文。`hide` は Mark II のレコードが持つ key で、
 * 値に `1` をとる。同じ key を 2 回書いた行は本 fixture が生成する。
 */
const repeatedKeyRawText =
  '10/08/2031 10:20:35.100 +0900 sn=112 evt=net subEvt=con com="HOST-A" ' +
  "tmid=00000000-0000-4000-8000-00000000000a " +
  "csid=S-1-5-21-1000000001-1000000002-1000000003 " +
  "ip=192.0.2.101,2001:db8::101 hide=1 hide=2 " +
  "psGUID={00000000-0000-4000-8000-000000000001} " +
  'psPath="C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe" ' +
  "srcIP=192.0.2.101 srcPort=50000 dstIP=203.0.113.21 dstPort=80";

/** `hide` の 2 回の出現が持つ文字列。2 件は別の値を持つ。 */
export const repeatedKeyValues = ["1", "2"] as const;

/**
 * `repeatedKeyRawText` の項目。`ip` の次に `hide` を 2 件置く。並びは原文の key の
 * 並びと同じである。
 */
function repeatedKeyFields(): unknown[] {
  const fields = hostALogFields();
  const ipIndex = fields.findIndex(
    (field) => (field as { name: string }).name === "ip",
  );
  if (ipIndex < 0) {
    throw new Error("the host-a.log fields carry no item named ip");
  }
  fields.splice(
    ipIndex + 1,
    0,
    ...repeatedKeyValues.map((value) => textField("hide", presentText(value))),
  );
  return fields;
}

/**
 * 同じ `name` を 2 件持つ応答。
 * backend は原資料の key の出現をそのまま返すため、同じ key を 2 回書いたレコードの
 * 応答は同じ `name` の要素を 2 件持つ。
 */
export function repeatedFieldNameRecordResponseJson(): unknown {
  return {
    recordRef: hostALogRecordRef(112, 1022),
    rawText: repeatedKeyRawText,
    sourceIdentity: hostALogSource(),
    fields: repeatedKeyFields(),
    observationKind: {
      raw: [
        textField("evt", presentText("net")),
        textField("subEvt", presentText("con")),
      ],
      status: "determined",
    },
  };
}

/** HTML の文字列を持つレコードの原文。 */
export const htmlLikeRawText =
  '192.0.2.101 - - [08/Oct/2031:10:20:35 +0900] "GET http://203.0.113.21/<script>alert(1)</script> HTTP/1.1" 200 1234 "-" "-" TCP_MEM_HIT:HIER_NONE';

/**
 * レコードの原文が HTML の文字列を持つ応答。
 * 原資料の文字列を実行できる HTML として扱わないことを確かめる。値は本 fixture が生成する。
 */
export function htmlLikeRecordResponseJson(): unknown {
  return {
    ...(accessLogRecordResponseJson() as Record<string, unknown>),
    rawText: htmlLikeRawText,
  };
}

/**
 * レコードの原文に bidi 制御 (U+202E) と tab (U+0009) を持つ応答。
 * 原資料が制御文字と書式文字を持つ場合の画面の表現を確かめる。値は本 fixture が生成する。
 */
export function invisibleCharacterRecordResponseJson(): unknown {
  return {
    ...(accessLogRecordResponseJson() as Record<string, unknown>),
    rawText: `${accessLogRecordRawText}\u202e\t`,
  };
}

/**
 * `recordRef` を持つ `ApiError` の文字列。**画面が読める形を手で組んだ値である。**
 *
 * `backend/core/api_error.go` の `ApiError` は `recordRef` を `code` のどの値でも省略可と
 * 定め、出す条件を `code` で限定しない。**現在の backend には `recordRef` を載せた
 * `ApiError` を返す経路が無い。**
 *
 * したがって本 fixture を使ってよいのは `decodeApiError` の検査だけである。画面の検査に
 * 使うと、実装が出さない値で緑になる。
 */
export function apiErrorWithRecordRefJson(): unknown {
  return {
    code: "internal_error",
    message: "reading the record failed",
    sourceId: hostALogSourceId,
    sourceContentSha256: hostALogSha256,
    recordRef: hostALogRecordRef(108, 1018),
  };
}

/**
 * `backend/api/records.go` の `writeRecord` が応答の本体を組めなかったときの失敗の応答。
 * `code` と `message` だけを載せる形は `backend/api` の `internal_error` に共通である。
 */
export function internalErrorJson(): unknown {
  return {
    code: "internal_error",
    message: "reading the record failed",
  };
}

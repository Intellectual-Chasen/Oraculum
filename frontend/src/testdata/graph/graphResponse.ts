/**
 * 操作 9 (`GET /api/v0/graph`) と操作 10 (`GET /api/v0/nodes/{id}`) の応答の JSON。
 * ノードとエッジの識別子は backend が発行する不透明な値であり、本 fixture が決める。
 * 端末と接続先と時刻の文字列、レコードの通番と行番号は本 fixture が決める値である。
 */

import { hostALogSha256, hostALogSourceId } from "../sources/sourcesResponse";

/** 端末のノード。 */
export const terminalNodeId = "n:terminal:1f0c";
/** プロセスのノード。 */
export const processNodeId = "n:process:8ab3";
/** 接続先の IP アドレスのノード。エッジの端点として応答に入る。 */
export const ipNodeId = "n:ip:5d72";
export const recordNodeId = "n:record:7c41";

/** 端末の外部識別子の文字列。 */
export const terminalIdText = "HOST-C-TMID";

function markiiRecordRef(sequenceNumber: number, lineNumber: number) {
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

function markiiEventTime(rawText: string, normalized: string) {
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

function observationKind(action: string) {
  return {
    raw: [
      {
        name: "evt",
        semantic: "event.category",
        kind: "text",
        text: { rawText: "net", valueState: "present" },
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

function ranOnEvidence() {
  return {
    recordRef: markiiRecordRef(112, 1022),
    eventTime: markiiEventTime(
      "2031/10/08 10:20:35.100",
      "2031-10-08T10:20:35.100+09:00",
    ),
    observationKind: observationKind("con"),
    eventKind: { category: "net", action: "con" },
  };
}

/** 時刻の両端を持つ、値ごとの件数 1 件。 */
export function valueCountJson() {
  return {
    value: "ExampleClient/1.0",
    recordCount: 640,
    firstEventTime: markiiEventTime(
      "2031/10/08 10:20:35.100",
      "2031-10-08T10:20:35.100+09:00",
    ),
    lastEventTime: markiiEventTime(
      "2031/10/08 11:05:48.500",
      "2031-10-08T11:05:48.500+09:00",
    ),
  };
}

/** 時刻を読める根拠を 1 件も持たない、値ごとの件数 1 件。 */
export function valueCountWithoutTimeJson() {
  return {
    value: "ExampleAgent/2.0",
    recordCount: 90,
  };
}

/**
 * 収集元のレコード 1 件のノード。識別鍵は収集元の内容の識別と位置の組で、表示名は
 * 収集元の file 名と位置から導いた値である。
 */
export function recordNode(selection: string) {
  return {
    id: recordNodeId,
    kind: "record",
    keyForm: "source_content_sha256_position",
    identity: [
      { value: hostALogSha256 },
      { value: "sequence_number" },
      { semantic: "record.sequence_number", value: "112" },
    ],
    label: {
      normalized: "host-a.log ID 112",
      derivation: "収集元の file 名とレコードの位置",
      valueState: "derived",
    },
    observation: "observed",
    // レコードは生成を記録した根拠を持てない。
    creationRecord: "item_absent",
    selection,
  };
}

function communicationEvidence() {
  return {
    recordRef: markiiRecordRef(115, 1025),
    eventTime: markiiEventTime(
      "2031/10/08 11:05:48.500",
      "2031-10-08T11:05:48.500+09:00",
    ),
    observationKind: observationKind("dcon"),
    eventKind: { category: "net", action: "dcon" },
  };
}

function terminalNode(selection: string) {
  return {
    id: terminalNodeId,
    kind: "terminal",
    keyForm: "terminal_id",
    identity: [{ semantic: "terminal.id", value: terminalIdText }],
    label: { rawText: "HOST-C", valueState: "present" },
    observation: "observed",
    // 端末は生成を記録した根拠を持てない。
    creationRecord: "item_absent",
    selection,
  };
}

/**
 * 通信のレコードだけが記録したプロセス。
 * **`observation` と `creationRecord` が別の軸であることを、この組が表す。**
 */
function processNodeItems() {
  return {
    id: processNodeId,
    kind: "process",
    keyForm: "terminal_id_process_id",
    identity: [
      { semantic: "terminal.id", value: terminalIdText },
      { semantic: "process.id", value: "{P1}" },
    ],
    label: { rawText: "C:\\Windows\\System32\\cmd.exe", valueState: "present" },
    observation: "observed",
    creationRecord: "absent",
    // 根拠のレコードが名乗った端末。識別鍵の値と別の列に出る派生の値である。
    terminals: [terminalNode("matched")].map(({ selection, ...node }) => {
      void selection;
      return node;
    }),
  };
}

function ipNode(selection: string) {
  return {
    id: ipNodeId,
    kind: "ip",
    keyForm: "address",
    identity: [{ value: "203.0.113.21" }],
    label: { rawText: "203.0.113.21", valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
    selection,
    // 端点として入ったノードは、根拠が端末を名乗っていない。
    terminals: [],
  };
}

/** 画面が送る要求の、描画の上限の既定値。 */
export const defaultNodeLimit = 2000;

/** 画面が最初に送る、上限を置いた要求への応答。図に描くノードは 3 件で上限に収まる。 */
export function limitedGraphResponseJson() {
  return { ...graphResponseJson(), nodeLimit: defaultNodeLimit };
}

/**
 * 上限 limit を置いた要求で、図に描くノードが count 件になり上限を超えた応答。
 * 合ったノードだけを持ち、エッジを持たず、描くはずだったエッジの本数を関係の種別ごとに持つ。
 */
export function overLimitGraphResponseJson(
  count: number,
  limit = defaultNodeLimit,
) {
  const response = graphResponseJson();
  return {
    ...response,
    nodes: response.nodes.filter((node) => node.selection === "matched"),
    edges: [],
    edgeCount: 0,
    subgraphNodeCount: count,
    nodeLimit: limit,
    nodeLimitExceeded: true,
    edgeKindCounts: [
      { kind: "ran_on", count: 1200 },
      { kind: "process_communication", count: 34 },
    ],
  };
}

/**
 * 合致したノード 2 件と、エッジの端点として入るノード 1 件を含む応答。
 * `nodeCount` は `selection` が `matched` の要素数である。
 */
export function graphResponseJson() {
  return {
    nodes: [
      terminalNode("matched"),
      { ...processNodeItems(), selection: "matched" },
      ipNode("edge_endpoint"),
    ],
    nodeCount: 2,
    matchedKinds: [
      { kind: "process", count: 1 },
      { kind: "terminal", count: 1 },
    ],
    subgraphNodeCount: 3,
    edges: [
      {
        id: "e:ran_on:0001",
        kind: "ran_on",
        state: "observed",
        sourceNodeId: processNodeId,
        targetNodeId: terminalNodeId,
        applicableRange: {
          from: markiiEventTime(
            "2031/10/08 10:20:35.100",
            "2031-10-08T10:20:35.100+09:00",
          ),
          to: markiiEventTime(
            "2031/10/08 11:05:48.500",
            "2031-10-08T11:05:48.500+09:00",
          ),
        },
        evidenceCount: 4,
      },
      {
        id: "e:process_communication:0002",
        kind: "process_communication",
        state: "observed",
        sourceNodeId: processNodeId,
        targetNodeId: ipNodeId,
        evidenceCount: 1,
      },
    ],
    edgeCount: 2,
    depth: 1,
  };
}

/**
 * レコードのノードと、レコードが対象を指す関係を含む応答。
 * 読めたレコードは 1 件ずつノードになる (`backend/pipeline/graph.go` の addRecordNode)。
 */
export function recordNodeGraphResponseJson() {
  const graph = graphResponseJson();
  return {
    ...graph,
    nodes: [...graph.nodes, recordNode("matched")],
    nodeCount: graph.nodeCount + 1,
    matchedKinds: [
      { kind: "process", count: 1 },
      { kind: "record", count: 1 },
      { kind: "terminal", count: 1 },
    ],
    edges: [
      ...graph.edges,
      {
        id: "e:record_names_object:0003",
        kind: "record_names_object",
        state: "observed",
        sourceNodeId: recordNodeId,
        targetNodeId: processNodeId,
        evidenceCount: 1,
      },
    ],
    edgeCount: graph.edgeCount + 1,
  };
}

/**
 * レコードのノードが位置と時刻と事象の種別の要約を持つ応答。要求が recordSummary を
 * 与えたときの形である。
 */
export function summarizedRecordGraphResponseJson() {
  const graph = recordNodeGraphResponseJson();
  return {
    ...graph,
    nodes: graph.nodes.map((node) =>
      node.id === recordNodeId
        ? {
            ...node,
            record: {
              recordRef: markiiRecordRef(112, 1022),
              eventTime: markiiEventTime(
                "2031/10/08 10:20:35.100",
                "2031-10-08T10:20:35.100+09:00",
              ),
              eventCategory: "Example-Provider",
              eventAction: "8001",
              windowsEvent: true,
              channel: "Example-Channel",
              recordHeaderId: "5112",
              eventRecordId: "4112",
            },
          }
        : node,
    ),
  };
}

/** 端末の表示名に bidi 制御を持つ応答。画面が可視の符号にして描くことを確かめる。 */
export function invisibleCharacterGraphResponseJson() {
  const response = graphResponseJson();
  return {
    ...response,
    nodes: [
      {
        ...terminalNode("matched"),
        label: { rawText: "HOST-\u202EC", valueState: "present" },
      },
      ...response.nodes.slice(1),
    ],
  };
}

/** 絞り込みに合ったノードが 0 件の応答。 */
export function emptyGraphResponseJson() {
  return {
    nodes: [],
    nodeCount: 0,
    matchedKinds: [],
    subgraphNodeCount: 0,
    edges: [],
    edgeCount: 0,
    depth: 1,
    emptyReason: "no_record_in_filter",
  };
}

/** プロセスのノード 1 つの詳細。同じ意味に 2 通りの値を観測した属性を含む。 */
export function nodeDetailResponseJson() {
  return {
    node: processNodeItems(),
    attributes: [
      {
        semantic: "process.command_line",
        valueCount: 2,
        values: [
          {
            field: {
              name: "cmdLine",
              semantic: "process.command_line",
              kind: "text",
              text: {
                rawText: "cmd.exe /c whoami",
                valueState: "present",
              },
            },
            observationCount: 3,
            firstRecordRef: markiiRecordRef(112, 1022),
          },
          {
            field: {
              name: "cmdLine",
              semantic: "process.command_line",
              kind: "text",
              text: {
                rawText: "cmd.exe /c net use",
                valueState: "present",
              },
            },
            observationCount: 1,
            firstRecordRef: markiiRecordRef(115, 1025),
          },
        ],
      },
    ],
    attributeCount: 1,
    evidence: [ranOnEvidence(), communicationEvidence()],
    evidenceCount: 2,
    edgeCounts: [
      {
        edgeKind: "ran_on",
        direction: "outgoing",
        edgeCount: 1,
        evidenceCount: 4,
      },
      {
        edgeKind: "process_communication",
        direction: "outgoing",
        edgeCount: 1,
        evidenceCount: 1,
      },
    ],
    // このプロセスは通信のレコードだけが記録しており、生成の根拠を持たない。
    creationRecords: [],
    creationRecordCount: 0,
    logonSessionRejections: [],
  };
}

/**
 * Logon ID が一致しながら、別の端末のログオンと結ばなかったレコードのノードの詳細。
 * 関係にしなかったログオンの節を画面が出すことを、この応答が確かめる。
 */
export function rejectedLogonNodeDetailResponseJson() {
  return {
    ...nodeDetailResponseJson(),
    logonSessionRejections: [
      { reason: "other_terminal", logon: ranOnEvidence() },
    ],
  };
}

/**
 * 生成を記録した根拠を持つプロセスのノードの詳細。
 * `creationRecord` が `present` の枝を画面が出すことを、この応答が確かめる。
 */
export function createdNodeDetailResponseJson() {
  const detail = nodeDetailResponseJson();
  return {
    ...detail,
    node: { ...detail.node, creationRecord: "present" },
    creationRecords: [ranOnEvidence()],
    creationRecordCount: 1,
  };
}

/**
 * 根拠のレコード 1 件を含みながら、`evidenceCount` が 2 件を名乗るノードの詳細。
 * 一覧に出ていない根拠があることを、総数と要素数の食い違いが表す。
 */
export function evidenceCountMismatchNodeDetailResponseJson() {
  return {
    ...nodeDetailResponseJson(),
    evidence: [ranOnEvidence()],
    evidenceCount: 2,
  };
}

/**
 * 属性 1 件を含みながら、`attributeCount` が 2 件を名乗るノードの詳細。
 * 一覧に出ていない属性があることを、総数と要素数の食い違いが表す。
 */
export function attributeCountMismatchNodeDetailResponseJson() {
  return { ...nodeDetailResponseJson(), attributeCount: 2 };
}

/** 遠隔のセッションの候補のエッジ。 */
export const remoteSessionEdgeId = "e:terminal_remote_session:0003";

/** 接続元の端末のノード。遠隔のセッションの起点である。 */
export const clientTerminalNodeId = "n:terminal:9c41";

/** 遠隔ログインのレコードに現れたアカウントのノード。 */
export const accountNodeId = "n:account:4a9d";

/** 接続元の端末。遠隔のセッションの起点として応答に入る。 */
function clientTerminalNode() {
  return {
    id: clientTerminalNodeId,
    kind: "terminal",
    keyForm: "terminal_id",
    identity: [{ semantic: "terminal.id", value: "HOST-F-TMID" }],
    label: { rawText: "HOST-F", valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
  };
}

/** 遠隔ログインのレコードに現れたアカウント。 */
function accountNode() {
  return {
    id: accountNodeId,
    kind: "account",
    keyForm: "account_domain_name",
    identity: [
      { semantic: "account.domain", value: "EXAMPLE" },
      { semantic: "account.name", value: "user02" },
    ],
    label: { rawText: "user02", valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
  };
}

export function remoteSessionObservationKind(category: string, action: string) {
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

/** 遠隔のセッションを記録したレコード 1 件。区分ごとに観測の種別が分かれる。 */
function remoteSessionEvidence(
  sequenceNumber: number,
  lineNumber: number,
  category: string,
  action: string,
) {
  return {
    recordRef: markiiRecordRef(sequenceNumber, lineNumber),
    eventTime: markiiEventTime(
      "2031/10/08 10:20:35.100",
      "2031-10-08T10:20:35.100+09:00",
    ),
    observationKind: remoteSessionObservationKind(category, action),
  };
}

/** 接続先 port 5985 の区分が含むレコード 1 件。 */
function port5985Evidence() {
  return remoteSessionEvidence(121, 1031, "net", "acpt");
}

function destinationPortField(rawText: string) {
  return {
    name: "dstPort",
    semantic: "connection.destination_port",
    kind: "text",
    text: { rawText, valueState: "present" },
  };
}

/** 区分のレコードがログオンの種別を持たないことを述べる理由。 */
export const logonTypeAbsentReason =
  "種別の値が無い。区分のレコードはログオンの種別の値を持ちません";

/**
 * 操作 11 の応答。1 本の遠隔のセッションが、用いた手段の異なる根拠を含む。
 * 接続先 port 5985 の acpt、445 の acpt、接続先 port を持たない遠隔ログインの 3 区分で
 * ある。遠隔ログインの区分のレコードだけにアカウントが現れる。どの区分もログオンの種別を持たない。
 */
export function edgeDetailResponseJson() {
  return {
    edge: {
      id: remoteSessionEdgeId,
      kind: "terminal_remote_session",
      state: "candidate",
      sourceNodeId: clientTerminalNodeId,
      targetNodeId: terminalNodeId,
      applicableRange: {
        from: markiiEventTime(
          "2031/10/08 10:20:35.100",
          "2031-10-08T10:20:35.100+09:00",
        ),
        to: markiiEventTime(
          "2031/10/08 11:05:48.500",
          "2031-10-08T11:05:48.500+09:00",
        ),
      },
      // 区分 4 件が 1 件ずつ含むレコードの全数である。
      evidence: [
        port5985Evidence(),
        remoteSessionEvidence(122, 1032, "net", "acpt"),
        remoteSessionEvidence(123, 1033, "session", "loginR"),
        remoteSessionEvidence(124, 1034, "net", "acpt"),
      ],
      evidenceCount: 4,
    },
    sourceNode: clientTerminalNode(),
    targetNode: terminalNode("matched"),
    matchRecords: [],
    matchStages: [],
    matches: [],
    matchCount: 0,
    evidenceGroups: [
      {
        observationKind: remoteSessionObservationKind("net", "acpt"),
        destinationPort: destinationPortField("5985"),
        logonTypeAbsence: logonTypeAbsentReason,
        selector: {
          eventCategory: "net",
          eventAction: "acpt",
          destinationPort: "5985",
          logonTypeAbsent: true,
        },
        accounts: [],
        authentications: [],
        evidenceCount: 1,
      },
      {
        observationKind: remoteSessionObservationKind("net", "acpt"),
        destinationPort: destinationPortField("445"),
        logonTypeAbsence: logonTypeAbsentReason,
        selector: {
          eventCategory: "net",
          eventAction: "acpt",
          destinationPort: "445",
          logonTypeAbsent: true,
        },
        accounts: [],
        authentications: [],
        evidenceCount: 1,
      },
      {
        observationKind: remoteSessionObservationKind("session", "loginR"),
        destinationPortAbsence:
          "区分のレコードが接続先 port の欄を持っていません",
        logonTypeAbsence: logonTypeAbsentReason,
        selector: {
          eventCategory: "session",
          eventAction: "loginR",
          destinationPortAbsent: true,
          logonTypeAbsent: true,
        },
        accounts: [{ node: accountNode(), evidenceCount: 1 }],
        authentications: [
          {
            value: {
              name: "AuthenticationPackageName",
              semantic: "event.authentication_package",
              kind: "text",
              text: { rawText: "NTLM", valueState: "present" },
            },
            evidenceCount: 1,
          },
        ],
        evidenceCount: 1,
      },
      {
        // 接続先 port の欄の値を比べられない区分。要求で指せない。
        observationKind: remoteSessionObservationKind("net", "acpt"),
        destinationPortAbsence:
          "区分のレコードの接続先 port の値を比べられません",
        logonTypeAbsence: logonTypeAbsentReason,
        selectorAbsence:
          "区分のレコードの接続先 port の値を比べられないため、要求で指せません",
        accounts: [],
        authentications: [],
        evidenceCount: 1,
      },
    ],
  };
}

/** 接続先 port 5985 の区分に絞った応答。区分の一覧は全数のままである。 */
export function edgeDetailOfPort5985Json() {
  const whole = edgeDetailResponseJson();
  return {
    ...whole,
    edge: {
      ...whole.edge,
      evidence: [port5985Evidence()],
      evidenceCount: 1,
    },
  };
}

/**
 * 関連付けを経て成立した候補の関係の詳細。
 *
 * **確度の材料を持つ。** 段階 1 が挙げた候補は 3 件、段階 2 が挙げた候補は 2 件で、その
 * 2 件が互いに区別できない。
 * 条件 1 件だけを読んで候補を確定と読ませないための fixture である。
 */
export function matchedEdgeDetailResponseJson() {
  const base = edgeDetailResponseJson();
  return {
    ...base,
    edge: {
      ...base.edge,
      kind: "cross_source_connection_match",
      state: "candidate",
    },
    // レコードは起点、候補、候補と区別できないレコードの順に置く。
    matchRecords: [
      {
        ref: markiiRecordRef(112, 1022),
        eventTime: markiiEventTime(
          "2031/10/08 10:20:35.100",
          "2031-10-08T10:20:35.100+09:00",
        ),
      },
      {
        ref: markiiRecordRef(115, 1025),
        eventTime: markiiEventTime(
          "2031/10/08 10:20:35.900",
          "2031-10-08T10:20:35.900+09:00",
        ),
      },
      { ref: markiiRecordRef(116, 1026) },
    ],
    matches: [{ stage: 0, candidate: 1, unresolvedReasons: 0 }],
    matchStages: [
      {
        origin: 0,
        stageKey: "second_time_matched",
        conditions: [
          {
            conditionKey: "destination_ip",
            use: "used",
            leftValue: [
              {
                name: "dstIP",
                semantic: "connection.destination_address",
                kind: "text",
                text: { rawText: "203.0.113.21", valueState: "present" },
              },
            ],
            rightValue: [
              {
                name: "dstIP",
                semantic: "connection.destination_address",
                kind: "text",
                text: { rawText: "203.0.113.21", valueState: "present" },
              },
            ],
          },
        ],
        assumptions: [
          {
            assumptionKey: "clock_offset_below_one_second",
            evidenceClass: "measured",
            statement: "2 つの収集元の時計のずれは 1 秒未満である",
          },
        ],
        timeWindow: {
          windowKind: "same_second",
          comparisonUnit: "second",
          centerTime: {
            requestText: "2031-10-08T10:20:35+09:00",
            precision: "second",
            offsetState: "in_value",
            normalized: "2031-10-08T10:20:35+09:00",
            normalizedForm: "rfc3339_absolute",
            derivation: "起点のレコードの事象の時刻を秒に切り捨てた",
          },
        },
        comparisonUnit: "second",
        clockDependencyNote:
          "段階 1 の terminal_ip_assignment が収集元の時計に依拠する",
        stageTallies: [
          {
            stageKey: "clock_independent",
            memberCount: 3,
            distinctProcessCount: 3,
          },
          {
            stageKey: "second_time_matched",
            memberCount: 2,
            distinctProcessCount: 2,
          },
        ],
        indistinguishableGroups: [[1, 2]],
        unresolvedReasonSets: [
          ["段階が用いた条件だけでは候補が 1 件に定まらない"],
        ],
      },
    ],
    matchCount: 1,
    assignmentBases: [
      {
        clientIp: "192.0.2.11",
        sourceId: "source-assignment",
        conditions: [
          {
            conditionKey: "terminal_ip_assignment",
            use: "used",
            leftValue: [
              {
                name: "clientIP",
                semantic: "connection.source_address",
                kind: "text",
                text: { rawText: "192.0.2.11", valueState: "present" },
              },
            ],
            rightValue: [
              {
                name: "clientTerminal",
                semantic: "terminal.id",
                kind: "text",
                text: {
                  normalized: terminalIdText,
                  derivation: "srcIP=192.0.2.11; rule=terminal_ip_assignment",
                  valueState: "derived",
                },
              },
            ],
            assignmentValidRange: {
              from: markiiEventTime(
                "2031/10/08 09:40:09.600",
                "2031-10-08T09:40:09.600+09:00",
              ),
              to: markiiEventTime(
                "2031/10/08 11:05:53.700",
                "2031-10-08T11:05:53.700+09:00",
              ),
            },
            outsideAssignmentRange: false,
          },
        ],
        assumptions: [
          {
            assumptionKey: "ip_assignment_holds_in_observation_gap",
            evidenceClass: "inferred",
            statement: "観測の切れ目でも IP の割当が続いている",
          },
        ],
        clockDependencyNote: "割当の期間は収集元の時計が刻んだ時刻で決まる",
      },
    ],
  };
}

/**
 * 起点の違う段階 2 つと関連付けの結果 3 件を持つ関係の詳細。関連付けは段階 0、段階 1、段階 0 の順に並ぶ。
 *
 * レコードは `matchedEdgeDetailResponseJson` の 3 件に、2 つ目の起点 (sn=113) と
 * 候補 2 件 (sn=117、sn=118) を足す。
 */
export function twoStageEdgeDetailResponseJson() {
  const base = matchedEdgeDetailResponseJson();
  const recordAt = (sn: number, line: number, time: string) => ({
    ref: markiiRecordRef(sn, line),
    eventTime: markiiEventTime(
      `2031/10/08 ${time}`,
      `2031-10-08T${time}+09:00`,
    ),
  });
  return {
    ...base,
    matchRecords: [
      ...base.matchRecords,
      recordAt(113, 1023, "10:20:36.300"),
      recordAt(117, 1027, "10:20:36.700"),
      recordAt(118, 1028, "10:20:35.400"),
    ],
    matchStages: [base.matchStages[0], { ...base.matchStages[0], origin: 3 }],
    matches: [
      { stage: 0, candidate: 1, unresolvedReasons: 0 },
      { stage: 1, candidate: 4, unresolvedReasons: 0 },
      { stage: 0, candidate: 5, unresolvedReasons: 0 },
    ],
    matchCount: 3,
  };
}

/**
 * 根拠のレコードを count 件含む関係の詳細。通番は 920000 から、行番号は 3000 から振る。
 * 一覧が見えている行だけを描くことを確かめるのに使う。
 */
export function manyEvidenceEdgeDetailResponseJson(count: number) {
  const base = edgeDetailResponseJson();
  const evidence = Array.from({ length: count }, (_, index) =>
    remoteSessionEvidence(920000 + index, 3000 + index, "net", "acpt"),
  );
  return {
    ...base,
    edge: { ...base.edge, evidence, evidenceCount: evidence.length },
  };
}

/**
 * 関連付けの段階を count 個含む関係の詳細。段階 i の起点は通番 930000 + i のレコードで、どの段階も
 * `matchedEdgeDetailResponseJson` の候補 (位置 1) を 1 件挙げる。
 */
export function manyStageEdgeDetailResponseJson(count: number) {
  const base = matchedEdgeDetailResponseJson();
  const origins = Array.from({ length: count }, (_, index) => ({
    ref: markiiRecordRef(930000 + index, 5000 + index),
    eventTime: markiiEventTime(
      "2031/10/08 10:20:36.000",
      "2031-10-08T10:20:36.000+09:00",
    ),
  }));
  const firstOrigin = base.matchRecords.length;
  return {
    ...base,
    matchRecords: [...base.matchRecords, ...origins],
    matchStages: origins.map((_, index) => ({
      ...base.matchStages[0],
      origin: firstOrigin + index,
    })),
    matches: origins.map((_, index) => ({
      stage: index,
      candidate: 1,
      unresolvedReasons: 0,
    })),
    matchCount: origins.length,
  };
}

/**
 * 関連付けの結果 1 件を含みながら、`matchCount` が 2 件を名乗る関係の詳細。
 * 一覧に出ていない関連付けがあることを、総数と要素数の食い違いが表す。
 */
export function matchCountMismatchEdgeDetailResponseJson() {
  return { ...matchedEdgeDetailResponseJson(), matchCount: 2 };
}

/**
 * 根拠のレコード 1 件を含みながら、`evidenceCount` が 4 件を名乗る関係の詳細。
 * エッジの中で、総数と要素数が食い違う。
 */
export function evidenceCountMismatchEdgeDetailResponseJson() {
  const whole = edgeDetailResponseJson();
  return {
    ...whole,
    edge: { ...whole.edge, evidence: [port5985Evidence()] },
  };
}

/**
 * レコードのノードの詳細。関係を導いた結果を含む。
 *
 * **時刻の一致を足して候補が 0 件になった起点である。** 原資料について言える事実と、
 * 本ツールが処理できなかったことを分析者が読み分ける材料になる。
 */
export function recordNodeDetailResponseJson() {
  const base = nodeDetailResponseJson();
  return {
    ...base,
    node: recordNode("matched"),
    relationDerivation: {
      outcome: "no_candidate_in_window",
      basis: "source_fact",
    },
  };
}

import type { EventKind } from "@/shared/contracts/eventKinds";
import type {
  EdgeDirection,
  GraphEmptyReason,
  GraphNode,
  NodeCreationRecord,
  NodeKeyForm,
  NodeKind,
  NodeObservation,
  NodeSelection,
  RelationState,
  ValueCountsEmptyReason,
  ValueMatchForm,
} from "@/shared/contracts/graph";
import type {
  LogonSessionRejectionReason,
  RelationDerivationBasis,
  RelationDerivationOutcome,
} from "@/shared/contracts/graphDetail";
import type { InvestigationOrderMethod } from "@/shared/contracts/investigationOrder";
import { filterFieldLabels, nodeKindLabels } from "@/shared/lib/graphLabels";

/** 画面で選べる調べる順序の目安の手法。先頭が既定の手法である。 */
export const shownInvestigationOrderMethods = [
  "degree",
  "sigma",
] as const satisfies readonly InvestigationOrderMethod[];
export type ShownInvestigationOrderMethod =
  (typeof shownInvestigationOrderMethods)[number];

/** 調べる順序の目安の手法の表示名。 */
export const investigationOrderMethodLabels: Record<
  ShownInvestigationOrderMethod,
  string
> = {
  degree: "隣接ノードの数",
  sigma: "Sigma ルールの一致",
};

/** 調べる順序の目安の手法が、何の値でどう並べるか。手法の tooltip に「名前: 値」の組で出す。 */
export const investigationOrderMethodDescriptions: Record<
  ShownInvestigationOrderMethod,
  readonly (readonly [string, string])[]
> = {
  degree: [
    ["順位の基準", "エッジか同じレコードで結ばれたノードの数"],
    ["順", "多い順"],
  ],
  sigma: [
    ["順位の基準", "一致した Sigma ルールの最高のレベルと一致したレコード数"],
    ["順", "高い順"],
    ["ルールの適用先", "Windows イベントログのレコード"],
  ],
};

export {
  edgeKindDescriptions,
  edgeKindLabels,
  filterFieldLabels,
  nodeKindLabels,
  timeUnitLabels,
} from "@/shared/lib/graphLabels";

/**
 * ノード 1 つの種別の表示。名前が `$` で終わるアカウントは、名前の形を観察した事実として
 * 表示に添える。名前は識別鍵の `account.name` と表示名から読む。
 */
export function nodeKindLabelOf(
  node: Pick<GraphNode, "kind" | "identity" | "label">,
): string {
  if (node.kind !== "account") {
    return nodeKindLabels[node.kind];
  }
  const names = [
    ...node.identity
      .filter((value) => value.semantic === "account.name")
      .map((value) => value.value),
    node.label.normalized ?? node.label.rawText ?? "",
  ];
  return names.some((name) => name.endsWith("$"))
    ? "$ で終わる名前のアカウント"
    : nodeKindLabels.account;
}

/**
 * Logon ID が一致したログオンと操作をエッジにしなかった理由の表示。
 * 文言は `backend/core/logon_session.go` の `LogonSessionRejectionReason` の各値の doc
 * コメントに対応する。
 */
export const logonSessionRejectionReasonLabels: Record<
  LogonSessionRejectionReason,
  string
> = {
  other_terminal: "別の端末のログオン",
  other_source: "別の収集元のログオン",
  logon_after_operation: "操作より後のログオン",
  time_not_comparable: "前後を比べられない時刻",
};

/** Logon ID が一致したログオンと操作をエッジにしなかった理由の補足。理由の tooltip に出す。 */
export const logonSessionRejectionReasonDetails: Record<
  LogonSessionRejectionReason,
  string
> = {
  other_terminal: "Logon ID が一意な範囲: 1 台の端末",
  other_source: "Logon ID を比べる範囲: 1 つの収集元",
  logon_after_operation: "ログオンの時刻が操作の時刻より後",
  time_not_comparable: "読み取れない時刻か、タイムゾーンの有無が異なる時刻の組",
};

/**
 * 対象を特定する基準の表示。複数のレコードが同じ値を持てば、同じ対象として 1 つの
 * ノードにまとめる。
 */
export const nodeKeyFormLabels: Record<NodeKeyForm, string> = {
  terminal_id: "端末の ID",
  terminal_id_process_id: "端末の ID とプロセスの ID",
  terminal_id_file_path: "端末の ID とファイルの path",
  terminal_id_registry_value_key_path: "端末の ID とレジストリキーの path",
  account_sid: "アカウントの SID",
  account_domain_name: "ドメインとログイン名",
  address: "IP アドレス",
  hostname: "接続先のホスト名",
  source_content_sha256_position: "収集元の file の内容とレコードの位置",
  terminal_id_process_pid_interval:
    "端末の ID と PID と、その PID が使われ始めた時刻",
  recording_source_content_sha256: "端末が記録した収集元の file の内容",
  recording_source_content_sha256_hostname: "収集元の file の内容とホスト名",
  terminal_id_account_name: "端末の ID とログイン名",
  terminal_id_address: "端末の ID と IP アドレス",
  collection_content_sha256: "端末 1 台から集めた収集の file の内容",
};

/**
 * 生成の記録を根拠に持たないプロセスの区間の、対象を特定する基準の表示。区間の時刻は、その PID を
 * 最初に記録したレコードの時刻であり、プロセスが起動した時刻と限らない。
 */
const pidIntervalWithoutCreationLabel =
  "端末の ID と PID と、その PID を最初に記録した時刻";

/** ノードの対象を特定する基準の表示。PID の区間は生成の記録の有無で文言を分ける。 */
export function nodeKeyFormLabelOf(
  node: Pick<GraphNode, "keyForm" | "creationRecord">,
): string {
  if (
    node.keyForm === "terminal_id_process_pid_interval" &&
    node.creationRecord !== "present"
  ) {
    return pidIntervalWithoutCreationLabel;
  }
  return nodeKeyFormLabels[node.keyForm];
}

/** 応答がノードを含む理由の表示。 */
export const nodeSelectionLabels: Record<NodeSelection, string> = {
  matched: "一致ノード",
  edge_endpoint: "エッジの端のノード",
};

/**
 * 参照だけのノード (observation が referenced) の成り立ちを、名前と値の組で書く。画面は
 * `nodeReferencedOnlyMark` の印の tooltip に出す。
 *
 * 作成レコードの有無は別の軸であり、`nodeCreationRecordLabels` が持つ。作成レコードを
 * 持つノードには `referencedCreationFact` を足す。作成レコードは、このノードを指したレコードの
 * 1 つである。
 */
export const referencedOnlyFacts: readonly (readonly [string, string])[] = [
  ["ノードの元", "別のノードのレコードが指した SID・名前・プロセス番号"],
  ["このノードを記録したレコード", "なし"],
];

/** 参照だけのノードが作成レコードを持つときに、`referencedOnlyFacts` に足す組。 */
export const referencedCreationFact: readonly [string, string] = [
  "作成レコード",
  "このノードを指したレコードの中",
];

/** 参照だけで分かったノードに付ける短い印。 */
export const nodeReferencedOnlyMark = "参照だけ";

/**
 * ノードの根拠のレコードの表の見出し。参照だけで分かったノードの根拠は、ノードを
 * 参照したレコードである。
 */
export const nodeEvidenceTitleLabels: Record<NodeObservation, string> = {
  observed: "ノードを記録したレコード",
  referenced: "このノードを参照したレコード",
};

/**
 * 作成レコードの有無の表示。
 * 作成レコードを持つプロセスだけが、コマンド行と起動時刻と親のプロセスを求められる。
 */
export const nodeCreationRecordLabels: Record<NodeCreationRecord, string> = {
  present: "あり",
  absent: "なし",
  item_absent: "対象外の種類",
};

/** ノードから見たエッジの向きの表示。選んだノードが始点か終点か。 */
export const edgeDirectionLabels: Record<EdgeDirection, string> = {
  outgoing: "始点",
  incoming: "終点",
};

/** エッジの作り方の表示。 */
export const relationStateLabels: Record<RelationState, string> = {
  observed: "観測",
  // 由来を文字列に含めない。推定は、案件を横断するエッジの推定が挙げたものと、同じ収集元の
  // 時刻の前後から挙げたものの両方を指す。由来はエッジの種類が持つ。
  candidate: "推定",
  uncertain_chain: "未確定の推定",
};

/** エッジの作り方の補足。値の tooltip に出す。 */
export const relationStateDescriptions: Record<RelationState, string> = {
  observed: "レコードから直接作ったエッジ",
  candidate: "推定したエッジ",
  uncertain_chain:
    "同じ終点に 2 つ以上の候補があり原因を 1 つに決められないエッジ",
};

/**
 * `nodeCount` が 0 になった理由の表示。一致ノードの件数 0 の横に出す。
 */
export const graphEmptyReasonLabels: Record<GraphEmptyReason, string> = {
  no_record_in_filter: "フィルタに一致するレコードなし",
  no_value_match: "読み取れたフィールドに文字列なし",
  value_match_outside_filter: "他のフィルタで除外",
  no_field_observed: "読み取れたレコードにフィールド名なし",
  record_match_without_object: "端末のほかのノードを指すレコードなし",
};

/** 0 件の理由ごとの次の操作。理由の tooltip に出す。 */
export const graphEmptyReasonNextActions: Partial<
  Record<GraphEmptyReason, string>
> = {
  no_field_observed: "フィールド名の確認",
  record_match_without_object: "ノードの種類にレコードを追加",
};

/**
 * 完全一致の条件だけで文字列を照合した応答の、0 件の理由の表示。値の一部を含むかで比べた
 * 理由と読み違えないよう、値の全体で比べたことを書く。
 */
const exactMatchEmptyReasonLabels: Partial<Record<GraphEmptyReason, string>> = {
  no_value_match: "読み取れたフィールドに全体が等しい値なし",
};

/** 0 件の応答の理由を、応答が用いた文字列の条件に合わせて書く。 */
export function describeGraphEmptyReason(response: {
  emptyReason: GraphEmptyReason;
  valueContains?: readonly string[];
  fieldContains?: readonly string[];
  fieldEquals?: readonly string[];
  searchExpression?: string;
}): string {
  const exactOnly =
    (response.fieldEquals?.length ?? 0) > 0 &&
    (response.valueContains?.length ?? 0) === 0 &&
    (response.fieldContains?.length ?? 0) === 0 &&
    response.searchExpression === undefined;
  return (
    (exactOnly
      ? exactMatchEmptyReasonLabels[response.emptyReason]
      : undefined) ?? graphEmptyReasonLabels[response.emptyReason]
  );
}

/**
 * 値ごとの件数が 0 件になった理由の表示。
 * 欄の名前が無い・値が無い・対象の種別が欄を持たない・絞り込みで残らない、を分ける。
 */
export const valueCountsEmptyReasonLabels: Record<
  ValueCountsEmptyReason,
  string
> = {
  no_field_observed: "読み取れたレコードにフィールド名なし",
  no_readable_value: "値を読み取れるレコードなし",
  field_on_other_node_kind: "表示中のノードの種類に値なし",
  no_value_in_filter: "フィルタで除外",
};

/** 同じ file 名の収集元に添える sourceId の先頭の文字数。 */
const sourceIdPrefixLength = 12;

/**
 * 収集元の sourceId から、画面で収集元を区別できる表示名を探す表を組む。
 * **同じ file 名の収集元が 2 件以上あるときは、file 名に sourceId の先頭を添える。**
 * 各端末から集めた同じ名前のログを取り込むと、file 名だけでは選んだ収集元を確かめられない。
 */
export function distinctSourceLabels(
  fileNames: ReadonlyMap<string, string>,
): ReadonlyMap<string, string> {
  const counts = new Map<string, number>();
  for (const fileName of fileNames.values()) {
    counts.set(fileName, (counts.get(fileName) ?? 0) + 1);
  }
  return new Map(
    [...fileNames].map(([id, fileName]) => [
      id,
      (counts.get(fileName) ?? 0) > 1
        ? `${fileName} ${id.slice(0, sourceIdPrefixLength)}`
        : fileName,
    ]),
  );
}

/**
 * 件数を集計するフィールドの値を持つノードの種類を並べる。
 * `valueCountsEmptyReason` が `field_on_other_node_kind` のときに「値を持つノードの種類」の値に出す。
 */
export function valueCountsNodeKinds(kinds: readonly NodeKind[]): string {
  return kinds.map((kind) => nodeKindLabels[kind]).join("、");
}

/** 事象の種別の条件の 2 つの欄の名前。 */
export type EventKindFieldLabels = {
  eventCategory: string;
  eventAction: string;
};

/**
 * 事象の種別の条件の欄の名前を返す。
 *
 * 条件と一致する組がすべて Windows イベントログの組であるときは、分類をプロバイダ、動作を
 * イベント ID と書く。一致する組が一覧に無いときは判定できないため、分類と動作の名前にする。
 */
export function eventKindFieldLabels(
  kinds: readonly EventKind[],
  category: string | undefined,
  action: string | undefined,
): EventKindFieldLabels {
  const matching = kinds.filter(
    (kind) =>
      kind.category === category &&
      (action === undefined || kind.action === action),
  );
  return matching.length > 0 && matching.every((kind) => kind.windowsEvent)
    ? { eventCategory: "プロバイダ", eventAction: "イベント ID" }
    : {
        eventCategory: filterFieldLabels.eventCategory,
        eventAction: filterFieldLabels.eventAction,
      };
}

/** 検索が一致した値の形の表示。 */
export const valueMatchFormLabels: Record<ValueMatchForm, string> = {
  raw_text: "原文",
  normalized: "正規化した値",
};

/**
 * ノードの種別ごとの描画の色を持つ、画面の配色 (theme.css) の変数の名前。
 * 色は種別をグラフの中で見分けるために置く。意味の順序を表さない。
 */
export const nodeKindColorTokens: Record<NodeKind, string> = {
  terminal: "--kind-terminal",
  process: "--kind-process",
  file: "--kind-file",
  registry_value: "--kind-registry_value",
  account: "--kind-account",
  ip: "--kind-ip",
  domain: "--kind-domain",
  record: "--kind-record",
};

/** ノードの種別ごとの描画の色。CSS の値として使い、画面の配色に従う。 */
export const nodeKindColors = Object.fromEntries(
  Object.entries(nodeKindColorTokens).map(([kind, token]) => [
    kind,
    `var(${token})`,
  ]),
) as Record<NodeKind, string>;

/**
 * 接続先の推定の結果の分類の表示。分類が述べるのは、接続先の推定
 * (`cross_source_connection_match`) の元になったかだけであり、ほかのエッジを述べない。
 *
 * **推定の結果が 0 件であることと、推定できなかったことを別の値で書く。** 混ぜると、
 * 分析者は原資料について言える事実と Oraculum が処理できなかったことを見分けられない。
 */
export const relationDerivationOutcomeLabels: Record<
  RelationDerivationOutcome,
  string
> = {
  not_used_as_origin: "推定の元に不使用",
  matched: "推定したエッジあり",
  destination_ip_absent: "接続先の IP アドレスとホスト名なし",
  destination_port_absent: "接続先の port なし",
  no_candidate_record: "接続先の文字列が一致するレコードなし",
  candidate_item_unreadable: "候補のレコードに比べられる値なし",
  terminal_undetermined: "IP アドレスの端末が 1 つに決まらない",
  terminal_id_absent: "端末の割り当てに端末 ID なし",
  proxy_address_unknown: "Proxy の IP アドレスが不明",
  origin_item_unreadable: "比べられる値か時刻なし",
  candidate_set_failed: "候補の計算に失敗",
  no_candidate_matching_conditions: "時刻を使わない条件で候補なし",
  no_candidate_in_window: "時刻の一致で候補なし",
  node_unresolved: "候補のノードがグラフに無い",
  source_declaration_conflict: "収集元の宣言が矛盾",
  no_compared_condition: "この収集元に使える推定条件なし",
};

/** 接続先の推定の結果ごとの次の操作。結果の tooltip に出す。 */
export const relationDerivationNextActions: Partial<
  Record<RelationDerivationOutcome, string>
> = {
  terminal_id_absent: "端末の割り当てに端末 ID を追加",
  proxy_address_unknown:
    "Proxy のログの収集元に IP アドレスの端末の割り当てを追加",
};

/**
 * 分類が何について言えることかの表示。
 * **分析者が次に採る手はこの区分で違う。**
 */
export const relationDerivationBasisLabels: Record<
  RelationDerivationBasis,
  string
> = {
  source_fact: "原資料の事実",
  input_item_absent: "レコードにフィールドなし",
  source_declaration_defect: "収集元の宣言の欠陥",
  internal_gap: "Oraculum の未対応",
  analyst_selection: "推定条件か端末の割り当て",
  not_applicable: "該当なし",
};

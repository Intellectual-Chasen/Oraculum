import type { EdgeKind, FilterUnit, NodeKind } from "../contracts/graph";

/** 期間の比較単位の表示。 */
export const timeUnitLabels: Record<FilterUnit, string> = {
  second: "秒単位",
  millisecond: "ミリ秒単位",
  microsecond: "マイクロ秒単位",
};

/** ノードの種類の表示。 */
export const nodeKindLabels: Record<NodeKind, string> = {
  terminal: "端末",
  process: "プロセス",
  file: "ファイル",
  registry_value: "レジストリの値",
  account: "アカウント",
  ip: "IP アドレス",
  domain: "ホスト名",
  record: "レコード",
};

/**
 * エッジの種類の短いラベル。凡例・chip・一覧の値に使う。イベントの種類をエッジの種類へ
 * 読み替えた言葉を使わない。推定で作る種類は語尾を「候補」にする。
 */
export const edgeKindLabels: Record<EdgeKind, string> = {
  ran_on: "端末で実行したプロセス",
  process_parent_child: "親子のプロセス",
  process_injection: "コードインジェクション",
  file_operation: "ファイルの操作",
  file_copy: "ファイルのコピー",
  process_executable: "プロセスの実行ファイル",
  registry_operation: "レジストリの操作",
  process_communication: "プロセスの通信",
  terminal_address: "端末の IP アドレス",
  terminal_remote_session: "リモートセッションの候補",
  terminal_account: "端末のアカウント",
  http_request: "HTTP の要求",
  file_content_match: "同じ内容のファイルの候補",
  cross_source_connection_match: "収集元をまたぐプロセスの対応の候補",
  record_names_object: "レコードに現れたノード",
  record_subject_account: "操作したアカウント",
  record_target_account: "操作の対象のアカウント",
  logon_session_operation: "ログオンセッションの操作の候補",
  argument_names_object: "引数に現れたノードの候補",
  task_registration_run: "タスクの登録と実行の候補",
  linked_logon: "対のログオンの候補",
  ticket_request_logon: "Kerberos のチケットとログオンの候補",
  reverse_lookup_name: "逆引きの名前の候補",
  connection_logon_match: "接続とログオンの候補",
  process_identity_match: "同じプロセスの候補",
  explicit_credential_logon: "資格情報を指定したログオンの候補",
  unidentified_source_remote_session: "接続元が不明のリモートセッションの候補",
  terminal_outbound_connection: "端末から始めた接続",
  account_identity_match: "同じアカウントの候補",
  inbound_connection_match: "着信の接続の候補",
  same_connection_match: "同じ接続の候補",
  requested_session_logon: "要求したセッションのログオンの候補",
  logon_chain: "ログオンの連鎖の候補",
};

/** エッジの種類の補足。ラベルの tooltip に出す。補足を持たない種類は項目を持たない。 */
export const edgeKindDescriptions: Partial<Record<EdgeKind, string>> = {
  reverse_lookup_name: "名前の出どころ: アドレスの持ち主が決めた逆引きの名前",
  connection_logon_match: "結ぶ条件: 接続元のアドレス・port と時刻の一致",
  explicit_credential_logon:
    "結ぶ条件: アカウントの名前・分析者の割当で導いた接続先の端末・時刻の一致",
  unidentified_source_remote_session: "起点: 接続元の IP アドレス",
  terminal_outbound_connection:
    "起点: 記録した端末\n終点: 記録が接続先に書いた IP アドレス\n接続の試行の記録を含む",
  inbound_connection_match:
    "接続元の端末: 分析者の割当で導いた端末\nport: 問わない",
  same_connection_match:
    "結ぶ条件: 接続元のアドレス・port、宛先の port と時刻の一致",
  requested_session_logon:
    "結ぶ条件: 要求の記録が持つセッションの Logon ID、アカウントの名前、時刻の範囲",
  logon_chain: "結ぶ条件: 分析者の割当で導いた接続元の端末とセッションの期間",
};

/**
 * 自由に書く絞り込みの入力欄の名前。
 * 入力欄の label と、拒んだ入力欄を示すラベルと、応答が使った条件のラベルが同じ文字列を使う。
 */
export const filterFieldLabels = {
  valueContains: "含む文字列",
  valueExcludes: "含まない文字列",
  fieldContains: "フィールドと文字列",
  fieldEquals: "フィールドと文字列の完全一致",
  searchExpression: "検索式",
  terminal: "Host",
  sources: "Artifact",
  countBy: "件数を数えるフィールド",
  addressInCidr: "CIDR の内のアドレス",
  addressNotInCidr: "CIDR の外のアドレス",
  // 応答が返す observationKind と別の語にする。要求の 2 つは原文を完全一致で比べる条件で
  // あり、応答の observationKind はレコードが記録した意味である。
  eventCategory: "イベントの分類",
  eventAction: "イベントの動作",
  caseId: "案件",
} as const;

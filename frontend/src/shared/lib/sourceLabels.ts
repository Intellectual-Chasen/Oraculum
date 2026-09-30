import type {
  DiagnosisClass,
  ImportCategory,
  PublicationState,
} from "../contracts/sources";

/** 公開の状態の表示。 */
export const publicationStateLabels: Record<PublicationState, string> = {
  published_full: "全体を公開",
  published_partial: "一部を公開",
  withheld: "公開を停止",
};

/** 既知の入力形式の表示。 */
const formatKeyLabels: Record<string, string> = {
  infotrace_mark_ii: "InfoTrace Mark II",
  squid_combined: "Squid combined",
  squid_combined_request_bytes: "Squid combined + 要求の byte 数",
  squid_logformat: "Squid logformat",
  apache_access_combined: "Apache access (combined)",
  apache_error: "Apache error",
  linux_auditd: "Linux auditd",
  royalts_rds_connection: "Royal TS の RDP 接続",
  windows_event_xml: "Windows イベントログの XML",
  windows_event_viewer_csv: "Windows イベントビューアーの CSV",
  windows_evtx: "Windows イベントログ (EVTX)",
  windows_prefetch: "Windows Prefetch",
  windows_registry_hive: "Windows レジストリ hive",
};

/** 入力形式の表示名を返す。表示名を持たない形式は識別子を返す。 */
export function formatKeyLabel(formatKey: string): string {
  return Object.hasOwn(formatKeyLabels, formatKey)
    ? (formatKeyLabels[formatKey] ?? formatKey)
    : formatKey;
}

/** 取り込み結果の区分の表示。 */
export const importCategoryLabels: Record<ImportCategory, string> = {
  read: "読み込み",
  succeeded: "成功",
  failed: "失敗",
};

/** 読めなかった理由の分類の表示。 */
export const diagnosisClassLabels: Record<DiagnosisClass, string> = {
  undetermined: "未判定",
  unsupported_format: "未対応の形式",
  inconsistent_input_confirmed: "入力の不整合",
  implementation_defect_confirmed: "実装の不具合",
};

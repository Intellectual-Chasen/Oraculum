import type {
  FailureStage,
  LineEnding,
  SkippedFileReason,
  SourcesEmptyReason,
  WithheldReason,
} from "@/shared/contracts/sources";
import type { KeyValuePair } from "@/shared/ui/KeyValueList";

/** 収集の directory のファイルを取り込まなかった理由の表示。 */
export const skippedFileReasonLabels: Record<SkippedFileReason, string> = {
  unsupported_format: "未対応の形式",
  empty_file: "0 byte のファイル",
  not_regular_file: "特殊なファイル",
  companion_without_main: "hive の無い transaction log",
};

/** 失敗した処理の段階の表示。 */
export const failureStageLabels: Record<FailureStage, string> = {
  read: "読み込み",
  tokenize: "文字列の分割",
  field_map: "フィールドの対応付け",
  normalize: "値の正規化",
  relate: "ノードとエッジの作成",
};

/** 公開を止めた理由の表示。 */
export const withheldReasonLabels: Record<WithheldReason, string> = {
  identifier_collision: "識別子の衝突",
  dangling_evidence_reference: "解決できない根拠の参照",
  mixed_analysis_run: "異なる解析の実行の混在",
};

const proxyRecordingScope: KeyValuePair[] = [
  { name: "記録する要求", value: "Proxy を経由した要求だけ" },
  { name: "記録の範囲の外", value: "Proxy を経由しない通信" },
];
const proxyFormatKeys: ReadonlySet<string> = new Set([
  "squid_combined",
  "squid_combined_request_bytes",
  "squid_logformat",
]);

/** 入力形式が Proxy のログであるかを返す。 */
export function isProxyFormat(formatKey: string): boolean {
  return proxyFormatKeys.has(formatKey);
}

const windowsEventRecordingScope: KeyValuePair[] = [
  {
    name: "記録するイベントの種類",
    value: "記録した端末の監査の設定で変わる",
  },
];

/**
 * 入力形式が記録する範囲の、形式ごとの一般的な説明の値の組。個々の収集元を見て判定した結果
 * ではない。help の吹き出しにだけ出す。説明を持たない形式は出さない。
 */
export const formatRecordingScopeNotes: Record<string, KeyValuePair[]> = {
  squid_combined: proxyRecordingScope,
  squid_combined_request_bytes: proxyRecordingScope,
  squid_logformat: proxyRecordingScope,
  windows_event_xml: windowsEventRecordingScope,
  windows_evtx: windowsEventRecordingScope,
  windows_event_viewer_csv: [
    ...windowsEventRecordingScope,
    {
      name: "イベントビューアーの書き出し",
      value: "イベントの種類のフィルタを適用した書き出しあり",
    },
  ],
  windows_prefetch: [
    {
      name: "記録する値",
      value: "実行ファイルごとの最近 8 回までの実行時刻・実行回数",
    },
    {
      name: "記録の範囲の外",
      value: "Prefetch を無効にした端末の実行・古い記録を消した後の実行",
    },
  ],
  windows_registry_hive: [
    {
      name: "取り込む値",
      value:
        "hive の root の key からたどれる key と値・key ごとの最終更新の時刻",
    },
    {
      name: "取り込まない値",
      value:
        "解放した領域に残る削除した key・key の class 名・security descriptor",
    },
  ],
};

/** 行末の byte 列の表示。 */
export const lineEndingLabels: Record<LineEnding, string> = {
  crlf: "CRLF",
  lf: "LF",
  mixed: "CRLF と LF の混在",
  undetermined: "判定できない",
};

/** `sourceCount` が 0 になった理由の表示。 */
export const sourcesEmptyReasonLabels: Record<SourcesEmptyReason, string> = {
  no_source_ingested: "取り込んだ収集元なし",
};

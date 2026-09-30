import type { SourceFileUndetectedReason } from "@/shared/contracts/sourceFiles";
import type {
  ProcessingStep,
  StageFailureReason,
  StageState,
} from "@/shared/contracts/stages";

/** file の入力形式の候補が無い理由の表示。 */
export const undetectedReasonLabels: Record<
  SourceFileUndetectedReason,
  string
> = {
  unsupported_format: "形式を判定できない",
  empty_file: "空の file",
  unreadable: "読めない file",
  companion_file: "主 file と一緒に読む",
};

/**
 * 選べない項目 (基準の外を指す link、特殊な file、読み込みの要求に渡せない名前の項目、再帰の一覧で
 * 辿らない symbolic link の directory と中を読めない directory) の表示。
 */
export const otherEntryLabel = "選べない項目";

/** 先頭の byte 列から分かった file の種類の表示。表示名を持たない種類は識別子を返す。 */
export function detectedKindLabel(kind: string): string {
  switch (kind) {
    case "text":
      return "テキスト";
    case "zip":
      return "ZIP";
    case "sqlite":
      return "SQLite";
    case "ese":
      return "ESE database";
    default:
      return kind;
  }
}

/** 段階が失敗した理由の種別の表示。 */
export const stageFailureReasonLabels: Record<StageFailureReason, string> = {
  source_import_failed: "収集元の読み取りか取り込みの失敗",
  import_failed: "取り込みの準備の失敗",
  recording_failed: "調査への記録の失敗",
  graph_build_failed: "グラフの構築の失敗",
  interrupted: "サーバーの停止による中断",
};

/** 段階と手順の状態の表示。 */
export const stageStateLabels: Record<StageState, string> = {
  not_started: "未開始",
  running: "実行中",
  completed: "完了",
  failed: "失敗",
};

/** 処理の手順の表示。並びは `processingSteps` の実行の順である。 */
export const processingStepLabels: Record<ProcessingStep, string> = {
  observed_layer: "レコードからのノードとエッジの作成",
  candidate_edges: "収集元をまたぐエッジの推定",
};

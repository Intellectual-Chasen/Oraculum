import type {
  AssumptionKey,
  ComparisonUnit,
  EvidenceClass,
  StageKey,
  WindowKind,
} from "../contracts/candidates";
import type { MatchStageEmptyReason } from "../contracts/graphDetail";

/** 推定の段階の表示。 */
export const stageKeyLabels: Record<StageKey, string> = {
  clock_independent: "段階 1: 時刻を使わない条件",
  second_time_matched: "段階 2: 同じ秒の時刻",
};

/** 候補が 0 件になった理由の表示。 */
export const matchStageEmptyReasonLabels: Record<
  MatchStageEmptyReason,
  string
> = {
  no_record_in_filter: "条件に一致するレコードなし",
  no_source_ingested: "収集元なし",
  no_candidate_in_window: "時刻の範囲に候補なし",
  no_candidate_matching_conditions: "推定条件に一致する候補なし",
  origin_outside_assignment_range: "基準の時刻が端末の割り当ての適用期間外",
  publication_withheld: "収集元の公開の停止",
  counterpart_item_absent: "候補にフィールドなし",
};

/** 時刻の範囲の種類の表示。 */
export const windowKindLabels: Record<WindowKind, string> = {
  not_compared: "時刻の比較なし",
  same_second: "同じ秒",
  second_range: "秒の範囲",
  symmetric_seconds: "中心 ± 秒",
};

/** 時刻を比べた単位の表示。 */
export const comparisonUnitLabels: Record<ComparisonUnit, string> = {
  second: "秒",
  not_compared: "時刻の比較なし",
};

/** 前提の種類の表示。 */
export const assumptionKeyLabels: Record<AssumptionKey, string> = {
  clock_offset_below_one_second: "端末時刻の差が 1 秒未満",
  ip_assignment_holds_in_observation_gap:
    "記録の無い期間も IP の割り当てが継続",
  counterpart_connected_to_proxy: "端末の接続先が Proxy",
};

/** 前提の根拠の種類の表示。値の意味は `backend/core/assumption.go` の `EvidenceClass` が持つ。 */
export const evidenceClassLabels: Record<EvidenceClass, string> = {
  official: "公式説明",
  measured: "実機確認",
  inferred: "推測",
  unconfirmed: "未確認",
};

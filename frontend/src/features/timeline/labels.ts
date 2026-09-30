import type { CoverageState } from "@/shared/contracts/timeline";
import type { TimelineScaleKey } from "./timelineGrouping";

/**
 * 記録期間と指定した期間の重なりの表示。
 *
 * **記録が無い状態と、イベントが無い状態を別の文字列で書く。** 分析者はこの 2 つを混ぜると、
 * 収集元が記録していなかった区間を「何も起きていなかった区間」と読む。
 */
export const coverageStateLabels: Record<CoverageState, string> = {
  covered: "記録期間内",
  partially_covered: "一部が記録期間外",
  outside_recording: "記録期間外",
  recording_range_unknown: "記録期間不明",
  range_not_requested: "期間の指定なし",
  undetermined: "判定不能",
};

/** 記録期間と指定した期間の重なりの tooltip。ラベルだけで意味が定まる状態は持たない。 */
export const coverageStateDescriptions: Partial<Record<CoverageState, string>> =
  {
    partially_covered: "指定した期間の一部が記録期間の外",
    outside_recording: "指定した期間に記録なし",
    recording_range_unknown: "UTC 時刻を持つレコードなし",
    undetermined: "記録期間の端か指定した期間の端を UTC 時刻にできない",
  };

/** 時系列の表の列名。 */
export const timelineColumnLabels = {
  eventTime: "時刻",
  terminal: "端末",
  account: "アカウント",
  observationKind: "イベントの種類",
  clock: "時刻の種類",
  record: "レコード",
} as const;

/** 表示の段階の名前。区切りの段階は区切りの幅で呼ぶ。 */
export const timelineScaleLabels: Record<TimelineScaleKey, string> = {
  day: "1 日",
  hour: "1 時間",
  ten_minutes: "10 分",
  minute: "1 分",
  ten_seconds: "10 秒",
  second: "1 秒",
  record: "1 件ずつ",
};

/** 時刻の精度が示す範囲が 2 つ以上の区切りにまたがるレコードの行と件数の名前。 */
export const coarserThanBucketLabel = "精度が区切りより粗い";

/** 並べる時刻を画面で解釈できないレコードの行と件数の名前。 */
export const timeUnreadableLabel = "時刻を解釈できない";

/** 区切りの段階の表の列名。 */
export const timelineGroupColumnLabels = {
  period: "UTC の期間",
  count: "件数",
  sources: "収集元ごとの件数",
} as const;

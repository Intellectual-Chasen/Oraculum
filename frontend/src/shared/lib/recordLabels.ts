import type {
  ObservationStatus,
  PositionKind,
  TimestampClock,
  TimestampPrecision,
} from "../contracts/common";

/**
 * 時刻の出どころの表示。
 *
 * **出どころを機器と読み替えない。** どの機器の時刻かは収集元の識別子と組にして初めて
 * 定まる。分類だけでは、別の端末が記録した 2 つの時刻が同じ出どころに見える。
 */
export const timestampClockLabels: Record<TimestampClock, string> = {
  terminal_local: "端末時刻",
  observer_local: "観測した機器の時刻",
  file_property: "ファイルの属性の時刻",
  undetermined: "出どころ不明",
};

/** 時刻の精度の表示。 */
export const timestampPrecisionLabels: Record<TimestampPrecision, string> = {
  year: "年",
  month: "月",
  day: "日",
  hour: "時",
  minute: "分",
  second: "秒",
  millisecond: "ミリ秒",
  microsecond: "マイクロ秒",
};

/**
 * 精度が秒より粗いか。表は秒より粗い精度だけを画面の文字で出し、ほかの精度は title に入れる。
 * 秒より粗い時刻は、同じ文字列でも時点の幅が広く、並びを読み違えやすい。
 */
export function isCoarserThanSecond(precision: TimestampPrecision): boolean {
  switch (precision) {
    case "year":
    case "month":
    case "day":
    case "hour":
    case "minute":
      return true;
    default:
      return false;
  }
}

/** 位置の指し方の表示。`recordPositionParts` の組の名前と同じ語を使う。 */
export const positionKindLabels: Record<PositionKind, string> = {
  line_number: "行",
  sequence_number: "ID",
  byte_range: "位置",
};

/** イベントの種類の意味の状態の表示。「意味」の名前と組にして出す。 */
export const observationStatusLabels: Record<ObservationStatus, string> = {
  determined: "確定",
  inferred: "推定",
  undetermined: "不明",
};

import {
  DecodeFailure,
  type Decoder,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireCount,
} from "./decoding";
import { decodeGraphNode, type GraphNode } from "./graph";

/** 件数の分布の 1 行。定義元は `backend/api/time_histogram.go` の `timeHistogramRow` である。 */
export type TimeHistogramRow = {
  /** 端末を特定できないレコードの行では出ない。 */
  terminal?: GraphNode;
  counts: number[];
};

/** 件数の分布の応答。定義元は `timeHistogramResponse` である。 */
export type TimeHistogramResponse = {
  /** 最初の区切りの始まりの UTC の時刻。時刻を比べられるレコードが無いときは出ない。 */
  start?: string;
  stepMs: number;
  rows: TimeHistogramRow[];
  /** UTC からのずれの決まらない地方時のレコードの件数。時点を持たず、区切りに入らない。 */
  localTimeRecordCount: number;
  /** 時刻を持たないレコードの件数。 */
  undatedRecordCount: number;
  /** 時点を持つが、時刻の精度の範囲が 1 つの区切りに収まらず、区切りに入れなかったレコードの件数。 */
  spanningRecordCount: number;
};

const decodeRow: Decoder<TimeHistogramRow> = (input, path) => {
  const source = readObject(input, path);
  return {
    terminal: optionalMember(source, "terminal", path, decodeGraphNode),
    counts: requireArray(source, "counts", path, (item, at) => {
      if (typeof item !== "number" || !Number.isInteger(item) || item < 0) {
        throw new DecodeFailure(at, "expected a count");
      }
      return item;
    }),
  };
};

/** `TimeHistogramResponse` を検証する。行があるときは始まりの時刻と 1 以上の幅を持つ。 */
export const decodeTimeHistogramResponse: Decoder<TimeHistogramResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const rows = requireArray(source, "rows", path, decodeRow);
  const start = optionalString(source, "start", path);
  const stepMs = requireCount(source, "stepMs", path);
  if (
    rows.length > 0 &&
    (start === undefined || Number.isNaN(Date.parse(start)) || stepMs < 1)
  ) {
    throw new DecodeFailure(path, "expected a start and a step with rows");
  }
  if (rows.some((row) => row.counts.length !== rows[0]?.counts.length)) {
    throw new DecodeFailure(
      `${path}.rows`,
      "expected the same columns in every row",
    );
  }
  return {
    start,
    stepMs,
    rows,
    localTimeRecordCount: requireCount(source, "localTimeRecordCount", path),
    undatedRecordCount: requireCount(source, "undatedRecordCount", path),
    spanningRecordCount: requireCount(source, "spanningRecordCount", path),
  };
};

import {
  DecodeFailure,
  type Decoder,
  optionalEnum,
  readObject,
  requireArray,
  requireEnum,
  requireString,
} from "./decoding";
import {
  decodeValueCount,
  type EdgeDirection,
  type EdgeKind,
  edgeDirections,
  edgeKinds,
  type ValueCount,
  type ValueCountsEmptyReason,
  valueCountsEmptyReasons,
} from "./graph";

/**
 * 関係の相手側の欄の値ごとの件数 (`GET /api/v0/nodes/{id}/value-counts`) の応答。
 * 定義元は `backend/api/node_value_counts.go` の `nodeValueCountsResponse` である。
 */
export type NodeValueCountsResponse = {
  nodeId: string;
  edgeKind: EdgeKind;
  direction: EdgeDirection;
  countBy: string;
  valueCounts: ValueCount[];
  /** 数える欄をグラフのどこにも観測していないときだけ出る。 */
  emptyReason?: ValueCountsEmptyReason;
};

/** 関係の相手側の欄の値ごとの件数の応答を検証する。 */
export const decodeNodeValueCountsResponse: Decoder<NodeValueCountsResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const valueCounts = requireArray(
    source,
    "valueCounts",
    path,
    decodeValueCount,
  );
  const emptyReason = optionalEnum(
    source,
    "emptyReason",
    path,
    valueCountsEmptyReasons,
  );
  if (emptyReason !== undefined && valueCounts.length > 0) {
    throw new DecodeFailure(`${path}.emptyReason`, "expected no value counts");
  }
  return {
    nodeId: requireString(source, "nodeId", path),
    edgeKind: requireEnum(source, "edgeKind", path, edgeKinds),
    direction: requireEnum(source, "direction", path, edgeDirections),
    countBy: requireString(source, "countBy", path),
    valueCounts,
    ...(emptyReason === undefined ? {} : { emptyReason }),
  };
};

import type { RecordLocator } from "./common";
import { decodeRecordLocator } from "./common";
import {
  type Decoder,
  decodeString,
  readObject,
  requireArray,
  requireMember,
} from "./decoding";

/**
 * 1 レコードを根拠に持つノードとエッジ (`GET /api/v0/record-graph`)。
 * 定義元は `backend/api/record_graph.go` の `recordGraphResponse` である。
 */
export type RecordGraphResponse = {
  recordRef: RecordLocator;
  /** そのレコードを根拠に持つノードと、`edgeIds` のエッジの端点。 */
  nodeIds: string[];
  /** そのレコードを根拠に持つエッジ。 */
  edgeIds: string[];
};

/** `RecordGraphResponse` を検証する。 */
export const decodeRecordGraphResponse: Decoder<RecordGraphResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    recordRef: requireMember(source, "recordRef", path, decodeRecordLocator),
    nodeIds: requireArray(source, "nodeIds", path, decodeString),
    edgeIds: requireArray(source, "edgeIds", path, decodeString),
  };
};

import { decodeTimestamp, type Timestamp } from "./common";
import {
  DecodeFailure,
  type Decoder,
  optionalMember,
  readObject,
  requireArray,
  requireCount,
  requireMember,
  requireString,
} from "./decoding";
import { decodeGraphNode, type GraphNode } from "./graph";

/**
 * 1 つのノードと、それに接するレコードの件数と時刻の範囲。
 * 定義元は `backend/api/node_summaries.go` の `nodeSummaryItem` である。
 */
export type NodeSummary = {
  node: GraphNode;
  /**
   * 端末の範囲に置いたアドレス (`keyForm` が `terminal_id_address`) の、範囲の端末のノード。
   * 範囲を持たないノードと、範囲の端末のノードがグラフに無いときは出ない。
   */
  terminal?: GraphNode;
  recordCount: number;
  /** 時刻を比べられるレコードが無いときは出ない。 */
  firstTime?: Timestamp;
  lastTime?: Timestamp;
  /** 数えたレコードのうち UTC からのずれの決まらない地方時のもの。時刻の範囲に入らない。 */
  localTimeRecordCount: number;
  /** 数えたレコードのうち時刻を持たないもの。時刻の範囲に入らない。 */
  undatedRecordCount: number;
};

/** ノードの一覧の応答。定義元は `nodeSummariesResponse` である。 */
export type NodeSummariesResponse = {
  nodes: NodeSummary[];
  nodeCount: number;
  nodeKind: string;
};

const decodeNodeSummary: Decoder<NodeSummary> = (input, path) => {
  const source = readObject(input, path);
  const summary = {
    node: requireMember(source, "node", path, decodeGraphNode),
    terminal: optionalMember(source, "terminal", path, decodeGraphNode),
    recordCount: requireCount(source, "recordCount", path),
    firstTime: optionalMember(source, "firstTime", path, decodeTimestamp),
    lastTime: optionalMember(source, "lastTime", path, decodeTimestamp),
    localTimeRecordCount: requireCount(source, "localTimeRecordCount", path),
    undatedRecordCount: requireCount(source, "undatedRecordCount", path),
  };
  if (
    summary.localTimeRecordCount + summary.undatedRecordCount >
    summary.recordCount
  ) {
    throw new DecodeFailure(
      path,
      "expected the records outside the time range within the record count",
    );
  }
  return summary;
};

/** `NodeSummariesResponse` を検証する。 */
export const decodeNodeSummariesResponse: Decoder<NodeSummariesResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const nodes = requireArray(source, "nodes", path, decodeNodeSummary);
  const nodeCount = requireCount(source, "nodeCount", path);
  if (nodeCount !== nodes.length) {
    throw new DecodeFailure(
      `${path}.nodeCount`,
      "expected the number of nodes",
    );
  }
  return {
    nodes,
    nodeCount,
    nodeKind: requireString(source, "nodeKind", path),
  };
};

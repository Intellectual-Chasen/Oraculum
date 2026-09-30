import {
  graphResponseJson,
  ipNodeId,
  nodeDetailResponseJson,
  processNodeId,
  terminalNodeId,
} from "./graphResponse";

/** 図の応答が持つノードの項目から、選び方の欄を除いたもの。 */
export function graphNodeJson(id: string) {
  const found = graphResponseJson().nodes.find((node) => node.id === id);
  if (found === undefined) throw new Error("fixture lacks the node");
  const { selection, ...node } = found;
  void selection;
  return node;
}

/**
 * 起点から終点への影響のエッジ 1 本の応答。backend と同じく、起点と終点は経路の要素を指し、
 * frontier は経路に乗らないノードの項目を持つ。
 */
export function influencePathResponseJson() {
  const evidence = nodeDetailResponseJson().evidence;
  return {
    from: processNodeId,
    to: ipNodeId,
    excludedBases: [
      "account_management",
      "argument_name",
      "requested_destination",
    ],
    origin: { key: "v:p", record: evidence[0] },
    destination: { key: "v:t", record: evidence[1] ?? evidence[0] },
    vertices: [
      { key: "v:p", node: graphNodeJson(processNodeId), influenceEdgeCount: 1 },
      { key: "v:t", node: graphNodeJson(ipNodeId), influenceEdgeCount: 1 },
    ],
    edges: [
      {
        id: "s:1",
        sourceKey: "v:p",
        targetKey: "v:t",
        graphEdgeId: "e:ran_on:0001",
        graphEdgeKind: "ran_on",
        bases: ["specified_operation", "observed", "single_node"],
        timeBases: ["same_terminal"],
        evidence: [evidence[0]],
      },
    ],
    omittedEdgeCount: 0,
    stops: [] as string[],
    routeExcludedBases: [] as string[],
    frontier: [
      {
        key: "v:f0",
        node: graphNodeJson(terminalNodeId),
        influenceEdgeCount: 0,
        reason: "no_outgoing_influence",
      },
    ],
    frontierCount: 1,
    untimedRecordCount: 3,
  };
}

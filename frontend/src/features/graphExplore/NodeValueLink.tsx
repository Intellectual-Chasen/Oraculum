import type { GraphEdge, GraphNode } from "@/shared/contracts/graph";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { DerivedLabelNote } from "@/shared/ui/DerivedLabelNote";
import { ValueLink } from "@/shared/ui/ValueLink";
import {
  edgeKindDescriptions,
  edgeKindLabels,
  nodeKindLabelOf,
} from "./labels";
import { NodeLabelValue, nodeLabelName } from "./NodeLabelView";
import { readNodeLabel } from "./subgraph";

type LinkedNode = Pick<GraphNode, "id" | "kind" | "identity" | "label">;

/**
 * ノードの表示名を、操作できる表の値として出す。click で Node Detail に表示してグラフで選び、
 * hover で種類と識別の値を出す。導き方の印は値の外に置き、操作の名前を表示名の値だけにする。
 */
export function NodeValueLink({ node }: { node: LinkedNode }) {
  const label = readNodeLabel(node.label);
  return (
    <>
      <ValueLink
        target={{ kind: "node", id: node.id, label: nodeLabelName(label) }}
        hover={
          <>
            <span className="block">{nodeKindLabelOf(node)}</span>
            {node.identity.map((value) => (
              <span
                key={JSON.stringify([value.semantic ?? null, value.value])}
                className="block"
              >
                {toVisibleRawText(value.value)}
              </span>
            ))}
          </>
        }
      >
        <NodeLabelValue label={label} />
      </ValueLink>
      {label.valueState === "derived" ? (
        <>
          {" "}
          <DerivedLabelNote derivation={label.derivation} />
        </>
      ) : null}
    </>
  );
}

/**
 * エッジの種類を、操作できる表の値として出す。click で Edge Detail に表示し、hover でエッジの
 * 種類と始点と終点を出す。
 */
export function EdgeValueLink({
  edge,
  nodeOf,
}: {
  edge: GraphEdge;
  /** 端点のノードを探す。応答に無い端点は識別子を出す。 */
  nodeOf: (nodeId: string) => LinkedNode | undefined;
}) {
  const endName = (nodeId: string) => {
    const node = nodeOf(nodeId);
    return node === undefined
      ? nodeId
      : nodeLabelName(readNodeLabel(node.label));
  };
  return (
    <ValueLink
      target={{ kind: "edge", id: edge.id }}
      hover={
        <>
          <span className="block">{edgeKindLabels[edge.kind]}</span>
          {edgeKindDescriptions[edge.kind] === undefined ? null : (
            <span className="block whitespace-pre-line">
              {edgeKindDescriptions[edge.kind]}
            </span>
          )}
          <span className="block">{`始点: ${endName(edge.sourceNodeId)}`}</span>
          <span className="block">{`終点: ${endName(edge.targetNodeId)}`}</span>
        </>
      }
    >
      {edgeKindLabels[edge.kind]}
    </ValueLink>
  );
}

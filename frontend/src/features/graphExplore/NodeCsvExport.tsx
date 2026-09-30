import { Download } from "lucide-react";
import { useState } from "react";
import { fetchGraph } from "@/shared/api/graph";
import type { SubgraphNode } from "@/shared/contracts/graph";
import { downloadCsv, toCsv } from "@/shared/lib/csv";
import { formatCount } from "@/shared/lib/format";
import { describeRecordPosition } from "@/shared/lib/recordPosition";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { nodeKindLabelOf } from "./labels";
import { graphRequestOf, type SubgraphCriteria } from "./useSubgraph";

/** 書き出しの状態。 */
type ExportState =
  | { status: "idle" }
  | { status: "running" }
  | { status: "done"; nodeCount: number; guardedCount: number }
  | { status: "failed" };

const header = [
  "表示名",
  "種類",
  "同一性の基準",
  "同一性の値",
  "一致したフィールド",
  "時刻の原文",
  "時刻の正規化した値",
  "イベント ID かイベントの動作",
  "チャネル",
  "Provider かイベントの分類",
  "収集元",
  "収集元の sourceId",
  "位置",
  "RecordHeader.RecordID",
  "EventRecordID",
];

/**
 * ノード 1 つを CSV の行にする。原資料の文字列は、画面の無害化を通さずに書く。
 * レコードの欄は、レコードのノードだけが埋める。
 */
function nodeRow(node: SubgraphNode): string[] {
  const record = node.record;
  return [
    node.label.rawText ?? node.label.normalized ?? "",
    nodeKindLabelOf(node),
    node.keyForm,
    node.identity.map((value) => value.value).join(" / "),
    (node.valueMatches ?? [])
      .map((match) => match.semantic ?? match.name ?? "")
      .join(" / "),
    record?.eventTime?.rawText ?? "",
    record?.eventTime?.normalized ?? "",
    record?.eventAction ?? "",
    record?.channel ?? "",
    record?.eventCategory ?? "",
    record?.recordRef.sourceFileName ?? "",
    record?.recordRef.sourceId ?? "",
    record === undefined ? "" : describeRecordPosition(record.recordRef),
    record?.recordHeaderId ?? "",
    record?.eventRecordId ?? "",
  ];
}

/**
 * 今の検索の条件に合ったノードを、CSV に書き出す。
 *
 * **図に描いたノードではなく、条件に合ったノードの全件を書き出す。** 描画の上限を
 * 外し、ホップ数 0 で同じ条件をもう 1 度要求する。関係の相手として図に出たノードは書かない。
 * CSV は押したときだけ組む。
 */
export function NodeCsvExport({ criteria }: { criteria: SubgraphCriteria }) {
  const [state, setState] = useState<ExportState>({ status: "idle" });
  const exportNodes = async () => {
    setState({ status: "running" });
    try {
      const result = await fetchGraph({
        ...graphRequestOf(criteria),
        nodeLimit: undefined,
        depth: 0,
        recordSummary: true,
      });
      if (!result.ok) {
        setState({ status: "failed" });
        return;
      }
      const matched = result.value.nodes.filter(
        (node) => node.selection === "matched",
      );
      const { text, guardedCount } = toCsv(header, matched.map(nodeRow));
      const stamp = new Date()
        .toISOString()
        .replaceAll(/[-:]/g, "")
        .slice(0, 15);
      downloadCsv(`oraculum-nodes-${stamp}Z.csv`, text);
      setState({ status: "done", nodeCount: matched.length, guardedCount });
    } catch {
      setState({ status: "failed" });
    }
  };
  return (
    <div className="csv-export flex items-center gap-2">
      <IconButton
        label="一致ノードを CSV に書き出す"
        isDisabled={state.status === "running"}
        disabledReason={{ title: "書き出し中", text: "書き出しの終了後" }}
        onPress={() => void exportNodes()}
      >
        <Download size={14} aria-hidden="true" />
      </IconButton>
      {state.status === "done" ? (
        <div role="status">
          <KeyValueList
            pairs={[
              { name: "書き出したノード", value: formatCount(state.nodeCount) },
              {
                name: "先頭に ' を付けた値",
                value:
                  state.guardedCount > 0 ? (
                    <Hint text="表計算の数式として解釈される値">
                      {formatCount(state.guardedCount)}
                    </Hint>
                  ) : undefined,
              },
            ]}
          />
        </div>
      ) : null}
      {state.status === "failed" ? (
        <div role="alert">
          <StatusLabel status="failed" label="ノードの書き出しに失敗" />
        </div>
      ) : null}
    </div>
  );
}

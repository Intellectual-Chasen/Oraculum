import { useId } from "react";
import { maxGraphDepth, minGraphDepth, type NodeRef } from "@/shared/api/graph";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";

/** 時系列をグラフで選んだノードの近くに絞るかと、辿るホップ数。 */
export type TimelineNodeScopeValue = { enabled: boolean; depth: number };

/** 既定では、選んだノードから 1 ホップのレコードに絞る。 */
export const defaultTimelineNodeScope: TimelineNodeScopeValue = {
  enabled: true,
  depth: 1,
};

const depthChoices = Array.from(
  { length: maxGraphDepth - minGraphDepth + 1 },
  (_, offset) => minGraphDepth + offset,
);

/**
 * 時系列をグラフで選んだノードから何ホップのレコードに絞るかの欄。ノードを選んでいない間は
 * 絞らないことを出す。
 */
export function TimelineNodeScope({
  node,
  value,
  onChange,
}: {
  node: NodeRef | undefined;
  value: TimelineNodeScopeValue;
  onChange: (value: TimelineNodeScopeValue) => void;
}) {
  const depthId = useId();
  return (
    <fieldset className="m-0 min-w-0 border-0 p-0">
      <legend className="sr-only">ノードのフィルタ</legend>
      <label>
        <input
          type="checkbox"
          checked={value.enabled}
          onChange={(event) =>
            onChange({ ...value, enabled: event.target.checked })
          }
        />
        Graph の選択ノードでフィルタ
      </label>
      <HelpPopover label="Graph の選択ノードでフィルタ">
        <KeyValueList
          stacked
          pairs={[
            {
              name: "対象",
              value: "選択ノードからホップ数までのノードとエッジのレコード",
            },
            {
              name: "適用する検索の条件",
              value: "期間・イベントの種類・案件・端末・収集元・検索式",
            },
          ]}
        />
      </HelpPopover>{" "}
      <label htmlFor={depthId}>ホップ数</label>{" "}
      <select
        id={depthId}
        value={value.depth}
        disabled={!value.enabled}
        onChange={(event) =>
          onChange({ ...value, depth: Number(event.target.value) })
        }
      >
        {depthChoices.map((depth) => (
          <option key={depth} value={depth}>
            {depth}
          </option>
        ))}
      </select>
      {value.enabled ? (
        <KeyValueList
          className="note"
          pairs={[
            {
              name: "ノード",
              value:
                node === undefined ? "未選択" : <RawText text={node.label} />,
            },
          ]}
        />
      ) : null}
    </fieldset>
  );
}

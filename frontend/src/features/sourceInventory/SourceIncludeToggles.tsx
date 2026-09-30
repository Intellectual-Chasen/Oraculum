import type { SourceRef } from "@/shared/api/graph";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { RawText } from "@/shared/ui/RawText";

/**
 * 取り込んだ収集元ごとに、グラフと時系列の結果に含めるかを切り替える。
 *
 * **選んだ収集元が空のときは、すべてを含める。** 全件に印を付けた状態も空へ戻し、
 * 要求に収集元を載せない。
 */
export function SourceIncludeToggles({
  sources,
  included,
  onChange,
}: {
  sources: readonly SourceIdentity[];
  /** 結果に含める収集元。空はすべてを含めることを表す。 */
  included: readonly SourceRef[];
  onChange: (included: SourceRef[]) => void;
}) {
  if (sources.length === 0) return null;
  const includes = (source: SourceIdentity) =>
    included.length === 0 || included.some((ref) => ref.id === source.sourceId);
  const toggle = (source: SourceIdentity, checked: boolean) => {
    const next = sources
      .filter((candidate) =>
        candidate === source ? checked : includes(candidate),
      )
      .map((candidate) => ({
        id: candidate.sourceId,
        label: candidate.fileName,
      }));
    onChange(next.length === sources.length ? [] : next);
  };
  return (
    <fieldset className="source-include m-0 min-w-0 border-0 p-0">
      <legend className="p-0 font-medium">結果に含める収集元</legend>
      {sources.map((source) => (
        <label key={source.sourceId}>
          <input
            type="checkbox"
            checked={includes(source)}
            // 最後の 1 件は外せない。空はすべてを含めることを表す。
            disabled={
              included.length === 1 && included[0]?.id === source.sourceId
            }
            onChange={(event) => toggle(source, event.target.checked)}
          />
          <RawText text={source.fileName} />
        </label>
      ))}
    </fieldset>
  );
}

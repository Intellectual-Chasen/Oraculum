import { ChevronsDown } from "lucide-react";
import { useState } from "react";
import { formatCount } from "@/shared/lib/format";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";

/** 一覧を開いたときと、続きを表示する操作 1 回で追加する行の数。 */
const rowPageSize = 1000;

/**
 * 一覧が表示する行の数を持つ。応答の全件を受け取った後も、数万行を一度に描くと画面が止まるため、
 * 先頭から順に表示する。
 */
export function useShownRows<T>(rows: readonly T[]) {
  const [shown, setShown] = useState(rowPageSize);
  return {
    visible: rows.length > shown ? rows.slice(0, shown) : rows,
    shown: Math.min(shown, rows.length),
    total: rows.length,
    showMore: () => setShown((count) => count + rowPageSize),
  };
}

/** 表示している行の数と全件数の組と、続きを表示する button。全件を表示したら出さない。 */
export function ShowMoreRows({
  shown,
  total,
  showMore,
}: {
  shown: number;
  total: number;
  showMore: () => void;
}) {
  if (shown >= total) {
    return null;
  }
  return (
    <div className="flex items-center gap-2">
      <KeyValueList
        pairs={[
          {
            name: "表示",
            value: `${formatCount(shown)} / ${formatCount(total)}`,
          },
        ]}
      />
      <IconButton label="続きを表示" onPress={showMore}>
        <ChevronsDown size={14} aria-hidden="true" />
      </IconButton>
    </div>
  );
}

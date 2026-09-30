import type { ReactNode } from "react";
import type { RecordLocator } from "@/shared/contracts/common";
import type { ValueCondition } from "@/shared/lib/valueCondition";
import { ValueActionsContext, type ValueTarget } from "@/shared/ui/ValueLink";

/**
 * 表の値の操作を、test が渡す受け手へつなぐ provider。レコードを開く操作は `onSelectRecord`、
 * 条件に追加する操作は `onAddCondition`、ほかの値は `onOpen` へ渡す。
 */
export function ValueActionsForTest({
  onSelectRecord = () => {},
  onAddCondition = () => {},
  onOpen = () => {},
  children,
}: {
  onSelectRecord?: (recordRef: RecordLocator) => void;
  onAddCondition?: (condition: ValueCondition) => void;
  onOpen?: (target: ValueTarget) => void;
  children: ReactNode;
}) {
  return (
    <ValueActionsContext
      value={{
        open: (target) =>
          target.kind === "record"
            ? onSelectRecord(target.ref)
            : onOpen(target),
        addCondition: onAddCondition,
      }}
    >
      {children}
    </ValueActionsContext>
  );
}

import { type ReactNode, useMemo } from "react";
import { toVisibleRawText } from "../lib/rawText";
import {
  conditionLabel,
  type ValueCondition,
  type ValueCopy,
} from "../lib/valueCondition";
import { type ContextMenuTriggers, useContextMenu } from "./ContextMenu";
import { Hint } from "./Hint";
import { type MenuEntry, menuSeparator } from "./Menu";
import { useCopyText } from "./useCopyText";
import { useValueActions } from "./ValueLink";

/**
 * 値のメニューの対象。`key` は表の中で値 1 つを指し、開いているメニューの値の button を見分ける。
 * `conditions` は値から足せる検索の条件、`copies` は写す文字列である。
 */
export type ValueMenuTarget = {
  key: string;
  /** 値の名前 (フィールドの名前、列の名前)。メニューの名前と hover に出す。 */
  name: string;
  /** 値の文字列。メニューの名前に出す。 */
  text: string;
  conditions: readonly ValueCondition[];
  copies: readonly ValueCopy[];
};

/** 値を押したときの操作。表 1 つが useValueMenu から組み、行へ渡す。 */
export type ValueMenuActions = {
  triggers: ContextMenuTriggers<ValueMenuTarget>;
  openKey: string | undefined;
};

/** メニューの名前。値の名前と、制御文字を符号にした値の文字列である。 */
export function valueMenuLabel(target: ValueMenuTarget): string {
  return `値の操作: ${target.name} ${toVisibleRawText(target.text)}`;
}

/**
 * 表の値を押したときに開く、検索の条件に追加する項目と写す項目のメニュー。表 1 つが 1 つ持ち、
 * 行へ actions を渡す。条件の追加は表の値の操作 (ValueActions) の `addCondition` を使い、
 * provider の無い画面では値を押せない文字列で出す。
 */
export function useValueMenu(): {
  /** 値を押したときの操作。provider の無い画面では出ない。 */
  actions: ValueMenuActions | undefined;
  menu: ReactNode;
} {
  const addCondition = useValueActions()?.addCondition;
  const { copy, notice } = useCopyText();
  const { triggers, openTarget, menu } = useContextMenu(
    (target: ValueMenuTarget) => {
      const conditions: MenuEntry[] = target.conditions.map(
        (condition, index) => ({
          kind: "item",
          key: `condition-${index}`,
          label: conditionLabel(condition),
          onSelect: () => addCondition?.(condition),
        }),
      );
      const copies: MenuEntry[] = target.copies.map((item, index) => ({
        kind: "item",
        key: `copy-${index}`,
        label: `${item.what}をコピー`,
        onSelect: () => copy(item.text, item.what),
      }));
      return {
        label: valueMenuLabel(target),
        entries:
          conditions.length === 0 || copies.length === 0
            ? [...conditions, ...copies]
            : [...conditions, menuSeparator("copy"), ...copies],
      };
    },
  );
  const enabled = addCondition !== undefined;
  const openKey = openTarget?.key;
  // 行は memo であり、開いている値が変わらない限り同じ組を渡す。
  const actions = useMemo(
    () => (enabled ? { triggers, openKey } : undefined),
    [enabled, triggers, openKey],
  );
  return {
    actions,
    menu: (
      <>
        {menu}
        {notice}
      </>
    ),
  };
}

/**
 * 値 1 つを、押すと値のメニューを開く button で出す。右クリックと Shift+F10 でも同じメニューを開き、
 * 行のメニューへは渡さない。hover で値の名前を出す。
 * **行の操作のメニューの button (RowMenuButton) と同じく、押し直すと閉じ、開いているかを
 * aria-expanded で示す。**
 */
export function ValueMenuButton({
  target,
  actions,
  hover = target.name,
  className,
  children,
}: {
  target: ValueMenuTarget;
  actions: ValueMenuActions | undefined;
  /** hover で出す文字列。既定は値の名前である。 */
  hover?: string;
  className?: string;
  children: ReactNode;
}) {
  if (actions === undefined) {
    return (
      <Hint text={hover} className={className}>
        {children}
      </Hint>
    );
  }
  return (
    <button
      type="button"
      className={
        className === undefined ? "value-link" : `value-link ${className}`
      }
      aria-label={valueMenuLabel(target)}
      aria-haspopup="menu"
      aria-expanded={actions.openKey === target.key}
      title={hover}
      onClick={(event) => actions.triggers.onButtonClick(event, target)}
      onContextMenu={(event) => {
        event.stopPropagation();
        actions.triggers.onContextMenu(event, target);
      }}
      onKeyDown={(event) => actions.triggers.onKeyDown(event, target)}
    >
      {children}
    </button>
  );
}

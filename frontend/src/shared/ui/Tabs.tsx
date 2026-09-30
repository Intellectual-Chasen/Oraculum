import { type ReactNode, useState } from "react";
import {
  Tabs as AriaTabs,
  type Key,
  Tab,
  TabList,
  TabPanel,
} from "react-aria-components";
import { tv } from "tailwind-variants";

const tab = tv({
  base: [
    "cursor-default whitespace-nowrap border-b-2 border-transparent px-2 py-1 text-sm text-muted outline-none",
    "data-[hovered]:text-ink data-[selected]:border-accent data-[selected]:text-ink",
    "data-[focus-visible]:outline-2 data-[focus-visible]:outline-accent",
  ],
});

/** tab 1 つ。`label` は短いラベル、`content` は tab を選んだときに出す中身。 */
export type TabItem = { id: string; label: ReactNode; content: ReactNode };

/**
 * ビューの中の節を切り替える tab。keyboard の操作と読み上げは React Aria に任せる。
 *
 * **1 度選んだ tab の中身は、別の tab を選んだ後も描き続ける。** 中身が取得した結果と入力途中の
 * 値を、tab を切り替えても保つ。選んでいない tab の中身は取得を始めない。
 */
export function Tabs({
  label,
  items,
}: {
  /** tab の一覧の読み上げの名前。 */
  label: string;
  items: readonly TabItem[];
}) {
  const [selected, setSelected] = useState<Key | undefined>(items[0]?.id);
  const [visited, setVisited] = useState<ReadonlySet<Key>>(
    () => new Set(selected === undefined ? [] : [selected]),
  );
  return (
    <AriaTabs
      selectedKey={selected}
      onSelectionChange={(key) => {
        setSelected(key);
        setVisited((current) => new Set(current).add(key));
      }}
    >
      <TabList
        aria-label={label}
        className="flex gap-2 overflow-x-auto border-b border-line"
      >
        {items.map((item) => (
          <Tab key={item.id} id={item.id} className={tab()}>
            {item.label}
          </Tab>
        ))}
      </TabList>
      {items.map((item) => (
        <TabPanel
          key={item.id}
          id={item.id}
          shouldForceMount={visited.has(item.id)}
          className="grid grid-cols-[minmax(0,1fr)] gap-3 pt-3 outline-none data-[inert]:hidden"
        >
          {item.content}
        </TabPanel>
      ))}
    </AriaTabs>
  );
}

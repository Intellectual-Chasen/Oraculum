// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { GraphEvidence } from "@/shared/contracts/graph";
import { decodeNodeDetailResponse } from "@/shared/contracts/graphDetail";
import { describeRecordLocation } from "@/shared/lib/recordPosition";
import { searchHighlightOf } from "@/shared/lib/searchHighlight";
import { noSearchTerms } from "@/shared/lib/searchTerms";
import type { ValueCondition } from "@/shared/lib/valueCondition";
import { nodeDetailResponseJson } from "@/testdata/graph/graphResponse";
import { ValueActionsForTest } from "@/testdata/valueActions";
import { visibleText } from "@/testdata/visibleText";
import { EvidenceRecordRow, EvidenceTableHead } from "./EvidenceRecordRow";
import { SearchHighlightContext } from "./Highlighted";
import { Hint } from "./Hint";
import { useValueMenu } from "./ValueMenu";

afterEach(() => {
  cleanup();
});

/** fixture の根拠のレコードに、Event ID のフィールドを足して読む。 */
function evidenceWithEventId(): GraphEvidence[] {
  const json = nodeDetailResponseJson();
  const evidence = json.evidence.map((item) => ({
    ...item,
    observationKind: {
      ...item.observationKind,
      raw: [
        {
          name: "EventID",
          kind: "text",
          text: { rawText: "9001", valueState: "present" },
        },
        ...item.observationKind.raw,
      ],
    },
  }));
  return decodeNodeDetailResponse({ ...json, evidence }, "fixture").evidence;
}

function EvidenceTable({ evidence }: { evidence: GraphEvidence[] }) {
  const termMenu = useValueMenu();
  return (
    <>
      <table aria-label="根拠">
        <EvidenceTableHead />
        <tbody>
          {evidence.map((item) => (
            <EvidenceRecordRow
              key={describeRecordLocation(item.recordRef)}
              evidence={item}
              termActions={termMenu.actions}
            />
          ))}
        </tbody>
      </table>
      {termMenu.menu}
    </>
  );
}

/**
 * 表を描く。`onAddCondition` を渡すと、表の値の操作の provider の中に描き、位置を押したときの
 * 受け手を返す。渡さないときは provider の外に描く。
 */
function renderTable(onAddCondition?: (condition: ValueCondition) => void) {
  const evidence = evidenceWithEventId();
  const onSelectRecord = vi.fn();
  const table = <EvidenceTable evidence={evidence} />;
  render(
    onAddCondition === undefined ? (
      table
    ) : (
      <ValueActionsForTest
        onSelectRecord={onSelectRecord}
        onAddCondition={onAddCondition}
      >
        {table}
      </ValueActionsForTest>
    ),
  );
  return { evidence, onSelectRecord };
}

test("根拠のレコード 1 件を 1 つの行に出す", () => {
  const { evidence } = renderTable();
  const rows = within(screen.getByRole("table", { name: "根拠" })).getAllByRole(
    "row",
  );
  // 見出しの行とレコードの行。
  expect(rows).toHaveLength(evidence.length + 1);
});

test("時刻と原文の時刻の両方に期間の強調を付ける", () => {
  const [evidence] = evidenceWithEventId();
  expect(evidence?.eventTime?.rawText).toBeDefined();
  render(
    <SearchHighlightContext.Provider
      value={searchHighlightOf(noSearchTerms, {
        from: { text: "2031-10-08T10:20:35+09:00" },
        to: { text: "2031-10-08T10:20:35+09:00" },
        unit: "second",
      })}
    >
      <EvidenceTable evidence={evidence === undefined ? [] : [evidence]} />
    </SearchHighlightContext.Provider>,
  );

  const [, row] = within(
    screen.getByRole("table", { name: "根拠" }),
  ).getAllByRole("row");
  const cells = within(row as HTMLElement).getAllByRole("cell");
  expect(cells[1]?.querySelector("mark")?.textContent).toBe(
    "2031-10-08 01:20:35.100",
  );
  expect(cells[4]?.querySelector("mark")?.textContent).toBe(
    "2031/10/08 10:20:35.100",
  );
});

test("1 行で Tab が止まる要素は、押せる button だけである", () => {
  renderTable(vi.fn());
  const [, row] = within(
    screen.getByRole("table", { name: "根拠" }),
  ).getAllByRole("row");
  const tabStops = Array.from(
    (row as HTMLElement).querySelectorAll<HTMLElement>(
      'button, a[href], input, select, textarea, [tabindex]:not([tabindex="-1"])',
    ),
  ).filter((element) => element.tabIndex >= 0);
  // 時刻・Event ID・位置・観測の種別の値の button。補足の印は Tab を止めない。
  expect(tabStops.map((element) => element.tagName)).toEqual([
    "BUTTON",
    "BUTTON",
    "BUTTON",
    "BUTTON",
    "BUTTON",
  ]);
});

test("表の外の補足の印は Tab を止め、表の中の補足の印は読み上げの文字列で補足を伝える", () => {
  render(
    <>
      <Hint text="外の補足">外</Hint>
      <table>
        <tbody>
          <tr>
            <td>
              <Hint text="中の補足">中</Hint>
            </td>
          </tr>
        </tbody>
      </table>
    </>,
  );
  const outside = screen.getByTitle("外の補足");
  const inside = screen.getByTitle("中の補足");
  expect(outside.tabIndex).toBe(0);
  expect(inside.hasAttribute("tabindex")).toBe(false);
  expect(within(inside).getByText("中の補足").className).toContain("sr-only");
});

test("位置の button を押すと、表の値の操作にその行のレコードの位置を渡す", () => {
  const { evidence, onSelectRecord } = renderTable(vi.fn());
  const buttons = screen.getAllByRole("button", { name: /^Record に表示: / });
  expect(buttons).toHaveLength(evidence.length);
  fireEvent.click(buttons[1] as HTMLElement);
  expect(onSelectRecord).toHaveBeenCalledTimes(1);
  expect(onSelectRecord).toHaveBeenCalledWith(evidence[1]?.recordRef);
});

/** name の値の button を押して開いたメニューの項目 item を選ぶ。 */
function chooseValueMenuItem(name: string, item: string) {
  const [button] = screen.getAllByRole("button", { name });
  fireEvent.click(button as HTMLElement);
  const menu = screen.getByRole("menu", { name });
  fireEvent.click(within(menu).getByRole("menuitem", { name: item }));
}

test.each([
  [
    "フィールドの値が等しい条件に追加",
    { kind: "field", field: "EventID", text: "9001", whole: true },
  ],
  [
    "フィールドの値が文字列を含む条件に追加",
    { kind: "field", field: "EventID", text: "9001", whole: false },
  ],
  ["含む文字列に追加", { kind: "text", mode: "contains", text: "9001" }],
  ["含まない文字列に追加", { kind: "text", mode: "excludes", text: "9001" }],
] as const)(
  "Event ID の値のメニューの「%s」は、その値の原文の条件を足す",
  (item, condition) => {
    const onAddCondition = vi.fn();
    renderTable(onAddCondition);
    chooseValueMenuItem("値の操作: EventID 9001", item);
    expect(onAddCondition).toHaveBeenCalledTimes(1);
    expect(onAddCondition).toHaveBeenCalledWith(condition);
  },
);

test("分類と動作の組を持つレコードの観測の種別の値は、イベントの種類の条件を先に出す", () => {
  const onAddCondition = vi.fn();
  renderTable(onAddCondition);
  const [button] = screen.getAllByRole("button", {
    name: "値の操作: evt net",
  });
  fireEvent.click(button as HTMLElement);
  const items = within(
    screen.getByRole("menu", { name: "値の操作: evt net" }),
  ).getAllByRole("menuitem");
  expect(items.map((item) => item.textContent)).toEqual([
    "イベントの種類の条件に追加",
    "フィールドの値が等しい条件に追加",
    "フィールドの値が文字列を含む条件に追加",
    "含む文字列に追加",
    "含まない文字列に追加",
    "原文の文字列をコピー",
  ]);
  fireEvent.click(items[0] as HTMLElement);
  const [first] = evidenceWithEventId();
  const action = first?.observationKind.raw.find(
    (field) => field.semantic === "event.action",
  );
  expect(onAddCondition).toHaveBeenCalledWith({
    kind: "eventKind",
    category: "net",
    action: action?.kind === "text" ? action.text.rawText : undefined,
  });
});

test("時刻の値のメニューは、表示の文字列ではなく時刻の値から期間の条件を作る", () => {
  const onAddCondition = vi.fn();
  renderTable(onAddCondition);
  chooseValueMenuItem(
    "値の操作: 時刻 2031-10-08T01:20:35.100Z",
    "期間の条件に追加",
  );
  expect(onAddCondition).toHaveBeenCalledWith({
    kind: "period",
    filter: {
      from: { text: "2031-10-08T10:20:35.100+09:00", precision: "millisecond" },
      to: { text: "2031-10-08T10:20:35.100+09:00", precision: "millisecond" },
      unit: "millisecond",
    },
  });
});

/** 語彙の項目を持たない、プロバイダの名前とイベント ID の観測の種別。 */
const providerAndEventId = {
  raw: [
    {
      name: "Provider",
      kind: "text",
      text: { rawText: "Synthetic-Provider", valueState: "present" },
    },
    {
      name: "EventID",
      kind: "text",
      text: { rawText: "9001", valueState: "present" },
    },
  ],
  status: "determined",
};

test("Event ID の値のメニューは、応答がレコードに付けたイベントの種類の組を条件の先に出す", () => {
  const onAddCondition = vi.fn();
  renderObservation(providerAndEventId, {
    onAddCondition,
    eventKind: { category: "synthetic-provider", action: "9001" },
  });
  chooseValueMenuItem("値の操作: EventID 9001", "イベントの種類の条件に追加");
  // 組は観測の種別の原文ではなく、応答の組 (backend が絞り込みに使う値) のまま渡す。
  expect(onAddCondition).toHaveBeenCalledWith({
    kind: "eventKind",
    category: "synthetic-provider",
    action: "9001",
  });
});

test("イベントの種類の組を持たないレコードには、イベントの種類の条件を出さない", () => {
  renderObservation(providerAndEventId, {
    onAddCondition: vi.fn(),
    eventKind: undefined,
  });
  const [button] = screen.getAllByRole("button", {
    name: "値の操作: EventID 9001",
  });
  fireEvent.click(button as HTMLElement);
  expect(
    within(screen.getByRole("menu")).queryByRole("menuitem", {
      name: "イベントの種類の条件に追加",
    }),
  ).toBeNull();
});

test("値の button はメニューが開いているかを aria-expanded で示し、押し直すと閉じる", () => {
  renderTable(vi.fn());
  const [button] = screen.getAllByRole("button", {
    name: "値の操作: EventID 9001",
  });
  expect(button).toHaveAttribute("aria-haspopup", "menu");
  expect(button).toHaveAttribute("aria-expanded", "false");
  fireEvent.click(button as HTMLElement);
  expect(button).toHaveAttribute("aria-expanded", "true");
  fireEvent.click(button as HTMLElement);
  expect(screen.queryByRole("menu")).toBeNull();
  expect(button).toHaveAttribute("aria-expanded", "false");
});

/** 観測の種別と Event ID のフィールドを差し替えた根拠 1 件の行を描き、その行のセルを返す。 */
function renderObservation(
  observationKind: unknown,
  options: {
    precision?: string;
    onAddCondition?: (condition: ValueCondition) => void;
    /** 渡すと、応答のイベントの種類の組を差し替える。undefined を渡すと組を外す。 */
    eventKind?: { category: string; action: string } | undefined;
  } = {},
) {
  const json = nodeDetailResponseJson();
  const [first] = json.evidence;
  const eventTime =
    options.precision === undefined
      ? first?.eventTime
      : { ...first?.eventTime, precision: options.precision };
  const eventKind =
    "eventKind" in options ? options.eventKind : first?.eventKind;
  const [evidence] = decodeNodeDetailResponse(
    {
      ...json,
      evidence: json.evidence.map((item, index) => {
        if (index !== 0) return item;
        const { eventKind: _eventKind, ...rest } = first ?? item;
        return {
          ...rest,
          observationKind,
          eventTime,
          ...(eventKind === undefined ? {} : { eventKind }),
        };
      }),
    },
    "fixture",
  ).evidence;
  const table = <EvidenceTable evidence={[evidence as GraphEvidence]} />;
  render(
    options.onAddCondition === undefined ? (
      table
    ) : (
      <ValueActionsForTest onAddCondition={options.onAddCondition}>
        {table}
      </ValueActionsForTest>
    ),
  );
  const [, row] = within(
    screen.getByRole("table", { name: "根拠" }),
  ).getAllByRole("row");
  return within(row as HTMLElement).getAllByRole("cell");
}

test("文字列を持たないフィールドは、値の状態ごとの理由を出す", () => {
  const cells = renderObservation({
    raw: [
      { name: "EventID", kind: "text", text: { valueState: "item_absent" } },
      { name: "evt", kind: "text", text: { valueState: "item_absent" } },
    ],
  });
  const reason = "フィールドなし";
  expect(cells[2]?.textContent).toContain(reason);
  expect(cells[5]?.textContent).toContain(reason);
});

test("イベントの種類の列に、意味の状態の組と推定した意味を出す", () => {
  const cells = renderObservation({
    raw: [
      {
        name: "evt",
        kind: "text",
        text: { rawText: "net", valueState: "present" },
      },
    ],
    status: "inferred",
    meaning: "接続の受け付け",
  });
  expect(within(cells[5] as HTMLElement).getByText("意味: 推定")).toBeTruthy();
  expect(
    within(cells[5] as HTMLElement).getByText("接続の受け付け"),
  ).toBeTruthy();
});

const netObservation = (status?: string) => ({
  raw: [
    {
      name: "evt",
      kind: "text",
      text: { rawText: "net", valueState: "present" },
    },
  ],
  ...(status === undefined ? {} : { status }),
});

/** セルの中で title を持つ要素の title を、改行でつないで返す。 */
function titlesOf(cell: HTMLElement | undefined): string {
  return Array.from(cell?.querySelectorAll("[title]") ?? [])
    .map((element) => element.getAttribute("title"))
    .join("\n");
}

test("精度は秒より粗いときだけ画面の文字で出し、どの行でも時刻のセルの title に入れる", () => {
  const fine = renderObservation(netObservation("determined"));
  expect(visibleText(fine[1])).not.toContain("精度:");
  expect(titlesOf(fine[1])).toMatch(/精度: /);
  cleanup();

  const coarse = renderObservation(netObservation("determined"), {
    precision: "minute",
  });
  expect(within(coarse[1] as HTMLElement).getByText("精度: 分")).toBeTruthy();
  expect(titlesOf(coarse[1])).toContain("精度: 分");
});

test("意味が確定したイベントの種類は状態を title にだけ入れ、推定と不明は画面の文字で出し、状態が無いときは印を出す", () => {
  const determined = renderObservation(netObservation("determined"));
  expect(
    within(determined[5] as HTMLElement).queryByText("意味: 確定"),
  ).toBeNull();
  expect(titlesOf(determined[5])).toContain("意味: 確定");
  cleanup();

  const undetermined = renderObservation(netObservation("undetermined"));
  expect(
    within(undetermined[5] as HTMLElement).getByText("意味: 不明"),
  ).toBeTruthy();
  cleanup();

  // イベントの種類に原文から読み取れないフィールドがあると、応答は意味の状態を持たない。
  const absent = renderObservation({
    raw: [
      ...netObservation().raw,
      { name: "subEvt", kind: "text", text: { valueState: "item_absent" } },
    ],
  });
  expect(absent[5]?.textContent).toContain("イベントの種類を読み取れない");
});

test("同じ名前と値のフィールドが 2 つあるときは、押した方の button だけが開いていることを示す", () => {
  renderObservation(
    {
      raw: [
        {
          name: "evt",
          kind: "text",
          text: { rawText: "net", valueState: "present" },
        },
        {
          name: "evt",
          kind: "text",
          text: { rawText: "net", valueState: "present" },
        },
      ],
      status: "determined",
    },
    { onAddCondition: vi.fn() },
  );
  const [first, second] = screen.getAllByRole("button", {
    name: "値の操作: evt net",
  });
  fireEvent.click(second as HTMLElement);
  expect(second).toHaveAttribute("aria-expanded", "true");
  expect(first).toHaveAttribute("aria-expanded", "false");
});

test("表の値の操作の provider が無い画面では、位置と Event ID と観測の種別の値を押せない文字列で出す", () => {
  renderTable();
  expect(screen.queryByRole("button")).toBeNull();
  const [, row] = within(
    screen.getByRole("table", { name: "根拠" }),
  ).getAllByRole("row");
  const cells = within(row as HTMLElement).getAllByRole("cell");
  expect(visibleText(cells[2])).toBe("9001");
  expect(cells[5]?.textContent).toContain("net");
});

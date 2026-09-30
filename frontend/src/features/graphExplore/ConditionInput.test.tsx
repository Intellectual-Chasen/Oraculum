// @vitest-environment jsdom
import {
  act,
  cleanup,
  fireEvent,
  render,
  screen,
  waitFor,
  within,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import {
  type Choice,
  type ConditionChip,
  ConditionInput,
  type ConditionInputProps,
} from "./ConditionInput";

afterEach(() => {
  cleanup();
});

const hosts: Choice[] = [
  { id: "n:terminal:01", label: "HOST-A" },
  { id: "n:terminal:02", label: "HOST-B" },
];

const initial: ConditionInputProps["initial"] = {
  expression: undefined,
  eventActionRange: {},
  timeFilter: undefined,
  matchConditions: { conditions: [{ conditionKey: "destination_ip" }] },
};

function renderInput(props: Partial<ConditionInputProps> = {}) {
  const onAdd = vi.fn();
  const onRemove = vi.fn();
  const view = render(
    <ConditionInput
      chips={[]}
      choices={{ terminal: { status: "loaded", value: hosts } }}
      initial={initial}
      onAdd={onAdd}
      onRemove={onRemove}
      {...props}
    />,
  );
  return { onAdd, onRemove, ...view };
}

function kindInput(): HTMLElement {
  return screen.getByRole("combobox", { name: "条件の種類" });
}

function press(element: Element, key: string) {
  fireEvent.keyDown(element, { key });
  fireEvent.keyUp(element, { key });
}

/** 種類の欄に focus し、種類の名前を入れてフィルタを適用した一覧の先頭を Enter で選ぶ。 */
function chooseKindByKeyboard(name: string) {
  const input = kindInput();
  act(() => input.focus());
  fireEvent.change(input, { target: { value: name } });
  press(input, "ArrowDown");
  press(input, "Enter");
}

test("focus で種類の一覧を区分ごとに出し、候補のある種類には候補の件数を添える", () => {
  renderInput();
  act(() => kindInput().focus());

  const listbox = screen.getByRole("listbox");
  // 区分の見出しは押せない行である。
  const sections = within(listbox)
    .getAllByRole("option")
    .filter((option) => option.getAttribute("aria-disabled") === "true")
    .map((option) => option.textContent);
  expect(sections).toEqual(["文字列", "レコード", "グラフ"]);
  const host = within(listbox)
    .getAllByRole("option")
    .find((option) => option.textContent?.startsWith("Host"));
  expect(host?.textContent).toBe("Host2");
  // 候補を渡さない種類は出さない。
  expect(listbox.textContent).not.toContain("Artifact");
  expect(listbox.textContent).toContain("検索式");
  expect(listbox.textContent).toContain("両端のレコードが期間内");
});

test("keyboard だけで、種類を選び、候補を選んで条件を追加し、種類の入力欄へ戻る", async () => {
  const { onAdd } = renderInput();

  chooseKindByKeyboard("Host");
  const value = screen.getByRole("combobox", { name: "Host" });
  expect(value).toHaveFocus();
  press(value, "ArrowDown");
  press(value, "ArrowDown");
  press(value, "Enter");

  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "terminal",
    id: "n:terminal:02",
  });
  await act(async () => {});
  expect(screen.queryByRole("combobox", { name: "Host" })).toBeNull();
  expect(kindInput()).toHaveFocus();
  // focus を戻しただけでは種類の一覧を開かず、↓ で開く。
  expect(screen.queryByRole("listbox")).toBeNull();
  press(kindInput(), "ArrowDown");
  expect(screen.getByRole("listbox")).toBeTruthy();
});

test("keyboard だけで、文字列の種類を選び、値を入れて Enter で追加し、空の値は誤りを出して追加しない", () => {
  const { onAdd } = renderInput();

  chooseKindByKeyboard("含まない");
  const field = screen.getByLabelText("含まない文字列");
  expect(field).toHaveFocus();
  fireEvent.submit(field);
  expect(onAdd).not.toHaveBeenCalled();
  expect(screen.getByText("入力なし")).toBeTruthy();

  fireEvent.change(field, { target: { value: " svchost " } });
  fireEvent.submit(field);
  expect(onAdd).toHaveBeenLastCalledWith({ kind: "excludes", text: "svchost" });
});

test("Escape で値の入力を閉じ、条件を追加しない", async () => {
  const { onAdd } = renderInput();
  chooseKindByKeyboard("含む");
  const field = screen.getByLabelText("含む文字列");
  fireEvent.change(field, { target: { value: "abc" } });
  press(field, "Escape");
  await act(async () => {});

  expect(screen.queryByLabelText("含む文字列")).toBeNull();
  expect(onAdd).not.toHaveBeenCalled();
  expect(kindInput()).toHaveFocus();
});

const chips: ConditionChip[] = [
  { key: "contains:a", label: "含む文字列", value: "a" },
  { key: "terminal", label: "端末", value: "HOST-A", nodeKind: "terminal" },
];

test("空欄で Backspace を押したときの操作を条件の入力の説明に示す", () => {
  renderInput();

  fireEvent.click(screen.getByRole("button", { name: "条件の入力 の説明" }));

  expect(
    screen.getByText("空の入力欄で Backspace を 2 回"),
  ).toBeInTheDocument();
});

test("chip の × と、chip の button の Backspace・Delete で、その chip を外す", () => {
  const { onRemove } = renderInput({ chips });
  const list = screen.getByRole("list", { name: "適用している検索の条件" });
  expect(list.textContent).toContain("含む文字列: a");

  const host = screen.getByRole("button", { name: "端末 HOST-A を削除" });
  fireEvent.click(screen.getByRole("button", { name: "含む文字列 a を削除" }));
  expect(onRemove).toHaveBeenLastCalledWith(chips[0]);

  press(host, "Backspace");
  expect(onRemove).toHaveBeenLastCalledWith(chips[1]);
  press(host, "Delete");
  expect(onRemove).toHaveBeenCalledTimes(3);
});

test("種類の欄が空のまま Backspace を押すと最後の chip の × へ移り、もう一度押すとその chip を外す。文字があるときは移らない", () => {
  const { onRemove } = renderInput({ chips });
  const input = kindInput();
  act(() => input.focus());
  press(input, "Backspace");
  expect(onRemove).not.toHaveBeenCalled();
  const last = screen.getByRole("button", { name: "端末 HOST-A を削除" });
  expect(document.activeElement).toBe(last);
  press(last, "Backspace");
  expect(onRemove).toHaveBeenLastCalledWith(chips[1]);

  onRemove.mockClear();
  act(() => input.focus());
  fireEvent.change(input, { target: { value: "H" } });
  press(input, "Backspace");
  expect(document.activeElement).toBe(input);
  expect(onRemove).not.toHaveBeenCalled();
});

test("ノードの種類と結び付く chip だけに、その種類の色の線を引く", () => {
  renderInput({ chips });
  const [text, host] = within(
    screen.getByRole("list", { name: "適用している検索の条件" }),
  ).getAllByRole("listitem");
  expect(text?.style.borderLeftColor).toBe("");
  expect(host?.style.borderLeftColor).toBe("var(--kind-terminal)");
});

test("外せない chip は × を押せず、Backspace でも外さない", () => {
  const { onRemove } = renderInput({
    chips: [
      {
        key: "x",
        label: "推定条件",
        value: "接続先 IP",
        removeDisabledReason: {
          title: "削除できない条件",
          text: "最後の 1 つ",
        },
      },
    ],
  });
  const remove = screen.getByRole("button", {
    name: "推定条件 接続先 IP を削除",
  });
  fireEvent.click(remove);
  press(remove, "Backspace");
  act(() => kindInput().focus());
  press(kindInput(), "Backspace");
  expect(onRemove).not.toHaveBeenCalled();
});

test("イベント ID の範囲は、最小が最大を超えると誤りを出し、数でない文字列も誤りにし、片側だけでも追加できる", () => {
  const { onAdd } = renderInput();
  chooseKindByKeyboard("イベント ID");
  const from = screen.getByLabelText("イベント ID の最小");
  const to = screen.getByLabelText("イベント ID の最大");

  fireEvent.submit(from);
  expect(screen.getByText("最小と最大の入力なし")).toBeTruthy();

  fireEvent.change(from, { target: { value: "4800" } });
  fireEvent.change(to, { target: { value: "4624" } });
  expect(screen.getByText("最大より大きい最小")).toBeTruthy();
  expect(from).toHaveAttribute("aria-invalid", "true");
  fireEvent.submit(from);
  expect(onAdd).not.toHaveBeenCalled();

  fireEvent.change(to, { target: { value: "48a" } });
  expect(screen.getByText("0 以上の整数ではない値")).toBeTruthy();
  expect(to).toHaveAttribute("aria-invalid", "true");

  fireEvent.change(to, { target: { value: "" } });
  fireEvent.submit(from);
  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "eventActionRange",
    from: 4800,
    to: undefined,
  });
});

test("アドレスの範囲は、prefix の長さを持たない文字列を誤りにして追加しない", () => {
  const { onAdd } = renderInput();
  chooseKindByKeyboard("CIDR の外のアドレス");
  const field = screen.getByLabelText("CIDR の外のアドレス");
  fireEvent.change(field, { target: { value: "198.51.100.0" } });
  fireEvent.submit(field);
  expect(onAdd).not.toHaveBeenCalled();
  expect(field).toHaveAttribute("aria-invalid", "true");
  expect(screen.getByText("CIDR の形式の誤り")).toBeTruthy();

  fireEvent.change(field, { target: { value: "2001:db8::/32" } });
  fireEvent.submit(field);
  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "addressNotInCidr",
    text: "2001:db8::/32",
  });
});

test("期間をブラウザの日時選択で変更しても、タイムゾーンと秒以下の精度を保つ", () => {
  const { onAdd } = renderInput({
    initial: {
      ...initial,
      timeFilter: {
        from: {
          text: "2031-10-08T10:20:35.123+09:00",
          precision: "millisecond",
        },
        unit: "millisecond",
      },
    },
  });
  chooseKindByKeyboard("期間");
  const picker = screen.getByLabelText("始まりの時刻の日時選択");
  expect(picker).toHaveAttribute("type", "datetime-local");
  fireEvent.change(picker, { target: { value: "2031-10-09T11:25" } });
  fireEvent.submit(screen.getByLabelText("始まりの時刻"));
  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "timeFilter",
    filter: {
      from: { text: "2031-10-09T11:25:00.123+09:00", precision: "millisecond" },
      to: undefined,
      unit: "millisecond",
    },
  });
});

test("期間は、形式の違う時刻を誤りにし、秒の小数部の桁から精度と比較の単位を決める", () => {
  const { onAdd } = renderInput();
  chooseKindByKeyboard("期間");
  const from = screen.getByLabelText("始まりの時刻");
  const to = screen.getByLabelText("終わりの時刻");

  fireEvent.change(from, { target: { value: "2031-10-08T10:20:35.1Z" } });
  fireEvent.submit(from);
  expect(onAdd).not.toHaveBeenCalled();
  expect(screen.getByText("時刻の形式の誤り")).toBeTruthy();
  expect(from).toHaveAttribute("aria-invalid", "true");

  fireEvent.change(from, { target: { value: "2031-10-08T10:20:35+09:00" } });
  fireEvent.change(to, { target: { value: "2031-10-08T11:00:00.500Z" } });
  fireEvent.submit(from);
  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "timeFilter",
    filter: {
      from: { text: "2031-10-08T10:20:35+09:00", precision: "second" },
      to: { text: "2031-10-08T11:00:00.500Z", precision: "millisecond" },
      unit: "millisecond",
    },
  });
});

test("期間と検索式は、適用している値を入れた状態で値の入力を開く", () => {
  renderInput({
    initial: {
      ...initial,
      expression: "LogonType == 3",
      timeFilter: {
        from: { text: "2031-10-08T00:00:02.000Z", precision: "millisecond" },
        unit: "millisecond",
      },
    },
  });
  chooseKindByKeyboard("期間");
  expect(screen.getByLabelText("始まりの時刻")).toHaveValue(
    "2031-10-08T00:00:02.000Z",
  );
  press(screen.getByLabelText("始まりの時刻"), "Escape");

  chooseKindByKeyboard("検索式");
  expect(screen.getByLabelText("検索式")).toHaveValue("LogonType == 3");
});

test("期間は、開いた条件の比較の単位を保って追加し直し、単位を選び直せる", () => {
  const { onAdd } = renderInput({
    initial: {
      ...initial,
      timeFilter: {
        from: { text: "2031-10-08T10:20:35+09:00", precision: "second" },
        to: { text: "2031-10-08T11:05:48+09:00", precision: "second" },
        unit: "millisecond",
      },
    },
  });
  chooseKindByKeyboard("期間");
  const unit = screen.getByLabelText("比較の単位");
  expect(unit).toHaveValue("millisecond");
  fireEvent.submit(screen.getByLabelText("始まりの時刻"));
  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "timeFilter",
    filter: {
      from: { text: "2031-10-08T10:20:35+09:00", precision: "second" },
      to: { text: "2031-10-08T11:05:48+09:00", precision: "second" },
      unit: "millisecond",
    },
  });
});

test("イベント ID の範囲は、適用している範囲を入れて開き、片側だけを直しても、もう片側を保つ", () => {
  const { onAdd } = renderInput({
    initial: { ...initial, eventActionRange: { from: 4624, to: 4634 } },
  });
  chooseKindByKeyboard("イベント ID の範囲");
  expect(screen.getByLabelText("イベント ID の最小")).toHaveValue("4624");
  const to = screen.getByLabelText("イベント ID の最大");
  expect(to).toHaveValue("4634");
  fireEvent.change(to, { target: { value: "4700" } });
  fireEvent.submit(to);
  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "eventActionRange",
    from: 4624,
    to: 4700,
  });
});

test("期間は、単位を選んでいなければ、書き換えた端の精度が細かいときにその精度で比べる", () => {
  const { onAdd } = renderInput({
    initial: {
      ...initial,
      timeFilter: {
        from: { text: "2031-10-08T10:20:35+09:00", precision: "second" },
        unit: "second",
      },
    },
  });
  chooseKindByKeyboard("期間");
  const from = screen.getByLabelText("始まりの時刻");
  fireEvent.change(from, {
    target: { value: "2031-10-08T10:20:35.123+09:00" },
  });
  expect(screen.getByLabelText("比較の単位")).toHaveValue("millisecond");
  fireEvent.submit(from);
  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "timeFilter",
    filter: {
      from: { text: "2031-10-08T10:20:35.123+09:00", precision: "millisecond" },
      to: undefined,
      unit: "millisecond",
    },
  });
});

test("推定条件は、1 つも選ばない適用と、整数でない許容幅を退ける", () => {
  const { onAdd } = renderInput();
  chooseKindByKeyboard("推定条件");
  const apply = screen.getByRole("button", { name: "推定条件を適用" });

  fireEvent.click(screen.getByLabelText("接続先 IP"));
  fireEvent.click(apply);
  expect(screen.getByRole("alert").textContent).toContain("推定条件の選択なし");

  fireEvent.click(screen.getByLabelText("秒単位の時刻"));
  fireEvent.change(screen.getByLabelText("秒単位の時刻の許容幅の秒数"), {
    target: { value: "1.5" },
  });
  fireEvent.click(apply);
  expect(screen.getByRole("alert").textContent).toContain(
    "0 以上の整数ではない許容幅",
  );
  expect(onAdd).not.toHaveBeenCalled();

  fireEvent.change(screen.getByLabelText("秒単位の時刻の許容幅の秒数"), {
    target: { value: "3" },
  });
  fireEvent.click(apply);
  expect(onAdd).toHaveBeenLastCalledWith({
    kind: "matchConditions",
    selection: {
      conditions: [{ conditionKey: "second_of_time", toleranceSeconds: 3 }],
    },
  });
});

test("値の要らない種類は、選んだ時点で追加する", () => {
  const { onAdd } = renderInput();
  chooseKindByKeyboard("条件を一致ノードだけに適用");
  expect(onAdd).toHaveBeenLastCalledWith({ kind: "conditionsOnOriginsOnly" });
});

test.each([
  ["keyboard", (name: string) => chooseKindByKeyboard(name)],
  [
    "pointer",
    (name: string) => {
      act(() => kindInput().focus());
      const option = screen.getByRole("option", { name });
      const pointer = { pointerId: 1, pointerType: "mouse", button: 0 };
      fireEvent.pointerDown(option, pointer);
      fireEvent.mouseDown(option);
      fireEvent.pointerUp(option, pointer);
      fireEvent.mouseUp(option);
      fireEvent.click(option);
    },
  ],
])("値の要らない種類を %s で選ぶと、種類の一覧を閉じる", async (_, choose) => {
  // 画面と同じく、追加した条件を chip にして描き直す。
  const onAdd = vi.fn();
  function Stateful() {
    const [chips, setChips] = useState<ConditionChip[]>([]);
    return (
      <ConditionInput
        chips={chips}
        choices={{ terminal: { status: "loaded", value: hosts } }}
        initial={initial}
        onAdd={(value) => {
          onAdd(value);
          setChips((current) => [
            ...current,
            {
              key: value.kind,
              label: "条件の適用先",
              value: "一致ノードだけ",
            },
          ]);
        }}
        onRemove={() => {}}
      />
    );
  }
  render(<Stateful />);
  choose("条件を一致ノードだけに適用");
  expect(onAdd).toHaveBeenLastCalledWith({ kind: "conditionsOnOriginsOnly" });
  await waitFor(() => expect(screen.queryByRole("listbox")).toBeNull());
  // 続けて種類を選べるよう、種類の欄へ focus を戻す。
  await waitFor(() => expect(document.activeElement).toBe(kindInput()));
  expect(screen.queryByRole("listbox")).toBeNull();
});

test("候補の取得に失敗した種類は、値の入力に失敗を出す", () => {
  renderInput({
    choices: {
      terminal: {
        status: "failed",
        failure: buildFetchFailure("network", "端末の一覧の取得"),
      },
    },
  });
  chooseKindByKeyboard("Host");
  expect(screen.getByRole("alert").textContent).toContain("端末の一覧の取得");
});

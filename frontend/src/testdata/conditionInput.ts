import { act, fireEvent, screen } from "@testing-library/react";

/** 選べる候補を並べる。区分の見出しは押せない行であり、並べない。 */
function choosableOptions(): HTMLElement[] {
  return screen
    .getAllByRole("option")
    .filter((option) => option.getAttribute("aria-disabled") !== "true");
}

/** 候補の一覧から、名前の文字列が合う候補を探す。名前は候補の 1 つ目の子の文字列である。 */
function findOption(matches: (name: string) => boolean): HTMLElement {
  const option = choosableOptions().find((candidate) =>
    matches(candidate.firstElementChild?.textContent ?? ""),
  );
  if (option === undefined) {
    throw new Error("the option is missing");
  }
  return option;
}

/** 検索の条件の種類の欄に focus し、名前が name の種類を選んで、値の入力を開く。 */
export function chooseConditionKind(name: string) {
  // 前の値の候補の一覧が開いている間は、その一覧の外が読み上げから外れている。
  const input = screen.getByRole("combobox", {
    name: "条件の種類",
    hidden: true,
  });
  act(() => input.focus());
  fireEvent.change(input, { target: { value: name } });
  fireEvent.click(findOption((text) => text === name));
}

/**
 * 種類を選び、label の入力欄に値を入れて、submit の button を押す。
 * fields は入力欄の label から入れる値への表である。
 */
export function addCondition(
  kind: string,
  fields: Record<string, string>,
  submit = "条件に追加",
) {
  chooseConditionKind(kind);
  for (const [label, value] of Object.entries(fields)) {
    fireEvent.change(screen.getByLabelText(label), { target: { value } });
  }
  fireEvent.click(screen.getByRole("button", { name: submit }));
}

/** 候補のある種類を選び、値の候補の一覧を開く。 */
export function openConditionValues(kind: string) {
  chooseConditionKind(kind);
  const input = screen.getByRole("combobox", { name: kind, hidden: true });
  act(() => input.focus());
  fireEvent.keyDown(input, { key: "ArrowDown" });
}

/** 候補のある種類を選び、表示の文字列が text で始まる候補を選ぶ。 */
export function chooseConditionValue(kind: string, text: string) {
  openConditionValues(kind);
  fireEvent.click(findOption((name) => name.startsWith(text)));
}

/** 候補のある種類を選び、候補の表示の文字列を並べて返す。値の入力は開いたままにする。 */
export function conditionValueOptions(kind: string): string[] {
  openConditionValues(kind);
  return choosableOptions().map(
    (option) => option.firstElementChild?.textContent ?? "",
  );
}

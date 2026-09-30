// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { Fold } from "./Fold";

afterEach(() => {
  cleanup();
});

/** jsdom は summary の click で開閉しないため、open を立てて toggle を送る。 */
function toggle(open: boolean) {
  const details = screen.getByText("見出し", { selector: "summary" })
    .parentElement as HTMLDetailsElement;
  details.open = open;
  fireEvent(details, new Event("toggle"));
  return details;
}

test("行数が少ない欄は開いた状態で中身を描く", () => {
  render(
    <Fold summary="見出し" rowCount={3}>
      <p>中身</p>
    </Fold>,
  );
  expect(screen.getByText("中身")).toBeTruthy();
  expect(
    (screen.getByText("見出し").parentElement as HTMLDetailsElement).open,
  ).toBe(true);
});

test("行数が多い欄は閉じた状態で中身を描かず、開くと描き、閉じると外す", () => {
  render(
    <Fold summary="見出し" rowCount={500}>
      <p>中身</p>
    </Fold>,
  );
  expect(screen.queryByText("中身")).toBeNull();

  toggle(true);
  expect(screen.getByText("中身")).toBeTruthy();

  toggle(false);
  expect(screen.queryByText("中身")).toBeNull();
});

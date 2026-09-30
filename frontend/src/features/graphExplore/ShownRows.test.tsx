// @vitest-environment jsdom
import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { ShowMoreRows, useShownRows } from "./ShownRows";

afterEach(cleanup);

function Rows({ total }: { total: number }) {
  const rows = useShownRows(Array.from({ length: total }, (_, i) => i));
  return (
    <>
      <ol>
        {rows.visible.map((row) => (
          <li key={row}>{row}</li>
        ))}
      </ol>
      <ShowMoreRows {...rows} />
    </>
  );
}

const rowItems = () =>
  screen.getAllByRole("listitem").filter((item) => item.closest("ol"));

test("先頭から 1,000 行ずつ表示し、表示している数と全件数の組を出し、全件で操作を消す", () => {
  const { container } = render(<Rows total={2500} />);
  expect(rowItems()).toHaveLength(1000);
  expect(container.textContent).toContain("表示: 1,000 / 2,500");

  fireEvent.click(screen.getByRole("button", { name: "続きを表示" }));
  expect(rowItems()).toHaveLength(2000);
  expect(container.textContent).toContain("表示: 2,000 / 2,500");

  fireEvent.click(screen.getByRole("button", { name: "続きを表示" }));
  expect(rowItems()).toHaveLength(2500);
  expect(screen.queryByRole("button")).toBeNull();
});

test("1,000 行以下の一覧は全行を表示し、続きを表示する操作を出さない", () => {
  render(<Rows total={3} />);
  expect(rowItems()).toHaveLength(3);
  expect(screen.queryByRole("button")).toBeNull();
});

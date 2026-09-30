// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import { DataTable } from "./DataTable";

afterEach(cleanup);

type Row = { name: string; count: number | undefined };

const rows: Row[] = [
  { name: "host10", count: 3 },
  { name: "host9", count: undefined },
  { name: "host2", count: 12 },
];

function renderTable() {
  render(
    <DataTable
      label="端末"
      rows={rows}
      rowKey={(row, index) => `${row.name}#${index}`}
      columns={[
        {
          key: "name",
          header: "名前",
          sortValue: (row) => row.name,
          cell: (row) => row.name,
        },
        {
          key: "count",
          header: "件数",
          numeric: true,
          sortValue: (row) => row.count,
          cell: (row) => row.count ?? "なし",
        },
        { key: "note", header: "メモ", cell: () => "" },
      ]}
    />,
  );
  return screen.getByRole("table", { name: "端末" });
}

function names(table: HTMLElement): string[] {
  return within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.querySelector("td")?.textContent ?? "");
}

test("並べ替える前は渡した順に描き、sortValue の無い列の見出しは押せない", () => {
  const table = renderTable();
  expect(names(table)).toEqual(["host10", "host9", "host2"]);
  const note = within(table).getByRole("columnheader", { name: "メモ" });
  expect(within(note).queryByRole("button")).toBeNull();
  expect(note.getAttribute("aria-sort")).toBeNull();
});

test("見出しを押すと値の意味の順に並べ、押し直すと向きを変える", () => {
  const table = renderTable();
  const name = within(table).getByRole("columnheader", { name: "名前" });
  fireEvent.click(within(name).getByRole("button"));
  expect(names(table)).toEqual(["host2", "host9", "host10"]);
  expect(name.getAttribute("aria-sort")).toBe("ascending");
  fireEvent.click(within(name).getByRole("button"));
  expect(names(table)).toEqual(["host10", "host9", "host2"]);
  expect(name.getAttribute("aria-sort")).toBe("descending");
});

test("数の列は数の大小で並べ、値の無い行は末尾に置く。別の列を押すと前の列の印を外す", () => {
  const table = renderTable();
  const name = within(table).getByRole("columnheader", { name: "名前" });
  fireEvent.click(within(name).getByRole("button"));
  const count = within(table).getByRole("columnheader", { name: "件数" });
  fireEvent.click(within(count).getByRole("button"));
  expect(names(table)).toEqual(["host10", "host2", "host9"]);
  fireEvent.click(within(count).getByRole("button"));
  expect(names(table)).toEqual(["host2", "host10", "host9"]);
  expect(name.getAttribute("aria-sort")).toBe("none");
  expect(count.getAttribute("aria-sort")).toBe("descending");
});

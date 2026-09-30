import { expect, test } from "vitest";
import { compareText, ipSortValue, sortRows } from "./sortValue";

const names = (rows: { row: string }[]) => rows.map(({ row }) => row);

test("文字列の中の数字の並びを数として比べる", () => {
  expect(compareText("host9", "host10")).toBeLessThan(0);
  expect(
    names(sortRows(["host10", "host9", "Host1"], (row) => row, "ascending")),
  ).toEqual(["Host1", "host9", "host10"]);
});

test("IP アドレスを文字の順ではなくアドレスの大小で並べる", () => {
  const addresses = ["192.0.2.10", "192.0.2.9", "2001:db8::1", "198.51.100.1"];
  expect(
    names(sortRows(addresses, (row) => ipSortValue(row), "ascending")),
  ).toEqual(["192.0.2.9", "192.0.2.10", "198.51.100.1", "2001:db8::1"]);
});

test("IPv4 のアドレスは IPv4 射影の IPv6 と同じ値になり、読めない文字列は文字列のまま返す", () => {
  expect(ipSortValue("192.0.2.1")).toBe(ipSortValue("::ffff:c000:201"));
  // 末尾を IPv4 で書いた IPv6 も、同じアドレスの値になる。
  expect(ipSortValue("::ffff:192.0.2.1")).toBe(ipSortValue("192.0.2.1"));
  expect(ipSortValue("64:ff9b::192.0.2.1")).toBe(
    ipSortValue("64:ff9b::c000:201"),
  );
  expect(ipSortValue("::ffff:192.0.2.256")).toBe("::ffff:192.0.2.256");
  expect(ipSortValue("::1")).toBe(1n);
  expect(ipSortValue("fe80::1%eth0")).toBe(ipSortValue("fe80::1"));
  expect(ipSortValue("192.0.2.256")).toBe("192.0.2.256");
  expect(ipSortValue("1::2::3")).toBe("1::2::3");
  expect(ipSortValue("host.example.test")).toBe("host.example.test");
});

test("同じ値の行は元の順を保ち、値の無い行は向きに依らず末尾に置く", () => {
  const rows = [
    { name: "a", count: 2 },
    { name: "b", count: undefined },
    { name: "c", count: 1 },
    { name: "d", count: 2 },
  ];
  const order = (direction: "ascending" | "descending") =>
    sortRows(rows, (row) => row.count, direction).map(({ row }) => row.name);
  expect(order("ascending")).toEqual(["c", "a", "d", "b"]);
  expect(order("descending")).toEqual(["a", "d", "c", "b"]);
});

test("元の位置を行と一緒に返す", () => {
  expect(sortRows(["b", "a"], (row) => row, "ascending")).toEqual([
    { row: "a", index: 1 },
    { row: "b", index: 0 },
  ]);
});

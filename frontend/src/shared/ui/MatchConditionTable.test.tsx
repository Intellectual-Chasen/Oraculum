// @vitest-environment jsdom
import { cleanup, render, screen, within } from "@testing-library/react";
import { afterEach, expect, test } from "vitest";
import type { MatchAssumption, MatchCondition } from "../contracts/candidates";
import { MatchConditionTable } from "./MatchConditionTable";

afterEach(cleanup);

const hostnameOnly: MatchCondition = {
  conditionKey: "destination_ip",
  use: "no_comparable_counterpart",
};

const proxyAssumption: MatchAssumption = {
  assumptionKey: "counterpart_connected_to_proxy",
  evidenceClass: "inferred",
  statement: "候補の接続先が 192.0.2.10 である接続を、Proxy への接続とみなす",
};

function destinationRow(): HTMLElement {
  return within(screen.getByRole("table")).getAllByRole("row")[1];
}

test("候補の接続先を Proxy のアドレスに限った段階は、接続先 IP の行にそのことを出す", () => {
  render(
    <MatchConditionTable
      conditions={[hostnameOnly]}
      tableLabel="この段階"
      assumptions={[proxyAssumption]}
    />,
  );

  const row = destinationRow();
  expect(row).toHaveTextContent("前提: 候補の接続先は Proxy");
  // 前提の文は tooltip に入れる。
  expect(within(row).getByTitle(proxyAssumption.statement)).toBeTruthy();
});

test("Proxy の前提を持たない段階は、接続先 IP の行に Proxy との比較を出さない", () => {
  render(
    <MatchConditionTable conditions={[hostnameOnly]} tableLabel="この段階" />,
  );

  expect(destinationRow()).not.toHaveTextContent("Proxy");
});

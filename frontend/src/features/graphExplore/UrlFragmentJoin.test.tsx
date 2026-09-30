// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { everyMatchCondition } from "@/shared/api/matchConditions";
import { decodeCaseId } from "@/shared/contracts/cases";
import { decodeUrlFragmentJoin } from "@/shared/contracts/urlFragments";
import { jsonResponse } from "@/testdata/http";
import { sigmaLocator } from "@/testdata/sigmaRuleCandidates/candidateResponse";
import { getPair } from "./graphExploreTestHarness";
import { UrlFragmentJoinView } from "./UrlFragmentJoin";

const edgeId = "e:http_request:synthetic";

function joinResponse() {
  return {
    edgeId,
    unnumberedRecordCount: 3,
    segments: [
      {
        fragmentCount: 1,
        lastNumber: 1,
        duplicateCount: 0,
        conflictingDuplicateCount: 0,
        missingNumberCount: 1,
        fragments: [{ number: 1, adopted: true, recordRef: sigmaLocator(1) }],
        decodeFailure: "missing_numbers",
      },
      {
        fragmentCount: 3,
        lastNumber: 1,
        duplicateCount: 1,
        conflictingDuplicateCount: 0,
        missingNumberCount: 0,
        fragments: [
          { number: 0, adopted: true, recordRef: sigmaLocator(2) },
          { number: 1, adopted: true, recordRef: sigmaLocator(3) },
          { number: 1, adopted: false, recordRef: sigmaLocator(4) },
        ],
        encoding: "base64url",
        decoded: {
          byteCount: 22,
          sha256: "d".repeat(64),
          leadingBytesHex: "504b0304",
          contentType: "application/zip",
          zip: {
            readable: true,
            integrity: "passed",
            entryCount: 3,
            entries: [
              { name: "a.txt" },
              { name: "b�.txt", nameHex: "62ff2e747874" },
            ],
          },
        },
      },
    ],
  };
}

function stubFetch(body: unknown) {
  const mock = vi.fn(async (_input: string) => jsonResponse(200, body));
  vi.stubGlobal("fetch", mock);
  return mock;
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

test("ボタンを押すと取得し、最後の回の ZIP の一覧と SHA-256 と番号の無い要求の件数を出す", async () => {
  const fetch = stubFetch(joinResponse());
  const onSelectRecord = vi.fn();
  render(
    <UrlFragmentJoinView
      edgeId={edgeId}
      caseId={decodeCaseId("case-a", "fixture")}
      matchConditions={everyMatchCondition()}
      onSelectRecord={onSelectRecord}
    />,
  );
  expect(fetch).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", { name: "URL の断片を連結" }));
  const result = await screen.findByRole("region", {
    name: "送信のまとまり 2",
  });
  expect(within(result).getByText("d".repeat(64))).toBeTruthy();
  expect(within(result).getByText("a.txt")).toBeTruthy();
  expect(within(result).getByText("62ff2e747874")).toBeTruthy();
  expect(getPair("表示していない file: 1", result)).toBeTruthy();
  expect(getPair("番号の無い要求: 3")).toBeTruthy();

  const url = new URL(String(fetch.mock.calls[0]?.[0]), "http://localhost");
  expect(url.pathname).toBe(
    `/api/v0/edges/${encodeURIComponent(edgeId)}/url-fragments`,
  );
  expect(url.searchParams.get("case")).toBe("case-a");
  expect(url.searchParams.getAll("matchCondition").length).toBeGreaterThan(0);

  // 行の少ない表は開いた状態で出る。
  const rows = within(
    within(result).getByRole("region", { name: "断片のレコードの表" }),
  ).getAllByRole("button");
  expect(rows).toHaveLength(3);
  fireEvent.click(rows[2] as HTMLElement);
  expect(onSelectRecord.mock.calls[0]?.[0]).toEqual(sigmaLocator(4));
});

test("見出しは操作の要素と focus を受ける要素を含まない", () => {
  stubFetch(joinResponse());
  render(
    <UrlFragmentJoinView
      edgeId={edgeId}
      caseId={undefined}
      matchConditions={everyMatchCondition()}
      onSelectRecord={vi.fn()}
    />,
  );
  const heading = screen.getByRole("heading", { name: "URL の断片の連結" });
  expect(heading.querySelector("button, [tabindex]")).toBeNull();
});

test("回を選ぶと、その回の復号しなかった理由を出す", async () => {
  stubFetch(joinResponse());
  render(
    <UrlFragmentJoinView
      edgeId={edgeId}
      matchConditions={everyMatchCondition()}
      onSelectRecord={() => {}}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "URL の断片を連結" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "送信のまとまり 1 を表示" }),
  );
  const result = screen.getByRole("region", { name: "送信のまとまり 1" });
  expect(getPair("未復号の理由: 欠けた番号", result)).toBeTruthy();
});

test("状態が成功でない行と途中で切れた行を、つながなかった理由と位置で出す", async () => {
  const response = joinResponse();
  Object.assign(response.segments[0] ?? {}, {
    fragmentCount: 4,
    lastNumber: 2,
    duplicateCount: 1,
    missingNumberCount: 2,
    fragments: [
      {
        number: 0,
        adopted: true,
        httpStatusCode: "200",
        recordRef: sigmaLocator(5),
      },
      {
        number: 0,
        adopted: false,
        httpStatusCode: "200",
        truncated: true,
        recordRef: sigmaLocator(8),
      },
      {
        number: 1,
        adopted: false,
        httpStatusCode: "403",
        recordRef: sigmaLocator(6),
      },
      {
        number: 2,
        adopted: false,
        httpStatusCode: "400",
        truncated: true,
        recordRef: sigmaLocator(7),
      },
    ],
  });
  stubFetch(response);
  const onSelectRecord = vi.fn();
  render(
    <UrlFragmentJoinView
      edgeId={edgeId}
      matchConditions={everyMatchCondition()}
      onSelectRecord={onSelectRecord}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: "URL の断片を連結" }));
  fireEvent.click(
    await screen.findByRole("button", { name: "送信のまとまり 1 を表示" }),
  );
  const result = screen.getByRole("region", { name: "送信のまとまり 1" });
  const table = within(result).getByRole("region", {
    name: "断片のレコードの表",
  });
  expect(within(table).getByText("未: 状態 403")).toBeTruthy();
  expect(within(table).getAllByText("未: 途中で切れた行")).toHaveLength(2);
  const truncated = within(result).getByRole("table", {
    name: "途中で切れた行",
  });
  // 同じ番号の別の行を連結した番号は、欠けた番号と書かない。
  const rows = within(truncated).getAllByRole("row").slice(1);
  expect(rows[0]).toHaveTextContent(/^0同じ番号の別の行を連結/);
  expect(rows[1]).toHaveTextContent(/^2欠けた番号/);
  fireEvent.click(within(rows[1] as HTMLElement).getByRole("button"));
  expect(onSelectRecord.mock.calls[0]?.[0]).toEqual(sigmaLocator(7));
});

test("復号の結果と理由の両方か、どちらも無い回を退ける", () => {
  const both = joinResponse();
  Object.assign(both.segments[1] ?? {}, { decodeFailure: "missing_numbers" });
  expect(() => decodeUrlFragmentJoin(both, "fixture")).toThrow();
  const neither = joinResponse();
  delete (neither.segments[0] as { decodeFailure?: string }).decodeFailure;
  expect(() => decodeUrlFragmentJoin(neither, "fixture")).toThrow();
  expect(() => decodeUrlFragmentJoin(joinResponse(), "fixture")).not.toThrow();
});

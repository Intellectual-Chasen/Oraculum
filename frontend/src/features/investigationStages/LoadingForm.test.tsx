// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, test, vi } from "vitest";
import type { SourceFileEntry } from "@/shared/contracts/sourceFiles";
import { stageFormatKeys } from "@/testdata/stages/stagesResponse";
import { LoadingForm } from "./LoadingForm";
import { type LoadingFormRow, loadingRowOf } from "./loadingRows";

afterEach(() => {
  cleanup();
});

test("書式の一括適用はSquid形式の資料だけを変更し、別形式と未選択の資料を保持する", () => {
  const onSubmit = vi.fn();
  render(
    <FormUnderTest
      formatKeys={[
        ...stageFormatKeys,
        "squid_logformat",
        "squid_combined_request_bytes",
        "apache_access_combined",
      ]}
      entries={[
        fileEntry("logs/one.log", ["squid_logformat"]),
        fileEntry("logs/two.log", ["squid_combined"]),
        fileEntry("logs/three.log", ["squid_combined_request_bytes"]),
        fileEntry("logs/web.log", ["apache_access_combined"]),
        fileEntry("logs/host.log", ["squid_combined", "infotrace_mark_ii"]),
        fileEntry("logs/unknown.log", []),
      ]}
      onSubmit={onSubmit}
    />,
  );
  fireEvent.change(screen.getByLabelText("logs/host.log の入力形式"), {
    target: { value: "infotrace_mark_ii" },
  });
  fireEvent.change(
    screen.getByLabelText("logs/host.log を記録した端末の表示名"),
    { target: { value: "retained.example.test" } },
  );
  expect(
    screen.getByRole("button", { name: "4 件の読み込みを開始" }),
  ).not.toHaveAttribute("aria-disabled", "true");
  fireEvent.click(
    screen.getByRole("button", { name: "logs/one.log のログ書式を指定" }),
  );
  const dialog = screen.getByRole("dialog", { name: "ログ書式の指定" });
  const spec = "%ts %>a %rm %ru";
  fireEvent.change(within(dialog).getByRole("textbox", { name: "ログ書式" }), {
    target: { value: spec },
  });
  fireEvent.click(
    within(dialog).getByRole("checkbox", {
      name: "選択中のSquid資料に同じ書式を適用",
    }),
  );
  fireEvent.click(within(dialog).getByRole("button", { name: "適用" }));
  expect(screen.getByLabelText("logs/web.log の入力形式")).toHaveValue(
    "apache_access_combined",
  );
  expect(screen.getByLabelText("logs/host.log の入力形式")).toHaveValue(
    "infotrace_mark_ii",
  );
  expect(screen.getByLabelText("logs/unknown.log の入力形式")).toHaveValue("");
  fireEvent.click(screen.getByRole("button", { name: "5 件の読み込みを開始" }));
  expect(onSubmit).toHaveBeenCalledWith(
    expect.arrayContaining([
      expect.objectContaining({
        originPath: "logs/one.log",
        formatKey: "squid_logformat",
        formatSpec: spec,
      }),
      expect.objectContaining({
        originPath: "logs/two.log",
        formatKey: "squid_logformat",
        formatSpec: spec,
      }),
      expect.objectContaining({
        originPath: "logs/three.log",
        formatKey: "squid_logformat",
        formatSpec: spec,
      }),
      expect.objectContaining({
        originPath: "logs/web.log",
        formatKey: "apache_access_combined",
        formatSpec: "",
      }),
      expect.objectContaining({
        originPath: "logs/host.log",
        formatKey: "infotrace_mark_ii",
        formatSpec: "",
        terminalHostname: "retained.example.test",
      }),
    ]),
  );
  expect(onSubmit.mock.calls[0][0]).toHaveLength(5);
});

test("一括適用を選ばない場合は編集した資料だけに書式を適用する", () => {
  const onSubmit = vi.fn();
  render(
    <FormUnderTest
      entries={[
        fileEntry("logs/one.log", ["squid_logformat"]),
        fileEntry("logs/two.log", ["squid_combined"]),
      ]}
      onSubmit={onSubmit}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "logs/one.log のログ書式を指定" }),
  );
  expect(
    screen.getByRole("checkbox", { name: "選択中のSquid資料に同じ書式を適用" }),
  ).not.toBeChecked();
  fireEvent.change(screen.getByRole("textbox", { name: "ログ書式" }), {
    target: { value: "combined" },
  });
  fireEvent.click(screen.getByRole("button", { name: "適用" }));
  fireEvent.click(screen.getByRole("button", { name: "2 件の読み込みを開始" }));
  expect(onSubmit.mock.calls[0][0]).toEqual(
    expect.arrayContaining([
      expect.objectContaining({
        originPath: "logs/one.log",
        formatSpec: "combined",
      }),
      expect.objectContaining({
        originPath: "logs/two.log",
        formatKey: "squid_combined",
        formatSpec: "",
      }),
    ]),
  );
});

test("ログ書式の編集を取り消すと適用済みの指定を保持する", () => {
  const onSubmit = vi.fn();
  render(
    <FormUnderTest
      entries={[fileEntry("logs/one.log", ["squid_logformat"])]}
      onSubmit={onSubmit}
    />,
  );
  fireEvent.click(
    screen.getByRole("button", { name: "logs/one.log のログ書式を指定" }),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "ログ書式" }), {
    target: { value: "combined" },
  });
  fireEvent.click(screen.getByRole("button", { name: "適用" }));
  fireEvent.click(
    screen.getByRole("button", { name: "logs/one.log のログ書式を指定" }),
  );
  fireEvent.change(screen.getByRole("textbox", { name: "ログ書式" }), {
    target: { value: "discarded" },
  });
  fireEvent.click(screen.getByRole("button", { name: "取消" }));
  fireEvent.click(screen.getByRole("button", { name: "1 件の読み込みを開始" }));
  expect(onSubmit.mock.calls[0][0][0].formatSpec).toBe("combined");
});

function fileEntry(
  originPath: string,
  formatCandidates: string[],
): SourceFileEntry {
  return {
    originPath,
    name: originPath,
    kind: "file",
    sizeBytes: 10,
    formatCandidates,
    undetected:
      formatCandidates.length === 0
        ? { reason: "unsupported_format" }
        : undefined,
  };
}

/**
 * 上位の画面と同じく、選んだ収集元と指定を外側で持つ。`externalChanges` は、directory の一覧のように
 * 表の外から行の並びを変える button である。
 */
function FormUnderTest(props: {
  entries: SourceFileEntry[];
  formatKeys?: string[];
  onSubmit: (rows: LoadingFormRow[]) => void;
  isSubmitting?: boolean;
  externalChanges?: {
    label: string;
    change: (rows: LoadingFormRow[]) => LoadingFormRow[];
  }[];
}) {
  const [rows, setRows] = useState<LoadingFormRow[]>(() =>
    props.entries.map(loadingRowOf),
  );
  const [caseId, setCaseId] = useState("");
  const [thenProcess, setThenProcess] = useState(true);
  return (
    <>
      {props.externalChanges?.map(({ label, change }) => (
        <button key={label} type="button" onClick={() => setRows(change)}>
          {label}
        </button>
      ))}
      <LoadingForm
        formatKeys={props.formatKeys ?? stageFormatKeys}
        rows={rows}
        onChangeRows={setRows}
        caseId={caseId}
        onChangeCaseId={setCaseId}
        thenProcess={thenProcess}
        onChangeThenProcess={setThenProcess}
        onSubmit={props.onSubmit}
        isSubmitting={props.isSubmitting ?? false}
      />
    </>
  );
}

/** `count` 件の、並びの位置を名前に持つ file。 */
function manyEntries(count: number): SourceFileEntry[] {
  return Array.from({ length: count }, (_, index) =>
    fileEntry(`many/${String(index).padStart(3, "0")}.log`, ["squid_combined"]),
  );
}

test("入力形式の選択肢は、判定した候補を先に、ほかの形式を後に表示名で出す", () => {
  render(
    <FormUnderTest
      entries={[fileEntry("a.log", ["infotrace_mark_ii"])]}
      onSubmit={vi.fn()}
    />,
  );

  const select = screen.getByLabelText("a.log の入力形式");
  expect(select).toHaveProperty("value", "infotrace_mark_ii");
  const groups = within(select)
    .getAllByRole("group")
    .map((group) => [
      group.getAttribute("label"),
      within(group)
        .getAllByRole("option")
        .map((option) => option.getAttribute("value")),
    ]);
  expect(groups).toEqual([
    ["判定した候補", ["infotrace_mark_ii"]],
    ["ほかの形式", ["squid_combined"]],
  ]);
});

test("入力形式を選んだ収集元だけを送り、選んでいない件数を出す", () => {
  const onSubmit = vi.fn();
  render(
    <FormUnderTest
      entries={[
        fileEntry("a.log", ["squid_combined"]),
        fileEntry("b.log", ["squid_combined", "infotrace_mark_ii"]),
      ]}
      onSubmit={onSubmit}
    />,
  );

  expect(
    screen.getByText("入力形式・ログ書式が未指定の 1 件は読み込まない"),
  ).toBeTruthy();
  fireEvent.change(screen.getByLabelText("a.log の記録した端末の IP"), {
    target: { value: "192.0.2.10" },
  });
  fireEvent.click(screen.getByRole("button", { name: "1 件の読み込みを開始" }));

  expect(onSubmit).toHaveBeenCalledTimes(1);
  expect(onSubmit.mock.calls[0]?.[0]).toEqual([
    {
      ...loadingRowOf(fileEntry("a.log", ["squid_combined"])),
      terminalIp: "192.0.2.10",
    },
  ]);
});

test("入力形式を選んだ収集元が無いときと応答を待つ間は、読み込みを始める操作を止める", () => {
  const { rerender } = render(
    <FormUnderTest entries={[fileEntry("a.log", [])]} onSubmit={vi.fn()} />,
  );
  expect(
    screen.getByRole("button", { name: "0 件の読み込みを開始" }),
  ).toHaveAttribute("aria-disabled", "true");

  fireEvent.change(screen.getByLabelText("a.log の入力形式"), {
    target: { value: "squid_combined" },
  });
  expect(
    screen.getByRole("button", { name: "1 件の読み込みを開始" }),
  ).not.toHaveAttribute("aria-disabled");

  rerender(
    <FormUnderTest
      entries={[fileEntry("a.log", [])]}
      onSubmit={vi.fn()}
      isSubmitting={true}
    />,
  );
  expect(
    screen.getByRole("button", { name: "1 件の読み込みを開始" }),
  ).toHaveProperty("disabled", true);
});

test("収集元を外すと、残った収集元の指定を保つ", () => {
  render(
    <FormUnderTest
      entries={[
        fileEntry("a.log", ["squid_combined"]),
        fileEntry("b.log", ["squid_combined"]),
      ]}
      onSubmit={vi.fn()}
    />,
  );
  fireEvent.change(screen.getByLabelText("b.log を記録した端末の表示名"), {
    target: { value: "host02" },
  });

  fireEvent.click(screen.getByRole("button", { name: "a.log を外す" }));

  expect(screen.queryByLabelText("a.log の入力形式")).toBeNull();
  expect(screen.getByLabelText("b.log を記録した端末の表示名")).toHaveProperty(
    "value",
    "host02",
  );
});

test("スクロールした表で行を外しても、上端に見えていた行を上端に残す", () => {
  const entries = Array.from({ length: 500 }, (_, index) =>
    fileEntry(`many/${String(index).padStart(3, "0")}.log`, ["squid_combined"]),
  );
  render(<FormUnderTest entries={entries} onSubmit={vi.fn()} />);
  const region = screen.getByRole("region", { name: "選んだ収集元の表" });
  // 行の高さの見積もりは 40 px である。4000 px で、行 100 が上端に来る。
  region.scrollTop = 4000;
  fireEvent.scroll(region);
  const rowIndexOf = (originPath: string) =>
    screen
      .getByRole("rowheader", { name: originPath })
      .closest("tr")
      ?.getAttribute("aria-rowindex");
  expect(rowIndexOf("many/100.log")).toBe("102");

  // 上端より上の行を外すと、上端の行は 1 つ前の位置になり、その位置へスクロールする。
  fireEvent.click(screen.getByRole("button", { name: "many/095.log を外す" }));
  expect(region.scrollTop).toBe(99 * 40);
  expect(rowIndexOf("many/100.log")).toBe("101");

  // 上端より下の行を外しても、上端の行の位置は変わらない。
  fireEvent.click(screen.getByRole("button", { name: "many/105.log を外す" }));
  expect(region.scrollTop).toBe(99 * 40);

  // 上端の行を外すと、同じ位置に詰まった次の行を上端に出す。
  fireEvent.click(screen.getByRole("button", { name: "many/100.log を外す" }));
  expect(region.scrollTop).toBe(99 * 40);
  expect(rowIndexOf("many/101.log")).toBe("101");
});

test("表の外から行の並びを変えても、上端に見えていた行を上端に残す", () => {
  const entries = manyEntries(500);
  render(
    <FormUnderTest
      entries={entries}
      onSubmit={vi.fn()}
      externalChanges={[
        {
          label: "上の行を外から外す",
          change: (rows) =>
            rows.filter((row) => row.originPath !== "many/010.log"),
        },
        {
          label: "行を外から足す",
          change: (rows) => [
            ...rows,
            loadingRowOf(fileEntry("added/000.log", ["squid_combined"])),
          ],
        },
        {
          label: "先頭の 50 行だけを残す",
          change: (rows) => rows.slice(0, 50),
        },
      ]}
    />,
  );
  const region = screen.getByRole("region", { name: "選んだ収集元の表" });
  region.scrollTop = 4000;
  fireEvent.scroll(region);

  fireEvent.click(screen.getByRole("button", { name: "上の行を外から外す" }));
  expect(region.scrollTop).toBe(99 * 40);
  fireEvent.click(screen.getByRole("button", { name: "行を外から足す" }));
  expect(region.scrollTop).toBe(99 * 40);
  // 覚えた上端の行が無くなり、位置も並びの外になったときは、最後の行を上端に出す。
  fireEvent.click(
    screen.getByRole("button", { name: "先頭の 50 行だけを残す" }),
  );
  expect(region.scrollTop).toBe(49 * 40);
});

test("スクロールしていない表は、表の外から行の並びを変えても先頭に留まる", () => {
  render(
    <FormUnderTest
      entries={manyEntries(500)}
      onSubmit={vi.fn()}
      externalChanges={[
        { label: "先頭の行を外から外す", change: (rows) => rows.slice(1) },
      ]}
    />,
  );
  const region = screen.getByRole("region", { name: "選んだ収集元の表" });

  fireEvent.click(screen.getByRole("button", { name: "先頭の行を外から外す" }));

  expect(region.scrollTop).toBe(0);
  expect(within(region).getAllByRole("rowheader")[0]?.textContent).toBe(
    "many/001.log",
  );
});

test("選んだ収集元が多いときは、先頭の行とその近くの行だけを描き、件数はすべてを数える", () => {
  const entries = Array.from({ length: 500 }, (_, index) =>
    fileEntry(`many/${String(index).padStart(3, "0")}.log`, ["squid_combined"]),
  );
  render(<FormUnderTest entries={entries} onSubmit={vi.fn()} />);

  const table = screen.getByRole("table");
  expect(table).toHaveAttribute("aria-rowcount", "501");
  // jsdom はスクロールする領域の高さを 0 と測る。描く行は、前後に描く範囲 (400 px) を行の高さの
  // 見積もり (40 px) で割った 10 行である。
  const rendered = within(table).getAllByRole("rowheader");
  expect(rendered.map((header) => header.textContent)).toEqual(
    entries.slice(0, 10).map((entry) => entry.originPath),
  );
  expect(rendered[0]?.closest("tr")).toHaveAttribute("aria-rowindex", "2");
  expect(rendered[9]?.closest("tr")).toHaveAttribute("aria-rowindex", "11");
  expect(screen.getByLabelText("many/000.log の入力形式")).toBeTruthy();
  expect(
    screen.getByRole("button", { name: "500 件の読み込みを開始" }),
  ).toBeTruthy();
});

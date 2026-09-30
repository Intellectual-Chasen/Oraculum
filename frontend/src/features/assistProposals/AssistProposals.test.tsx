// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { buildFetchFailure, decodeApiError } from "@/shared/api/apiFailure";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import {
  type AssistProposalItem,
  decodeAssistProposalItem,
} from "@/shared/contracts/assistProposals";
import { assertionTargetKey } from "@/shared/lib/assertionTargetKey";
import { SignedInContext } from "@/shared/ui/SignedInAccount";
import {
  addedRelationTarget,
  adoptedItemJson,
  proposalNodeId,
  proposalNote,
  proposedItemJson,
  relationItemJson,
} from "@/testdata/assistProposals/assistProposalsResponse";
import { signedInAlice, signedInViewer } from "@/testdata/session";
import {
  AssistProposalList,
  AssistProposalsForTarget,
  sameMatchSelection,
} from "./AssistProposals";
import type { AssistProposalsView } from "./useAssistProposals";

afterEach(() => {
  cleanup();
});

/** fixture の提案を作った発言と同じ関連付けの条件の選択。 */
const sameConditions: MatchConditionSelection = {
  conditions: [{ conditionKey: "destination_ip" }],
};

function item(json: unknown): AssistProposalItem {
  return decodeAssistProposalItem(json, "$");
}

function loadedView(
  items: AssistProposalItem[],
  overrides: Partial<AssistProposalsView> = {},
): AssistProposalsView {
  const byTarget = new Map<string, AssistProposalItem[]>();
  for (const entry of items) {
    const key = assertionTargetKey(entry.proposal.target);
    if (key !== undefined) {
      byTarget.set(key, [...(byTarget.get(key) ?? []), entry]);
    }
  }
  return {
    state: { status: "loaded", value: { items, byTarget } },
    reload: () => {},
    drafts: new Map(),
    changeDraft: () => {},
    deciding: new Set(),
    failures: new Map(),
    adopt: () => {},
    reject: () => {},
    ...overrides,
  };
}

const adoptLabel = "提案を採用";
const adoptEditedLabel = "修正した記述で採用";
const rejectLabel = "提案を却下";

/** 「名前: 値」の組のうち、name の組の文字列を返す。 */
function pairTexts(name: string): string[] {
  return Array.from(document.querySelectorAll("li"))
    .map((item) => item.textContent ?? "")
    .filter((text) => text.startsWith(`${name}: `));
}

/** 操作の IconButton を押し、確認の dialog の同じ名前の button を押す。 */
function confirm(name: string) {
  fireEvent.click(screen.getByRole("button", { name }));
  const dialog = screen.getByRole("alertdialog");
  fireEvent.click(within(dialog).getByRole("button", { name }));
}

test("提案に「AI 提案」の印と AI の記述を出し、確認の後に採用と却下を送る", () => {
  const adopt = vi.fn();
  const reject = vi.fn();
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([item(proposedItemJson())], { adopt, reject })}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );

  expect(
    screen.getByText("AI 提案", { selector: ".ai-proposal-mark" }),
  ).toBeTruthy();
  expect(pairTexts("記述")).toEqual([`記述: ${proposalNote}`]);
  expect(pairTexts("未決")).toEqual(["未決: 1"]);
  expect(pairTexts("すべて")).toEqual(["すべて: 1"]);

  // 確認の dialog で取消を押すと送らない。
  fireEvent.click(screen.getByRole("button", { name: adoptLabel }));
  const dialog = screen.getByRole("alertdialog");
  expect(dialog.textContent).toContain(`ノード: ${proposalNodeId}`);
  fireEvent.click(within(dialog).getByRole("button", { name: "取消" }));
  expect(adopt).not.toHaveBeenCalled();

  confirm(adoptLabel);
  confirm(rejectLabel);

  expect(adopt).toHaveBeenCalledWith("ap:0001", { analyst: undefined });
  expect(reject).toHaveBeenCalledWith("ap:0001", {
    analyst: undefined,
    reason: "",
  });
});

test("記述を修正するまで「修正した記述で採用」を押せず、修正した記述の下書きで採用する", () => {
  const adopt = vi.fn();
  const changeDraft = vi.fn();
  const proposed = item(proposedItemJson());
  const { rerender } = render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([proposed], { adopt, changeDraft })}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );
  expect(
    screen.getByRole("button", { name: adoptEditedLabel }),
  ).toHaveAttribute("aria-disabled", "true");
  fireEvent.change(screen.getByLabelText("修正した記述"), {
    target: { value: "直した記述" },
  });
  expect(changeDraft).toHaveBeenCalledWith("ap:0001", {
    reason: "",
    analyst: "",
    note: "直した記述",
  });

  rerender(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([proposed], {
          adopt,
          drafts: new Map([
            ["ap:0001", { note: "直した記述", reason: "", analyst: "" }],
          ]),
        })}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );
  // 直した記述がある間は、元の記述で採用する操作を押せない。
  expect(screen.getByRole("button", { name: adoptLabel })).toHaveAttribute(
    "aria-disabled",
    "true",
  );
  confirm(adoptEditedLabel);

  expect(adopt).toHaveBeenCalledWith("ap:0001", {
    analyst: undefined,
    note: "直した記述",
  });
});

test("採否の操作は、どの提案への操作かを対象の行で説明する", () => {
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([
          item(proposedItemJson()),
          item(proposedItemJson("ap:0002", addedRelationTarget)),
        ])}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );

  const descriptions = screen
    .getAllByRole("button", { name: adoptLabel })
    .map((button) => button.getAttribute("aria-describedby"))
    .map((id) => document.getElementById(id ?? "")?.textContent ?? "");
  expect(descriptions).toHaveLength(2);
  expect(descriptions[0]).toContain(`ノード: ${proposalNodeId}`);
  expect(descriptions[1]).toContain("エッジ: file_copy");
});

test("エッジを追加する提案に印を付け、両端のノードが無いときだけ欠落の印を付ける", () => {
  const { rerender } = render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([item(relationItemJson(true))])}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );
  expect(screen.getByText("エッジの追加")).toBeTruthy();
  expect(screen.queryByText("対象の欠落")).toBeNull();

  rerender(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([item(relationItemJson(false))])}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );
  expect(screen.getByText("対象の欠落")).toBeTruthy();
  expect(pairTexts("両端のノード")).toEqual([
    "両端のノード: 今の取り込み結果に無い",
  ]);
});

test("閲覧者の役割では採否の操作を無効にする", () => {
  render(
    <SignedInContext.Provider value={signedInViewer}>
      <AssistProposalList
        view={loadedView([item(proposedItemJson())])}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );

  expect(screen.getByText("閲覧者は記録不可")).toBeTruthy();
  for (const name of [adoptLabel, adoptEditedLabel, rejectLabel]) {
    expect(screen.getByRole("button", { name })).toBeDisabled();
  }
});

test("推定条件が違う提案と、対象が消えた提案に印を付ける", () => {
  const proposed = proposedItemJson();
  const absent = item({ ...proposed, targetOrigin: "absent" });
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([absent])}
        matchConditions={{ conditions: [{ conditionKey: "destination_port" }] }}
      />
    </SignedInContext.Provider>,
  );

  expect(screen.getByText("推定条件の不一致")).toBeTruthy();
  expect(pairTexts("今")).toEqual(["今: destination_port"]);
  expect(screen.getByText("対象の欠落")).toBeTruthy();
  expect(pairTexts("対象")).toEqual(["対象: 今の取り込み結果に無い"]);
});

test("採否を決めた提案は、採否の記録を出し、操作を出さない", () => {
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([item(adoptedItemJson())])}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );

  expect(pairTexts("決定")).toEqual(["決定: 採用"]);
  expect(pairTexts("分析者")).toEqual(["分析者: analyst-b"]);
  expect(screen.queryByRole("button", { name: adoptLabel })).toBeNull();
});

test("この起動で使えないとき、一覧はその旨を出し、詳細の欄は何も出さない", () => {
  const view = loadedView([], {
    state: {
      status: "failed",
      failure: buildFetchFailure(
        "request_rejected",
        "AI 提案を取得できませんでした。",
        decodeApiError({ code: "assist_unavailable", message: "x" }, "error"),
      ),
    },
  });
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList view={view} matchConditions={sameConditions} />
      <div data-testid="detail">
        <AssistProposalsForTarget
          target={{ kind: "node", nodeId: proposalNodeId }}
          view={view}
          matchConditions={sameConditions}
        />
      </div>
    </SignedInContext.Provider>,
  );

  expect(pairTexts("状態")[0]).toMatch(/^状態: 使用不可/);
  expect(pairTexts("次の操作")[0]).toMatch(
    /運用者に調査の directory を指定した起動を依頼/,
  );
  // data-testid は、詳細の欄が何も描かないことを確かめる入れ物を指すために使う。
  expect(screen.getByTestId("detail").childElementCount).toBe(0);
});

test("一覧と詳細の欄は、読み込み中と空と取得の失敗を分けて出す", () => {
  const failed = loadedView([], {
    state: {
      status: "failed",
      failure: buildFetchFailure("server", "AI 提案を取得できませんでした。"),
    },
  });
  const target = { kind: "node" as const, nodeId: proposalNodeId };
  const { rerender } = render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([])}
        matchConditions={sameConditions}
      />
      <AssistProposalsForTarget
        target={target}
        view={loadedView([])}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );
  expect(pairTexts("AI 提案")).toEqual(["AI 提案: 0"]);
  // 対象への提案が 0 件の詳細の欄は何も出さない。見える欄は一覧の 1 つである。
  expect(screen.getAllByRole("region", { name: "AI 提案" })).toHaveLength(1);

  rerender(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList view={failed} matchConditions={sameConditions} />
      <AssistProposalsForTarget
        target={target}
        view={failed}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );
  expect(
    screen.getAllByText("AI 提案を取得できませんでした。", { exact: false }),
  ).toHaveLength(2);

  rerender(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalList
        view={loadedView([], { state: { status: "loading" } })}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );
  expect(screen.getByText("AI 提案の読み込み中")).toBeTruthy();
});

test("詳細の欄は、対象への提案と未決の件数を出す", () => {
  render(
    <SignedInContext.Provider value={signedInAlice}>
      <AssistProposalsForTarget
        target={{ kind: "node", nodeId: proposalNodeId }}
        view={loadedView([
          item(proposedItemJson()),
          item(adoptedItemJson("ap:0002")),
        ])}
        matchConditions={sameConditions}
      />
    </SignedInContext.Provider>,
  );

  expect(screen.getByRole("heading", { name: "AI 提案" })).toBeTruthy();
  expect(pairTexts("未決")).toEqual(["未決: 1"]);
  expect(pairTexts("記述")).toHaveLength(2);
});

test("幅を送らない条件を幅 0 として、選択の同一を並びに依らずに比べる", () => {
  expect(
    sameMatchSelection(
      [
        { conditionKey: "destination_port", tolerance: 0 },
        { conditionKey: "destination_ip", tolerance: 0 },
      ],
      {
        conditions: [
          { conditionKey: "destination_ip" },
          { conditionKey: "destination_port" },
        ],
      },
    ),
  ).toBe(true);
  expect(
    sameMatchSelection([{ conditionKey: "second_of_time", tolerance: 2 }], {
      conditions: [{ conditionKey: "second_of_time", toleranceSeconds: 0 }],
    }),
  ).toBe(false);
  expect(
    sameMatchSelection([], {
      conditions: [{ conditionKey: "destination_ip" }],
    }),
  ).toBe(false);
});

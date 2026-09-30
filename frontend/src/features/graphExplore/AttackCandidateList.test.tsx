// @vitest-environment jsdom
import {
  cleanup,
  fireEvent,
  render,
  screen,
  within,
} from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import type { AttackCandidatesResponse } from "@/shared/contracts/attackCandidates";
import { decodeAttackCandidatesResponse } from "@/shared/contracts/attackCandidates";
import { decodeTerminalAssignment } from "@/shared/contracts/terminalAssignments";
import type { FetchState } from "@/shared/lib/fetchState";
import {
  candidateMatch,
  candidateResponse,
  candidateRule,
  remoteServicesCandidateMatch,
  remoteServicesCandidateRule,
} from "@/testdata/attackCandidates/candidateResponse";
import {
  analystAssignmentJson,
  assignmentSourceId,
} from "@/testdata/terminalAssignments/terminalAssignmentsResponse";
import { AttackCandidateList } from "./AttackCandidateList";

afterEach(cleanup);

/** ルールの表の、名前が pattern に一致する行の一致の件数を返す。 */
function ruleMatchCount(pattern: RegExp): string | undefined {
  const table = screen.getByRole("table", { name: "ATT&CK のルール" });
  const row = within(table)
    .getAllByRole("row")
    .find((element) => pattern.test(element.textContent ?? ""));
  return row?.children[2]?.textContent ?? undefined;
}

function renderList(
  state: FetchState<AttackCandidatesResponse>,
  selectedEdgeId: string | undefined = undefined,
) {
  const onSelectRecord = vi.fn();
  render(
    <AttackCandidateList
      state={state}
      selectedEdgeId={selectedEdgeId}
      onSelectRecord={onSelectRecord}
    />,
  );
  return onSelectRecord;
}

// 端末の割り当てから作った文脈のエッジは、割り当てと、割り当てを付けた収集元を出す。
test("割り当てから作ったエッジに、エッジを作った割り当てを出す", () => {
  const response = responseWithEdgeKinds();
  const match = response.matches[0];
  const contextual = match?.edges.find((edge) => edge.role === "address");
  if (match === undefined || contextual === undefined) {
    throw new Error("missing synthetic contextual edge");
  }
  contextual.terminalAssignments = [
    decodeTerminalAssignment(analystAssignmentJson(), "assignment"),
  ];
  render(
    <AttackCandidateList
      state={{ status: "loaded", value: response }}
      selectedEdgeId={contextual.edgeId}
      onSelectRecord={() => {}}
      sourceFileNames={new Map([[assignmentSourceId, "assigned-host.log"]])}
    />,
  );
  const table = screen.getByRole("table", {
    name: /エッジを作った端末の割り当て: 1 件/,
  });
  expect(table.textContent).toContain("分析者");
  expect(table.textContent).toContain("assigned-host.log");
  expect(table.textContent).toContain(String(analystAssignmentJson().author));
});

function responseWithEdgeKinds(): AttackCandidatesResponse {
  const match = remoteServicesCandidateMatch;
  return decodeAttackCandidatesResponse(
    candidateResponse(
      [
        {
          ...match,
          edges: match.edges.map((edge) => ({
            ...edge,
            kind:
              edge.role === "session"
                ? "terminal_remote_session"
                : "terminal_address",
          })),
        },
      ],
      [remoteServicesCandidateRule],
    ),
    "response",
  );
}

test("読み込み中と通信失敗を区別する", () => {
  const view = render(
    <AttackCandidateList
      state={{ status: "loading" }}
      selectedEdgeId={undefined}
      onSelectRecord={() => {}}
    />,
  );
  expect(screen.getByRole("status").textContent).toContain(
    "ATT&CK 候補の読み込み中",
  );
  view.rerender(
    <AttackCandidateList
      state={{
        status: "failed",
        failure: {
          kind: "network",
          summary: "ATT&CK 候補の取得",
          nextAction: "接続の確認と再試行",
        },
      }}
      selectedEdgeId={undefined}
      onSelectRecord={() => {}}
    />,
  );
  expect(screen.getByText("ATT&CK 候補の取得")).not.toBeNull();
  expect(screen.queryByText("ATT&CK 候補なし")).toBeNull();
});

test("全ruleと件数を示し、候補0件を明示する", () => {
  renderList({ status: "loaded", value: candidateResponse([]) });
  expect(screen.getByRole("heading", { name: "ATT&CK 候補" })).not.toBeNull();
  expect(screen.getByText("Process Injection")).not.toBeNull();
  expect(ruleMatchCount(/T1055.*Process Injection/)).toBe("0");
  expect(screen.getByText("ATT&CK 候補なし")).not.toBeNull();
});

test("必須input不足を候補0件と区別して表示する", () => {
  const response = candidateResponse([]);
  const rule = response.rules[0];
  const variant = rule?.variants[0];
  if (rule === undefined || variant === undefined)
    throw new Error("missing synthetic variant");
  variant.evaluation = {
    state: "not_evaluated",
    reason: "missing_input",
    detail: "windows.sysmon",
  };
  response.notEvaluated = [
    {
      ruleId: rule.id,
      variantId: variant.id,
      reason: "missing_input",
      detail: "windows.sysmon",
    },
  ];
  renderList({ status: "loaded", value: response });
  expect(screen.getAllByText("未評価").length).toBeGreaterThan(0);
  expect(screen.queryByText("ATT&CK 候補なし")).toBeNull();
});

test("選択edgeだけの候補を示し、複数matchの根拠を区別する", () => {
  const firstEdge = candidateMatch.edges[0];
  const firstEvidence = firstEdge?.evidence[0];
  if (firstEdge === undefined || firstEvidence === undefined)
    throw new Error("missing synthetic evidence");
  const second = {
    ...candidateMatch,
    matchId: "match:injection:2",
    edges: [
      {
        ...firstEdge,
        edgeId: "e:injection:2",
        evidence: [{ ...firstEvidence, lineNumber: 9 }],
      },
    ],
  };
  const response = candidateResponse([candidateMatch, second]);
  renderList({ status: "loaded", value: response }, "e:injection:2");
  expect(ruleMatchCount(/T1055/)).toBe("2");
  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(selected.textContent).toContain("e:injection:2");
  expect(selected.textContent).toContain("行: 9");
  expect(selected.textContent).not.toContain("e:injection:1");
  expect(selected.textContent).not.toContain("行: 7");
});

test("向き付きsource/sinkと理由を表示し、根拠の選択を原文表示へ渡す", () => {
  const onSelectRecord = renderList(
    { status: "loaded", value: candidateResponse() },
    "e:injection:1",
  );
  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(within(selected).getByText(/T1055.*Process Injection/)).not.toBeNull();
  expect(
    within(selected).getByText(
      /観測graphに明示的なprocess injection関係がある/,
    ),
  ).not.toBeNull();
  expect(within(selected).getByText("候補")).not.toBeNull();
  expect(selected.textContent).toContain(
    "未確定: 成功・認証・横展開・sub-technique",
  );
  expect(selected.textContent).toMatch(/始点: .*source\.exe/);
  expect(selected.textContent).toMatch(/終点: .*sink\.exe/);
  expect(selected.textContent).toContain("コードインジェクション");
  fireEvent.click(
    within(selected).getByRole("button", {
      name: /synthetic.log.*行: 7(\D|$)/,
    }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(
    candidateMatch.edges[0]?.evidence[0],
  );
});

test("T1055の単一edgeを必須のエッジと役割 edge で表示する", () => {
  renderList(
    { status: "loaded", value: candidateResponse([candidateMatch]) },
    candidateMatch.edges[0]?.edgeId,
  );

  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(
    within(selected).getByRole("heading", { name: "必須のエッジ" }),
  ).not.toBeNull();
  expect(selected.textContent).toContain("役割: edge");
});

test("T1021 ruleをT1055と区別して一覧表示する", () => {
  const response = candidateResponse(
    [candidateMatch, remoteServicesCandidateMatch],
    [candidateRule, remoteServicesCandidateRule],
  );
  renderList({ status: "loaded", value: response });

  expect(ruleMatchCount(/T1055.*Process Injection/)).toBe("1");
  expect(ruleMatchCount(/T1021.*Remote Services/)).toBe("1");
});

test("複数のATT&CK mappingを一覧と選択中の候補にすべて表示する", () => {
  const multiTechniqueRule = {
    ...candidateRule,
    attack: [
      ...candidateRule.attack,
      { id: "T1055.001", basis: "inferred" as const },
    ],
  };
  renderList(
    {
      status: "loaded",
      value: candidateResponse([candidateMatch], [multiTechniqueRule]),
    },
    candidateMatch.edges[0]?.edgeId,
  );

  expect(ruleMatchCount(/T1055, T1055\.001Process Injection/)).toBe("1");
  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(
    within(selected).getByText(/T1055, T1055\.001.*Process Injection/),
  ).not.toBeNull();
});

test("T1021の候補を選択するとRemote Servicesの関係と根拠を示す", () => {
  const onSelectRecord = renderList(
    {
      status: "loaded",
      value: candidateResponse(
        [remoteServicesCandidateMatch],
        [remoteServicesCandidateRule],
      ),
    },
    remoteServicesCandidateMatch.edges[0]?.edgeId,
  );
  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });

  expect(within(selected).getByText(/T1021.*Remote Services/)).not.toBeNull();
  expect(selected.textContent).toMatch(
    /始点: host-a\.example\.test終点: host-b\.example\.test/,
  );
  expect(selected.textContent).toContain("e:remote-session:1");
  const evidence = remoteServicesCandidateMatch.edges[0]?.evidence[0];
  if (evidence === undefined)
    throw new Error("missing remote services evidence");
  fireEvent.click(
    within(selected).getByRole("button", {
      name: /synthetic-session\.log.*行: 12(\D|$)/,
    }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(evidence);
});

test("primaryを選ぶと同じmatchの全edgeと役割別の根拠を示す", () => {
  const match = remoteServicesCandidateMatch;
  const primary = match.edges.find((edge) => edge.role === "session");
  const contextual = match.edges.find((edge) => edge.role === "address");
  if (primary === undefined || contextual === undefined) {
    throw new Error("missing synthetic primary or contextual edge");
  }
  const onSelectRecord = renderList(
    {
      status: "loaded",
      value: responseWithEdgeKinds(),
    },
    primary.edgeId,
  );

  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(selected.textContent).toContain("session");
  expect(selected.textContent).toContain("address");
  expect(selected.textContent).toContain("リモートセッション");
  expect(selected.textContent).toContain("端末の IP アドレス");
  expect(selected.textContent).toContain(primary.edgeId);
  expect(selected.textContent).toContain(contextual.edgeId);
  expect(selected.textContent).toMatch(
    /始点: host-a\.example\.test終点: host-b\.example\.test/,
  );
  expect(selected.textContent).toMatch(
    /始点: host-a\.example\.test終点: 192\.0\.2\.44/,
  );
  expect(selected.textContent).toContain("synthetic-session.log");
  expect(selected.textContent).toContain("synthetic-address.log");

  const contextualEvidence = contextual.evidence[0];
  if (contextualEvidence === undefined)
    throw new Error("missing synthetic contextual evidence");
  fireEvent.click(
    within(selected).getByRole("button", {
      name: /synthetic-address\.log.*行: 19(\D|$)/,
    }),
  );
  expect(onSelectRecord).toHaveBeenCalledWith(contextualEvidence);
});

test("contextualを選んでも同じmatch全体だけを表示し別matchと混ぜない", () => {
  const firstMatch = remoteServicesCandidateMatch;
  const firstContextual = firstMatch.edges.find(
    (edge) => edge.role === "address",
  );
  if (firstContextual === undefined)
    throw new Error("missing synthetic contextual edge");
  const unrelatedMatch = {
    ...firstMatch,
    matchId: "match:remote-service:2",
    edges: firstMatch.edges.map((edge) => ({
      ...edge,
      edgeId: `${edge.edgeId}:other-match`,
      evidence: edge.evidence.map((locator) => ({
        ...locator,
        sourceFileName: "unrelated-match.log",
        lineNumber: 31,
        recordRawTextRef: "/api/v0/records?lineNumber=31",
      })),
    })),
  };
  const onSelectRecord = renderList(
    {
      status: "loaded",
      value: candidateResponse(
        [firstMatch, unrelatedMatch],
        [remoteServicesCandidateRule],
      ),
    },
    firstContextual.edgeId,
  );

  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(selected.textContent).toContain("e:remote-session:1");
  expect(selected.textContent).toContain("e:terminal-address:1");
  expect(selected.textContent).toContain("synthetic-session.log");
  expect(selected.textContent).toContain("synthetic-address.log");
  expect(selected.textContent).not.toContain("other-match");
  expect(selected.textContent).not.toContain("unrelated-match.log");
  expect(selected.textContent).not.toContain("行: 31");
  expect(onSelectRecord).not.toHaveBeenCalled();
});

test("どのmatchにも属さないedgeには候補の詳細を出さない", () => {
  renderList(
    {
      status: "loaded",
      value: candidateResponse(
        [remoteServicesCandidateMatch],
        [remoteServicesCandidateRule],
      ),
    },
    "e:not-in-a-match",
  );

  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(selected.textContent).toContain("ATT&CK 候補なし");
  expect(selected.textContent).not.toContain("e:remote-session:1");
  expect(selected.textContent).not.toContain("e:terminal-address:1");
  expect(selected.textContent).not.toContain("synthetic-session.log");
  expect(selected.textContent).not.toContain("synthetic-address.log");
});

test("同じedgeに異なるruleが一致しても候補と根拠を別々に示す", () => {
  const firstRule = candidateResponse().rules[0];
  const firstEdge = candidateMatch.edges[0];
  const firstEvidence = firstEdge?.evidence[0];
  if (
    firstRule === undefined ||
    firstEdge === undefined ||
    firstEvidence === undefined
  ) {
    throw new Error("missing synthetic candidate values");
  }
  const secondRule = {
    ...firstRule,
    id: "attack.t1055.another-rule",
    description: "別の合成ruleによる候補。",
    matchCount: 1,
  };
  const response: AttackCandidatesResponse = candidateResponse(
    [
      candidateMatch,
      {
        ...candidateMatch,
        ruleId: secondRule.id,
        edges: [
          {
            ...firstEdge,
            evidence: [{ ...firstEvidence, lineNumber: 9 }],
          },
        ],
      },
    ],
    [firstRule, secondRule],
  );
  renderList({ status: "loaded", value: response }, firstEdge.edgeId);
  const selected = screen.getByRole("region", {
    name: "選択中のエッジの ATT&CK 候補",
  });
  expect(within(selected).getAllByRole("article")).toHaveLength(2);
  expect(selected.textContent).toContain(
    "観測graphに明示的なprocess injection関係がある",
  );
  expect(selected.textContent).toContain("別の合成ruleによる候補");
  expect(selected.textContent).toContain("行: 7");
  expect(selected.textContent).toContain("行: 9");
});

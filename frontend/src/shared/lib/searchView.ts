import type { GraphGranularity, NodeKind } from "@/shared/contracts/graph";

/**
 * 図に出す対象の選択。
 *
 * `auto` は検索の条件から決める。`manual` は分析者が選んだ種別であり、選んだ種別の
 * どれかに該当するノードを出す (OR)。種別を 1 つも選んでいない `manual` は、レコードを除く
 * すべての対象を出す。
 */
export type ViewChoice =
  | { kind: "auto" }
  | { kind: "manual"; nodeKinds: readonly NodeKind[] };

export const autoView: ViewChoice = { kind: "auto" };

/** 図に出す対象が要求へ載せる値。 */
export type ResolvedView = {
  granularity: GraphGranularity;
  /** 出ない場合は種別で絞らない。 */
  nodeKinds?: readonly NodeKind[];
};

const terminalIpView: ResolvedView = {
  granularity: "object",
  nodeKinds: ["terminal", "ip"],
};
const processView: ResolvedView = {
  granularity: "object",
  nodeKinds: ["process"],
};
const processIpView: ResolvedView = {
  granularity: "object",
  nodeKinds: ["process", "ip"],
};
const objectView: ResolvedView = { granularity: "object" };

/** 自動の選択を決めるのに読む、検索の条件の要約。 */
export type ViewInput = {
  /** 文字列・端末・事象の分類・期間・起点のどれかを持つか。 */
  hasCondition: boolean;
  /** 事象の分類の文字列。原資料の文字列のまま比べる。 */
  eventCategory?: string;
};

// 既知の制限: 事象の分類の文字列から粒度を選ぶ対応を、InfoTrace Mark II の文字列 (ps / net / file /
// reg) だけで持つ, 測れない理由: 分類の文字列は入力形式ごとに異なり、他の入力形式 (auditd の
// type、Squid) は分類の欄を持たないか別の文字列を使う。Mark II の分類の値は ps / net 以外に
// os / session / file / reg / win / powerShell / sys などがあり、対応に無い文字列は
// すべての対象を出す, 他の入力形式の分類で粒度を選び分ける必要が出たときに見直す
const categoryViews = new Map<string, ResolvedView>([
  ["ps", processView],
  ["net", processIpView],
]);

/**
 * 図に出す対象を決める。
 *
 * **検索の条件が無いときは端末と IP アドレスを出す。** 取り込んだ全体を最初に 1 枚で見渡すためである。
 * 接続元の端末を特定できない遠隔のログオンは、IP アドレスから端末へのエッジになるので、端末だけでは
 * エッジが 1 本も描かれない。
 * 条件があるときは、事象の分類に合う粒度を選び、合う粒度が無いときはすべての対象を出す。
 *
 * **手で選んだ種別にレコードが入るときは、レコードの粒度で出す。** 対象の粒度はレコードの
 * ノードを出さないためである。
 */
export function resolveView(
  choice: ViewChoice,
  input: ViewInput,
): ResolvedView {
  if (choice.kind === "manual") {
    if (choice.nodeKinds.length === 0) {
      return objectView;
    }
    return {
      granularity: choice.nodeKinds.includes("record") ? "record" : "object",
      nodeKinds: choice.nodeKinds,
    };
  }
  if (!input.hasCondition) {
    return terminalIpView;
  }
  const byCategory =
    input.eventCategory === undefined
      ? undefined
      : categoryViews.get(input.eventCategory);
  return byCategory ?? objectView;
}

/** 選択の種別の集合に kind を足すか外した、手で選んだ選択を返す。並びは与えた順を保つ。 */
export function toggledView(
  resolved: ResolvedView,
  kind: NodeKind,
  order: readonly NodeKind[],
): ViewChoice {
  const current = new Set(resolved.nodeKinds ?? []);
  if (current.has(kind)) {
    current.delete(kind);
  } else {
    current.add(kind);
  }
  return {
    kind: "manual",
    nodeKinds: order.filter((candidate) => current.has(candidate)),
  };
}

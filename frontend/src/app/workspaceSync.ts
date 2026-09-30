import type { WorkspacePatch } from "@/shared/contracts/workspaces";

/** 状態を最上位の欄の組として扱う。server と揃えた状態と画面の状態の両方に使う。 */
export type StateFields = Record<string, unknown>;

/** 状態の最上位の欄の、利用者に見せる名前。 */
const fieldLabels: Record<string, string> = {
  "*": "ワークスペースの全体",
  dockLayout: "作業場所の配置",
  recordFilter: "レコードのフィルタ",
  comparedSourceId: "比較する収集元",
  selectedSourceId: "選択中の収集元",
  searchTerms: "検索の条件",
  matchConditions: "関連付けの条件",
  openedRecord: "開いたレコード",
  selectedEdgeId: "選択中のエッジ",
  evidenceGroupSelector: "根拠のグループ",
  nodeScope: "時系列のフィルタに使うノードの範囲",
  history: "見た場所の履歴",
  bookmarks: "ブックマーク",
  graph: "グラフの探索",
};

/** 欄の名前を「、」でつないで返す。名前を持たない欄は、欄の鍵をそのまま出す。 */
export function fieldLabelList(fields: readonly string[]): string {
  return fields.map((field) => fieldLabels[field] ?? field).join("、");
}

/** server が欄を消した印の null を除く。画面の状態は、無い欄を省略で表す。 */
export function withoutNulls(state: StateFields): StateFields {
  return Object.fromEntries(
    Object.entries(state).filter(([, value]) => value !== null),
  );
}

/** 2 つの状態で値が異なる最上位の欄。 */
export function changedFields(a: StateFields, b: StateFields): string[] {
  return [...new Set([...Object.keys(a), ...Object.keys(b)])].filter(
    (key) => JSON.stringify(a[key]) !== JSON.stringify(b[key]),
  );
}

/** state の欄 fields を持つ差分。state に無い欄は null で消す。 */
export function patchOf(
  state: StateFields,
  fields: readonly string[],
): WorkspacePatch {
  return Object.fromEntries(fields.map((key) => [key, state[key] ?? null]));
}

/** 差分を重ねた状態を返す。null の欄は消す。 */
export function applyPatch(
  state: StateFields,
  patch: WorkspacePatch,
): StateFields {
  return withoutNulls({ ...state, ...patch });
}

/** 他の接続に知らせ、追従した接続が画面へ適用する自分の選択。 */
export type WorkspaceSelection = {
  openedRecord?: unknown;
  selectedEdgeId?: unknown;
  evidenceGroupSelector?: unknown;
  /** グラフで選んだノードかグループ。状態の graph.selected にあたる。 */
  graphSelected?: unknown;
};

function objectOr(value: unknown): Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : {};
}

/** 状態から選択を取り出す。 */
export function selectionOf(state: StateFields): WorkspaceSelection {
  return withoutUndefined({
    openedRecord: state.openedRecord,
    selectedEdgeId: state.selectedEdgeId,
    evidenceGroupSelector: state.evidenceGroupSelector,
    graphSelected: objectOr(state.graph).selected,
  });
}

function withoutUndefined<T extends Record<string, unknown>>(value: T): T {
  return Object.fromEntries(
    Object.entries(value).filter(([, item]) => item !== undefined),
  ) as T;
}

/**
 * 状態の選択を selection に置き換える。selection は他の接続が送った JSON のままでよい。
 * 置き換えた結果が読めるかは呼び出し側が確かめる。
 */
export function withSelection(
  state: StateFields,
  selection: unknown,
): StateFields {
  const source = objectOr(selection);
  const graph =
    state.graph === undefined
      ? undefined
      : withoutUndefined({
          ...objectOr(state.graph),
          selected: source.graphSelected ?? undefined,
        });
  return withoutUndefined({
    ...state,
    openedRecord: source.openedRecord ?? undefined,
    selectedEdgeId: source.selectedEdgeId ?? undefined,
    evidenceGroupSelector: source.evidenceGroupSelector ?? undefined,
    graph,
  });
}

const presenceSelectionLimit = 2 * 1024;

/** server が受け取る選択は JSON で 2 KiB までである。 */
export function fitsPresence(selection: WorkspaceSelection): boolean {
  return new Blob([JSON.stringify(selection)]).size <= presenceSelectionLimit;
}

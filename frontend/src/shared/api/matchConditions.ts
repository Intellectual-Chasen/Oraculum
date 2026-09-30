import { type ConditionKey, conditionKeys } from "../contracts/candidates";

/**
 * 値の一致に幅を取れる条件の種別。
 * 定義元は `backend/pipeline/match_selection.go` の `toleratedConditions` である。
 * 幅を取らない条件へ幅を付けた要求は `invalid_request` になる。
 */
export const toleratedConditionKeys: readonly ConditionKey[] = [
  "second_of_time",
];

/** 幅を取れる条件かを返す。 */
export function takesTolerance(conditionKey: ConditionKey): boolean {
  return toleratedConditionKeys.includes(conditionKey);
}

/** 分析者が候補の絞り込みに用いると決めた条件 1 件。 */
export type SelectedMatchCondition = {
  conditionKey: ConditionKey;
  /**
   * 値の一致に認める幅。秒で数える。0 は同じ秒の候補を取る。
   * 出ない場合は幅を付けずに送る。
   */
  toleranceSeconds?: number;
};

/**
 * 候補を絞るのに用いる条件の選択。
 *
 * **グラフを読む操作がすべて同じ選択を読む。** 選択を 2 つ持つと、ノードの詳細と
 * 関係の詳細が別の関連付けの結果を出す。
 */
export type MatchConditionSelection = {
  /** 用いる条件。要素数 0 は、どの条件でも絞らない関連付けを求める選択である。 */
  conditions: SelectedMatchCondition[];
};

/**
 * 契約が定めるすべての条件を幅 0 で選んだ選択を返す。
 * 呼ぶ側は、選択を持つ画面の初期の状態としてこの値を使う。
 */
export function everyMatchCondition(): MatchConditionSelection {
  return {
    conditions: conditionKeys.map((conditionKey) => ({
      conditionKey,
      toleranceSeconds: takesTolerance(conditionKey) ? 0 : undefined,
    })),
  };
}

/** 条件の種別と幅を分ける字。 */
const toleranceSeparator = "~";

/**
 * 選択を `matchCondition` の項目の値へ直す。
 * 1 つの条件につき 1 つの要素を返し、要求は同じ名前の項目をその個数だけ繰り返して送る。
 *
 * **要素数 0 の選択を渡さない。** 条件を 1 つも比べない段階は、相手の側のレコードを
 * そのまま候補に並べる。backend はその要求を退ける。呼ぶ側が 1 つ以上を選ばせる。
 */
export function matchConditionParams(
  selection: MatchConditionSelection,
): string[] {
  return selection.conditions.map((condition) => {
    const tolerance = condition.toleranceSeconds;
    if (tolerance === undefined || !takesTolerance(condition.conditionKey)) {
      return condition.conditionKey;
    }
    return `${condition.conditionKey}${toleranceSeparator}${tolerance}`;
  });
}

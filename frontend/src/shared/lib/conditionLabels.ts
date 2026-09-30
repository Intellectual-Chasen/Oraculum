import type { ConditionKey, ConditionUse } from "../contracts/candidates";

/**
 * 推定条件の種類の表示。`ConditionKey` の全数を持ち、段階が持つ条件は入力形式の組で変わる。
 */
export const conditionKeyLabels: Record<ConditionKey, string> = {
  terminal_ip_assignment: "IP の割り当ての端末",
  terminal_identity_matches: "端末の ID の一致",
  parent_process_id_matches: "親プロセスの ID の一致",
  destination_ip: "接続先 IP",
  destination_port: "接続先 port",
  destination_authority: "接続先のホストと port",
  second_of_time: "秒単位の時刻",
  sub_second_of_time: "秒未満の時刻",
  client_port: "接続元 port",
  process: "プロセス",
  user: "利用者",
};

/** 推定条件の使用の表示。「使用」の列の値である。 */
export const conditionUseLabels: Record<ConditionUse, string> = {
  used: "使用",
  not_used: "未使用",
  no_comparable_counterpart: "比較の値なし",
  item_absent_on_counterpart: "候補にフィールドなし",
};

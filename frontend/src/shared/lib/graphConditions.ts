import type { EdgeKind, NodeKind } from "../contracts/graph";

/** 一致ノードを限るノード。`NodeRef` と同じ形である。 */
export type OriginNode = {
  id: string;
  /** ノードの表示名。原資料の文字列である。 */
  label: string;
  kind?: NodeKind;
};

/**
 * 図の要求だけが読む検索の条件。段数、関係の種別、アドレスの範囲、件数を集計する欄、文字列と
 * 事象の種別の条件を起点だけに当てるか、両端のレコードが期間の中にあるエッジだけを出すかを持つ。
 * 根拠のレコードを絞る条件と検索の文字列は、時系列も読むため別の値が持つ。AI 支援の card と
 * 発言に添える文脈が、この組で図の条件を渡す。
 */
export type GraphConditions = {
  /** 起点から広げるホップ数。 */
  depth: number;
  /** 出ない場合はノードで一致ノードを限らない。他の条件にも合うノードだけが一致ノードになる。 */
  origins?: readonly OriginNode[];
  /** 出ない場合は関係の種別で絞らない。どれかの種別の関係だけを辿る。 */
  edgeKinds?: readonly EdgeKind[];
  /** IP アドレスのノードを残すアドレスの範囲。出ない場合は範囲で絞らない。 */
  addressInCidr?: string;
  /** IP アドレスのノードを外すアドレスの範囲。出ない場合は範囲で絞らない。 */
  addressNotInCidr?: string;
  /** 値ごとに数える欄。語彙の項目または原資料の key。出ない場合は数えない。 */
  countBy?: string;
  /** 真のとき、文字列と事象の種別の条件を起点の判定だけに当てる。 */
  conditionsOnOriginsOnly?: boolean;
  /** 真のとき、端点のレコードが期間の外にあるエッジを辿らない。 */
  endpointRecordsInPeriod?: boolean;
};

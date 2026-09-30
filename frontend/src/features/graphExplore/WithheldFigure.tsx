import { ChevronsUp, Eye, List } from "lucide-react";
import type { ReactNode } from "react";
import type { NodeKind, WithheldGraphResponse } from "@/shared/contracts/graph";
import { formatCount } from "@/shared/lib/format";
import { IconButton } from "@/shared/ui/IconButton";
import {
  type DrawLimit,
  drawLimitChoices,
  smallestLimitFor,
} from "./drawLimit";
import { edgeKindLabels, nodeKindColors, nodeKindLabels } from "./labels";

function drawLimitLabel(limit: DrawLimit): string {
  return limit === "all" ? "すべて" : `${formatCount(limit)} 件`;
}

/** 図に描くノードの数の上限を選ぶ。 */
export function DrawLimitSelect({
  value,
  onChange,
}: {
  value: DrawLimit;
  onChange: (limit: DrawLimit) => void;
}) {
  return (
    <label className="draw-limit">
      描画するノードの上限
      <select
        value={String(value)}
        onChange={(event) => {
          const chosen = drawLimitChoices.find(
            (limit) => String(limit) === event.target.value,
          );
          if (chosen !== undefined) {
            onChange(chosen);
          }
        }}
      >
        {drawLimitChoices.map((limit) => (
          <option key={String(limit)} value={String(limit)}>
            {drawLimitLabel(limit)}
          </option>
        ))}
      </select>
    </label>
  );
}

/**
 * グラフに描くノードの数が上限を超えたときに、グラフの代わりに出す値の組と操作。
 *
 * **描画しなかった件数と上限だけを出す。** 描いたグラフに一部だけを載せると、載せなかった
 * ノードとの関係を分析者が見逃す。ノードの種類ごとの件数とエッジの種類ごとの本数は、Nodes の
 * ビューが出す (WithheldKindTables)。
 */
export function WithheldFigure({
  response,
  onChangeDrawLimit,
  onShowNodes,
  children,
}: {
  response: WithheldGraphResponse;
  onChangeDrawLimit: (limit: DrawLimit) => void;
  /** Nodes のビューを前面に出す。出ない場合は操作を出さない。 */
  onShowNodes: (() => void) | undefined;
  /** 値の組に続けて出す組。開いたレコードの強調の件数を渡す。 */
  children?: ReactNode;
}) {
  const count = response.subgraphNodeCount;
  const raised = smallestLimitFor(count);
  return (
    <section aria-label="描画上限超過" className="withheld">
      <ul className="value-pairs">
        <li>
          <span className="pair-name">対象ノード:</span> {formatCount(count)}
        </li>
        <li>
          <span className="pair-name">上限:</span>{" "}
          {formatCount(response.nodeLimit)}
        </li>
        {children}
      </ul>
      {/* GPU で描く環境では、ノード 1 万件を超える図も数秒で操作できるようになる (drawLimit.ts)。 */}
      <IconButton
        label={raised === "all" ? "上限を解除して描画" : "上限を上げて描画"}
        onPress={() => onChangeDrawLimit(raised)}
      >
        <ChevronsUp size={16} aria-hidden="true" />
      </IconButton>
      {onShowNodes === undefined ? null : (
        <IconButton label="Nodes を表示" onPress={onShowNodes}>
          <List size={16} aria-hidden="true" />
        </IconButton>
      )}
    </section>
  );
}

/**
 * 上限を超えた応答の、一致ノードの種類ごとの件数と、描画するはずだったエッジの種類ごとの本数。
 * 種類ごとに、その種類だけをグラフに出す操作を置く。
 *
 * **今出している種類が 1 つだけのとき、その種類だけを出す操作を出さない。** 押しても要求が
 * 変わらず、描画しない画面のまま残る。
 */
export function WithheldKindTables({
  response,
  shownKinds,
  onShowOnlyKind,
}: {
  response: WithheldGraphResponse;
  /** 今の要求がグラフに出す種類。出ない場合は種類で絞っていない。 */
  shownKinds: readonly NodeKind[] | undefined;
  onShowOnlyKind: (kind: NodeKind) => void;
}) {
  const onlyShownKind = shownKinds?.length === 1 ? shownKinds[0] : undefined;
  // SID と名前の組を 1 組も持たない応答では、列を出さない。
  const pairCount =
    response.matchedAccountIdentityPairCount === 0
      ? undefined
      : response.matchedAccountIdentityPairCount;
  return (
    <>
      {response.matchedKinds.length === 0 ? null : (
        <table aria-label="一致ノードの種類">
          <thead>
            <tr>
              <th scope="col">種類</th>
              <th scope="col" className="text-right">
                件数
              </th>
              {pairCount === undefined ? null : (
                <th scope="col" className="text-right">
                  SID と名前の組
                </th>
              )}
              <th scope="col">
                <span className="sr-only">操作</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {response.matchedKinds.map((kind) => (
              <tr key={kind.kind}>
                <td className="whitespace-nowrap">
                  <span
                    aria-hidden="true"
                    className="kind-dot"
                    style={{ background: nodeKindColors[kind.kind] }}
                  />{" "}
                  {nodeKindLabels[kind.kind]}
                </td>
                <td className="text-right tabular-nums">
                  {formatCount(kind.count)}
                </td>
                {pairCount === undefined ? null : (
                  <td className="text-right tabular-nums">
                    {kind.kind === "account" ? formatCount(pairCount) : null}
                  </td>
                )}
                <td>
                  {kind.kind === onlyShownKind ? null : (
                    <IconButton
                      label={`${nodeKindLabels[kind.kind]}だけをグラフに表示`}
                      onPress={() => onShowOnlyKind(kind.kind)}
                    >
                      <Eye size={14} aria-hidden="true" />
                    </IconButton>
                  )}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      {response.edgeKindCounts.length === 0 ? null : (
        <table aria-label="エッジの種類">
          <thead>
            <tr>
              <th scope="col">エッジの種類</th>
              <th scope="col" className="text-right">
                本数
              </th>
            </tr>
          </thead>
          <tbody>
            {response.edgeKindCounts.map((kind) => (
              <tr key={kind.kind}>
                <td>{edgeKindLabels[kind.kind]}</td>
                <td className="whitespace-nowrap text-right tabular-nums">
                  {formatCount(kind.count)}
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </>
  );
}

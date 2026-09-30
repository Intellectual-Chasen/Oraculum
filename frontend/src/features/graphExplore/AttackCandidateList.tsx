import type { AttackCandidatesResponse } from "@/shared/contracts/attackCandidates";
import type { RecordLocator } from "@/shared/contracts/common";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import {
  describeRecordLocation,
  recordRefKey,
} from "@/shared/lib/recordPosition";
import { DataTable } from "@/shared/ui/DataTable";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { TerminalAssignmentTable } from "@/shared/ui/TerminalAssignmentTable";
import { edgeKindLabels } from "./labels";
import { NodeLabelValue } from "./NodeLabelView";
import { readNodeLabel } from "./subgraph";

type AttackCandidateListProps = {
  state: FetchState<AttackCandidatesResponse>;
  selectedEdgeId: string | undefined;
  onSelectRecord: (locator: RecordLocator) => void;
  /** 収集元の sourceId から file 名を探す表。割当の「適用する収集元」を file 名で出す。 */
  sourceFileNames?: ReadonlyMap<string, string>;
};

/** rule一覧と、選択edgeが属するmatchの候補と根拠を表示する。 */
export function AttackCandidateList({
  state,
  selectedEdgeId,
  onSelectRecord,
  sourceFileNames,
}: AttackCandidateListProps) {
  return (
    <section aria-labelledby="attack-candidates-heading">
      <h3 id="attack-candidates-heading">ATT&CK 候補</h3>
      <FetchStateView
        state={state}
        loadingDescription="ATT&CK 候補の読み込み中"
      >
        {({ rules, matches }) => {
          const selectedMatches = matches.filter((match) =>
            match.edges.some((edge) => edge.edgeId === selectedEdgeId),
          );
          const rulesById = new Map(rules.map((rule) => [rule.id, rule]));
          const allVariantsNotEvaluated =
            rules.length > 0 &&
            rules.every((rule) =>
              rule.variants.every(
                (variant) => variant.evaluation.state === "not_evaluated",
              ),
            );
          return (
            <>
              {rules.length === 0 ? null : (
                <DataTable
                  label="ATT&CK のルール"
                  rows={rules}
                  rowKey={(rule) => rule.id}
                  columns={[
                    {
                      key: "attack",
                      header: "ATT&CK",
                      mono: true,
                      cell: (rule) => attackMappingLabel(rule),
                    },
                    {
                      key: "title",
                      header: "ルール",
                      cell: (rule) => <RawText text={rule.title} />,
                    },
                    {
                      key: "matches",
                      header: "一致",
                      numeric: true,
                      cell: (rule) => formatCount(rule.matchCount),
                    },
                    {
                      key: "state",
                      header: "評価",
                      cell: (rule) =>
                        rule.variants.some(
                          (variant) =>
                            variant.evaluation.state === "not_evaluated",
                        ) ? (
                          <NotEvaluatedLabel />
                        ) : null,
                    },
                  ]}
                />
              )}
              {matches.length === 0 ? (
                <p>
                  {allVariantsNotEvaluated ? (
                    <NotEvaluatedLabel />
                  ) : (
                    "ATT&CK 候補なし"
                  )}
                </p>
              ) : null}
              {selectedEdgeId === undefined ? null : (
                <section aria-label="選択中のエッジの ATT&CK 候補">
                  <h4 className="flex items-center gap-2">
                    選択中のエッジの ATT&CK 候補
                    <StatusLabel
                      status="idle"
                      label="候補"
                      details="未確定: 成功・認証・横展開・sub-technique"
                    />
                  </h4>
                  {selectedMatches.length === 0 ? (
                    <p>ATT&CK 候補なし</p>
                  ) : (
                    selectedMatches.map((match) => {
                      const rule = rulesById.get(match.ruleId);
                      if (rule === undefined) return null;
                      return (
                        <article key={`${match.ruleId}:${match.matchId}`}>
                          <KeyValueList
                            stacked
                            pairs={[
                              {
                                name: "Match ID",
                                value: <RawText text={match.matchId} />,
                              },
                              {
                                name: "候補",
                                value: (
                                  <>
                                    {attackMappingLabel(rule)}{" "}
                                    <RawText text={rule.title} />
                                  </>
                                ),
                              },
                              {
                                name: "候補の理由",
                                value: <RawText text={rule.description} />,
                              },
                            ]}
                          />
                          {match.edges.map((edge) => (
                            <section
                              key={edge.edgeId}
                              aria-label={`${edge.role} edge ${edge.edgeId}`}
                            >
                              <h5>
                                {edge.evidenceRole === "required"
                                  ? "必須のエッジ"
                                  : "補助のエッジ"}
                              </h5>
                              <KeyValueList
                                stacked
                                pairs={[
                                  {
                                    name: "役割",
                                    value: <RawText text={edge.role} />,
                                  },
                                  {
                                    name: "エッジの種類",
                                    value: edgeKindLabels[edge.kind],
                                  },
                                  {
                                    name: "エッジ ID",
                                    value: <RawText text={edge.edgeId} />,
                                  },
                                  {
                                    name: "始点",
                                    value: (
                                      <NodeLabelValue
                                        label={readNodeLabel(
                                          edge.sourceNode.label,
                                        )}
                                      />
                                    ),
                                  },
                                  {
                                    name: "終点",
                                    value: (
                                      <NodeLabelValue
                                        label={readNodeLabel(
                                          edge.sinkNode.label,
                                        )}
                                      />
                                    ),
                                  },
                                ]}
                              />
                              {edge.terminalAssignments === undefined ? null : (
                                <TerminalAssignmentTable
                                  assignments={edge.terminalAssignments}
                                  caption="エッジを作った端末の割り当て"
                                  sourceFileNames={sourceFileNames}
                                />
                              )}
                              <ul aria-label="根拠のレコード">
                                {edge.evidence.map((locator) => (
                                  <li key={recordRefKey(locator)}>
                                    <button
                                      type="button"
                                      className="value-link"
                                      onClick={() => onSelectRecord(locator)}
                                    >
                                      <RawText
                                        text={describeRecordLocation(locator)}
                                      />
                                    </button>
                                  </li>
                                ))}
                              </ul>
                            </section>
                          ))}
                        </article>
                      );
                    })
                  )}
                </section>
              )}
            </>
          );
        }}
      </FetchStateView>
    </section>
  );
}

/** 入力不足で評価しなかったルールの状態。 */
function NotEvaluatedLabel() {
  return <StatusLabel status="idle" label="未評価" details="理由: 入力不足" />;
}

function attackMappingLabel(
  rule: AttackCandidatesResponse["rules"][number],
): string {
  const identifiers = rule.attack.map((reference) => reference.id).join(", ");
  return identifiers || rule.id;
}

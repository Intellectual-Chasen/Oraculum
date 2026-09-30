/** プロセスのノード。 */
function processNodeJson(index: number) {
  return {
    id: `n:process:${index.toString(16).padStart(4, "0")}`,
    kind: "process",
    keyForm: "terminal_id_process_id",
    identity: [
      { semantic: "terminal.id", value: "HOST-E-TMID" },
      { semantic: "process.id", value: `{P${index}}` },
    ],
    label: {
      rawText: `C:\\synthetic\\tool${index}.exe`,
      valueState: "present",
    },
    observation: "observed",
    creationRecord: "absent",
  };
}

function terminalNodeJson() {
  return {
    id: "n:terminal:0e0e",
    kind: "terminal",
    keyForm: "terminal_id",
    identity: [{ semantic: "terminal.id", value: "HOST-E-TMID" }],
    label: { rawText: "HOST-E", valueState: "present" },
    observation: "observed",
    creationRecord: "item_absent",
  };
}

/** 時刻。 */
function orderTime(second: number) {
  const text = `2031-10-08T10:20:${second.toString().padStart(2, "0")}+09:00`;
  return {
    rawText: text,
    normalized: text,
    normalizedForm: "rfc3339_absolute",
    precision: "second",
    offsetState: "in_value",
    offsetText: "+09:00",
    clock: "terminal_local",
    meaning: "event",
    valueState: "present",
  };
}

/**
 * 次数の応答。端末 1 件と、processCount 件のプロセスを含む。
 * 1 番目と 2 番目のプロセスは同じ値で同じ順位にあり、最後のプロセスは値を持たない。
 */
export function investigationOrderResponseJson(processCount = 3) {
  const entries = Array.from({ length: processCount }, (_, index) => {
    const last = index === processCount - 1;
    return {
      node: processNodeJson(index + 1),
      value: last ? null : index < 2 ? 5 : processCount - index,
      rank: last ? processCount : index < 2 ? 1 : index + 1,
      tieCount: last ? 1 : index < 2 ? 2 : 1,
    };
  });
  return {
    method: "degree",
    parameters: [],
    inputs: {
      objectCount: processCount + 1,
      pairCount: processCount,
      recordCount: 12,
      timedRecordCount: 10,
      timeRange: { from: orderTime(1), to: orderTime(59) },
    },
    kinds: [
      { kind: "process", entries },
      {
        kind: "terminal",
        entries: [
          { node: terminalNodeJson(), value: 12, rank: 1, tieCount: 1 },
        ],
      },
    ],
  };
}

/**
 * Sigma の一致の集計の応答。プロセス 3 件を含む。値は high・4 件、low・9 件、一致なしである。
 * ruleSet が偽のときは、ルールの集合を渡していない起動の応答 (値なし、集合の係数なし) にする。
 */
export function sigmaOrderResponseJson({
  ruleSet = true,
  evaluatedRecordCount = 30,
} = {}) {
  const values = ruleSet
    ? [5_000_000_004, 3_000_000_009, 0]
    : [null, null, null];
  const ruleSetParameters = ruleSet
    ? [
        { name: "rule_set_revision", value: "unverified" },
        { name: "rule_set_content_sha256", value: "a".repeat(64) },
        {
          name: "evaluated_record_count",
          value: String(evaluatedRecordCount),
        },
      ]
    : [];
  return {
    ...investigationOrderResponseJson(),
    method: "sigma",
    parameters: [
      ...ruleSetParameters,
      {
        name: "value_formula",
        value: "level_rank * 1000000000 + matched_record_count",
      },
    ],
    kinds: [
      {
        kind: "process",
        entries: values.map((value, index) => ({
          node: processNodeJson(index + 1),
          value,
          rank: ruleSet ? index + 1 : 1,
          tieCount: ruleSet ? 1 : 3,
        })),
      },
    ],
  };
}

import type { RecordLocator } from "@/shared/contracts/common";
import type { SigmaRuleCandidatesResponse } from "@/shared/contracts/sigmaRuleCandidates";

/** レコードの位置。 */
export function sigmaLocator(lineNumber: number): RecordLocator {
  return {
    sourceId: "synthetic-events",
    sourceContentSha256: "b".repeat(64),
    sourceFileName: "synthetic-events.xml",
    positionKind: "line_number",
    lineNumber,
    recordRawTextRef: `raw:synthetic-${lineNumber}`,
  };
}

/** ルール 1 つが 2 件のレコードに一致し、1 つのルールを評価しなかった応答。 */
export function sigmaCandidatesResponse(): SigmaRuleCandidatesResponse {
  return {
    ruleSet: {
      directory: "synthetic-rules",
      revision: "ab12".repeat(10),
      revisionSource: "git_head",
      gitWorkTree: "/synthetic/checkout",
      contentSha256: "c".repeat(64),
      ruleFileCount: 3,
    },
    evaluatedRuleCount: 2,
    evaluatedRecordCount: 9,
    unevaluatedRecordGroups: [
      { channel: "Example/Operational", recordCount: 4 },
      { provider: "Example-Provider", recordCount: 1 },
    ],
    skippedPairCount: 6,
    skippedPairRecordCount: 3,
    recordsWithoutSemantics: 2,
    rules: [
      {
        path: "process/synthetic.yml",
        id: "00000000-0000-4000-8000-000000000001",
        title: "Synthetic Process Rule",
        author: "Synthetic Author",
        level: "high",
        status: "test",
        condition: "selection and not filter",
        selections: [
          {
            name: "selection",
            definition: "Image|endswith:\n    - \\tool.exe\n",
          },
          { name: "filter", definition: "ParentImage: C:\\ok.exe\n" },
        ],
        matchCount: 2,
        nodeIds: ["n:record:0003", "n:process:0001"],
        edgeIds: ["e:0001"],
      },
    ],
    matches: [
      {
        rulePath: "process/synthetic.yml",
        record: sigmaLocator(3),
        matchedSelections: ["selection"],
        recordNode: {
          id: "n:record:0003",
          kind: "record",
          keyForm: "source_content_sha256_position",
          identity: [{ semantic: "record.position", value: "3" }],
          label: { rawText: "synthetic-events.xml:3", valueState: "present" },
          observation: "observed",
          creationRecord: "item_absent",
        },
      },
      {
        rulePath: "process/synthetic.yml",
        record: sigmaLocator(8),
        matchedSelections: ["selection"],
      },
    ],
    outsideGraphMatchCount: 0,
    unevaluatedRules: [
      {
        path: "other/keywords.yml",
        id: "00000000-0000-4000-8000-000000000002",
        title: "Synthetic Keyword Rule",
        reason: "keyword_search",
        detail: "selection keywords searches values without a field name",
      },
    ],
  };
}

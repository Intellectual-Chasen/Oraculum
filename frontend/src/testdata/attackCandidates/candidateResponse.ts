import type {
  AttackCandidateRule,
  AttackCandidatesResponse,
} from "@/shared/contracts/attackCandidates";
import type { RecordLocator } from "@/shared/contracts/common";
import type { GraphNode } from "@/shared/contracts/graph";
import type { EdgeAssignmentBasis } from "@/shared/contracts/graphDetail";

const sourceProcess: GraphNode = {
  id: "n:process:source",
  kind: "process",
  keyForm: "terminal_id_process_id",
  identity: [
    { semantic: "terminal.id", value: "host-a.example.test" },
    { semantic: "process.id", value: "p-a" },
  ],
  label: { rawText: "source.exe", valueState: "present" },
  observation: "observed",
  creationRecord: "present",
};

const sinkProcess: GraphNode = {
  ...sourceProcess,
  id: "n:process:sink",
  identity: [
    { semantic: "terminal.id", value: "host-a.example.test" },
    { semantic: "process.id", value: "p-b" },
  ],
  label: { rawText: "sink.exe", valueState: "present" },
};

const sourceTerminal: GraphNode = {
  id: "n:terminal:source",
  kind: "terminal",
  keyForm: "terminal_id",
  identity: [{ semantic: "terminal.id", value: "host-a.example.test" }],
  label: { rawText: "host-a.example.test", valueState: "present" },
  observation: "observed",
  creationRecord: "present",
};

const remoteTerminal: GraphNode = {
  ...sourceTerminal,
  id: "n:terminal:remote",
  identity: [{ semantic: "terminal.id", value: "host-b.example.test" }],
  label: { rawText: "host-b.example.test", valueState: "present" },
};

const clientIp: GraphNode = {
  id: "n:ip:client",
  kind: "ip",
  keyForm: "address",
  identity: [{ semantic: "connection.source_address", value: "192.0.2.44" }],
  label: { rawText: "192.0.2.44", valueState: "present" },
  observation: "observed",
  creationRecord: "present",
};

function locator(sourceFileName: string, lineNumber: number): RecordLocator {
  return {
    sourceId: `synthetic-${sourceFileName}`,
    sourceContentSha256: "a".repeat(64),
    sourceFileName,
    positionKind: "line_number",
    lineNumber,
    recordRawTextRef: `/api/v0/records?lineNumber=${lineNumber}`,
  };
}

const processEvidence = locator("synthetic.log", 7);
const sessionEvidence = locator("synthetic-session.log", 12);
const addressEvidence = locator("synthetic-address.log", 19);

const assignmentBasis: EdgeAssignmentBasis = {
  clientIp: "192.0.2.44",
  sourceId: "source-a",
  conditions: [
    {
      conditionKey: "terminal_ip_assignment",
      use: "used",
      leftValue: [
        {
          name: "clientIP",
          semantic: "connection.source_address",
          kind: "text",
          text: { rawText: "192.0.2.44", valueState: "present" },
        },
      ],
      rightValue: [
        {
          name: "clientTerminal",
          semantic: "terminal.id",
          kind: "text",
          text: {
            normalized: "host-a.example.test",
            derivation: "synthetic address assignment",
            valueState: "derived",
          },
        },
      ],
      assignmentValidRange: {
        from: {
          rawText: "2031-04-05T06:07:08Z",
          normalized: "2031-04-05T06:07:08Z",
          normalizedForm: "rfc3339_absolute",
          precision: "second",
          offsetState: "in_value",
          offsetText: "Z",
          clock: "terminal_local",
          meaning: "event",
          valueState: "present",
        },
        to: {
          rawText: "2031-04-05T06:17:08Z",
          normalized: "2031-04-05T06:17:08Z",
          normalizedForm: "rfc3339_absolute",
          precision: "second",
          offsetState: "in_value",
          offsetText: "Z",
          clock: "terminal_local",
          meaning: "event",
          valueState: "present",
        },
      },
      outsideAssignmentRange: false,
    },
  ],
  assumptions: [],
  clockDependencyNote: "Synthetic assignment basis for decoder coverage.",
};

type CandidateRule = Omit<AttackCandidateRule, "matchCount">;

export const candidateRule: CandidateRule = {
  id: "attack.t1055.process-injection",
  title: "Process Injection",
  description: "観測graphに明示的なprocess injection関係がある。",
  references: ["https://attack.mitre.org/techniques/T1055/"],
  attack: [{ id: "T1055", basis: "official" as const }],
  distinguishesFrom: [],
  variants: [
    {
      id: "default",
      platforms: [],
      rationale: "synthetic",
      evaluation: { state: "evaluated" as const },
      matchCount: 1,
    },
  ],
};

export const remoteServicesCandidateRule: CandidateRule = {
  id: "attack.t1021.remote-services",
  title: "Remote Services",
  description: "通信先との関係からRemote Servicesに関連する可能性がある候補。",
  references: ["https://attack.mitre.org/techniques/T1021/"],
  attack: [{ id: "T1021", basis: "official" as const }],
  distinguishesFrom: [],
  variants: [
    {
      id: "default",
      platforms: [],
      rationale: "synthetic",
      evaluation: { state: "evaluated" as const },
      matchCount: 1,
    },
  ],
};

export const candidateMatch: AttackCandidatesResponse["matches"][number] = {
  ruleId: candidateRule.id,
  variantId: "default",
  matchId: "match:injection:1",
  edges: [
    {
      role: "edge",
      evidenceRole: "required",
      kind: "process_injection",
      edgeId: "e:injection:1",
      sourceNode: sourceProcess,
      sinkNode: sinkProcess,
      assignmentBases: [],
      evidence: [processEvidence],
    },
  ],
};

export const remoteServicesCandidateMatch: AttackCandidatesResponse["matches"][number] =
  {
    ruleId: remoteServicesCandidateRule.id,
    variantId: "default",
    matchId: "match:remote-service:1",
    edges: [
      {
        role: "session",
        evidenceRole: "required",
        kind: "terminal_remote_session",
        edgeId: "e:remote-session:1",
        sourceNode: sourceTerminal,
        sinkNode: remoteTerminal,
        assignmentBases: [assignmentBasis],
        evidence: [sessionEvidence],
      },
      {
        role: "address",
        evidenceRole: "required",
        kind: "terminal_address",
        edgeId: "e:terminal-address:1",
        sourceNode: sourceTerminal,
        sinkNode: clientIp,
        assignmentBases: [],
        evidence: [addressEvidence],
      },
    ],
  };

export function candidateResponse(
  matches: AttackCandidatesResponse["matches"] = [candidateMatch],
  rules: CandidateRule[] = [candidateRule],
): AttackCandidatesResponse {
  return {
    ruleSet: {
      directory: "/rules/attack",
      fileCount: rules.length,
      contentSha256: "a".repeat(64),
      revision: "test",
      revisionSource: "test",
    },
    rules: rules.map((rule) => ({
      ...rule,
      matchCount: matches.filter((match) => match.ruleId === rule.id).length,
      variants: rule.variants.map((variant) => ({
        ...variant,
        matchCount: matches.filter(
          (match) => match.ruleId === rule.id && match.variantId === variant.id,
        ).length,
      })),
    })),
    matches,
    notEvaluated: [],
  };
}

import {
  decodeRecordLocator,
  decodeTimestamp,
  type RecordLocator,
  type Timestamp,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalBoolean,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireCount,
  requireEnum,
  requireString,
} from "./decoding";
import { decodeGraphNode, type GraphNode } from "./graph";

/** commit をどこから決めたか。`backend/pipeline/sigma_candidates.go` の SigmaRuleSetInfo。 */
export const sigmaRevisionSources = [
  "git_head",
  "argument",
  "unverified",
] as const;
export type SigmaRevisionSource = (typeof sigmaRevisionSources)[number];

/** 評価しなかった理由の分類。`backend/adapters/sigma` の Reason で始まる定数。 */
export const sigmaUnevaluatedReasons = [
  "yaml_unreadable",
  "not_a_detection_rule",
  "rule_collection",
  "logsource_unsupported",
  "keyword_search",
  "modifier_unsupported",
  "value_unsupported",
  "regex_unsupported",
  "condition_unsupported",
  "field_unavailable",
] as const;
export type SigmaUnevaluatedReason = (typeof sigmaUnevaluatedReasons)[number];

/** 評価に使ったルールの集合と、その commit。 */
export type SigmaRuleSet = {
  directory: string;
  revision?: string;
  revisionSource: SigmaRevisionSource;
  revisionDetail?: string;
  gitWorkTree?: string;
  contentSha256: string;
  ruleFileCount: number;
};

/**
 * どのルールも該当しえないレコードの分け方と件数。Channel の欄を持つレコードはチャネル、
 * 持たずに Provider の欄を持つレコードはプロバイダの文字列 (空の文字列を含む) で分ける。
 * どちらの欄も持たないレコードは channelAndProviderAbsent で表す。
 */
export type SigmaUnevaluatedRecordGroup =
  | { channel: string; recordCount: number }
  | { provider: string; recordCount: number }
  | { channelAndProviderAbsent: true; recordCount: number };

/** 検索 1 つの名前と、ルールの file の定義を YAML に書き直したもの。 */
export type SigmaSelection = { name: string; definition: string };

/** 1 件以上のレコードに一致したルール。 */
export type SigmaMatchedRule = {
  path: string;
  id: string;
  title: string;
  author: string;
  level: string;
  status: string;
  condition: string;
  selections: SigmaSelection[];
  matchCount: number;
  /** 一致したレコードのどれかを根拠に持つノード (エッジの端点を含む) とエッジ。 */
  nodeIds: string[];
  edgeIds: string[];
};

/** ルール 1 つに一致したレコード 1 件。 */
export type SigmaRuleMatch = {
  rulePath: string;
  record: RecordLocator;
  matchedSelections: string[];
  /**
   * レコードが記録した端末の名前か、レコードが端末を記録しないときに取り込みの起動で収集元に
   * 指定した端末の名前。後者のとき `terminalAssigned` が真である。どちらも無いときは出ない。
   */
  terminal?: string;
  terminalAssigned?: boolean;
  /** レコードの時刻。読めなかったときは出ない。 */
  eventTime?: Timestamp;
  /** 一致したレコードのノード。レコードがグラフに無いか、ノードを持たないときは出ない。 */
  recordNode?: GraphNode;
};

/** 評価しなかったルールの file 1 つ。 */
export type SigmaUnevaluatedRule = {
  path: string;
  id: string;
  title: string;
  reason: SigmaUnevaluatedReason;
  detail: string;
};

/** `backend/api/sigma_rule_candidates.go` の応答。 */
export type SigmaRuleCandidatesResponse = {
  ruleSet?: SigmaRuleSet;
  evaluatedRuleCount: number;
  evaluatedRecordCount: number;
  unevaluatedRecordGroups: SigmaUnevaluatedRecordGroup[];
  skippedPairCount: number;
  skippedPairRecordCount: number;
  recordsWithoutSemantics: number;
  rules: SigmaMatchedRule[];
  matches: SigmaRuleMatch[];
  /** 検索の条件を与えた要求で、グラフに無いレコードのため除いた一致の数。 */
  outsideGraphMatchCount: number;
  unevaluatedRules: SigmaUnevaluatedRule[];
};

const decodeRecordGroup: Decoder<SigmaUnevaluatedRecordGroup> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const recordCount = requireCount(source, "recordCount", path);
  const channel = optionalString(source, "channel", path);
  const provider = optionalString(source, "provider", path);
  const absent = optionalBoolean(source, "channelAndProviderAbsent", path);
  const kinds = [
    channel !== undefined,
    provider !== undefined,
    absent === true,
  ];
  if (kinds.filter(Boolean).length !== 1 || absent === false) {
    throw new DecodeFailure(
      path,
      "expected exactly one of channel, provider and channelAndProviderAbsent",
    );
  }
  if (channel !== undefined) return { channel, recordCount };
  if (provider !== undefined) return { provider, recordCount };
  return { channelAndProviderAbsent: true, recordCount };
};

const decodeRuleSet: Decoder<SigmaRuleSet> = (input, path) => {
  const source = readObject(input, path);
  return {
    directory: requireString(source, "directory", path),
    revision: optionalString(source, "revision", path),
    revisionSource: requireEnum(
      source,
      "revisionSource",
      path,
      sigmaRevisionSources,
    ),
    revisionDetail: optionalString(source, "revisionDetail", path),
    gitWorkTree: optionalString(source, "gitWorkTree", path),
    contentSha256: requireString(source, "contentSha256", path),
    ruleFileCount: requireCount(source, "ruleFileCount", path),
  };
};

const decodeSelection: Decoder<SigmaSelection> = (input, path) => {
  const source = readObject(input, path);
  return {
    name: requireString(source, "name", path),
    definition: requireString(source, "definition", path),
  };
};

const decodeRule: Decoder<SigmaMatchedRule> = (input, path) => {
  const source = readObject(input, path);
  return {
    path: requireString(source, "path", path),
    id: requireString(source, "id", path),
    title: requireString(source, "title", path),
    author: requireString(source, "author", path),
    level: requireString(source, "level", path),
    status: requireString(source, "status", path),
    condition: requireString(source, "condition", path),
    selections: requireArray(source, "selections", path, decodeSelection),
    matchCount: requireCount(source, "matchCount", path),
    nodeIds: requireArray(source, "nodeIds", path, decodeString),
    edgeIds: requireArray(source, "edgeIds", path, decodeString),
  };
};

const decodeMatch: Decoder<SigmaRuleMatch> = (input, path) => {
  const source = readObject(input, path);
  return {
    rulePath: requireString(source, "rulePath", path),
    record: decodeRecordLocator(source.record, `${path}.record`),
    matchedSelections: requireArray(
      source,
      "matchedSelections",
      path,
      decodeString,
    ),
    terminal: optionalString(source, "terminal", path),
    terminalAssigned: optionalBoolean(source, "terminalAssigned", path),
    eventTime: optionalMember(source, "eventTime", path, decodeTimestamp),
    recordNode: optionalMember(source, "recordNode", path, decodeGraphNode),
  };
};

const decodeUnevaluated: Decoder<SigmaUnevaluatedRule> = (input, path) => {
  const source = readObject(input, path);
  return {
    path: requireString(source, "path", path),
    id: requireString(source, "id", path),
    title: requireString(source, "title", path),
    reason: requireEnum(source, "reason", path, sigmaUnevaluatedReasons),
    detail: requireString(source, "detail", path),
  };
};

/**
 * Sigma のルールの候補の応答を検証する。ルールの path と、ルールの中の検索の名前が
 * 重ならないこと、一致がどれも一覧のルールとそのルールの検索を指し、ルールの件数が一致の
 * 数と等しいことを確かめる。
 */
export const decodeSigmaRuleCandidatesResponse: Decoder<
  SigmaRuleCandidatesResponse
> = (input, path) => {
  const source = readObject(input, path);
  const rules = requireArray(source, "rules", path, decodeRule);
  const matches = requireArray(source, "matches", path, decodeMatch);
  const counts = new Map<string, number>();
  const selectionNames = new Map<string, Set<string>>();
  for (const [index, rule] of rules.entries()) {
    if (counts.has(rule.path)) {
      throw new DecodeFailure(
        `${path}.rules[${index}].path`,
        "expected unique rule paths",
      );
    }
    counts.set(rule.path, 0);
    const names = new Set(rule.selections.map((selection) => selection.name));
    if (names.size !== rule.selections.length) {
      throw new DecodeFailure(
        `${path}.rules[${index}].selections`,
        "expected unique selection names",
      );
    }
    selectionNames.set(rule.path, names);
  }
  for (const [index, match] of matches.entries()) {
    const count = counts.get(match.rulePath);
    if (count === undefined) {
      throw new DecodeFailure(
        `${path}.matches[${index}].rulePath`,
        "expected a listed rule path",
      );
    }
    counts.set(match.rulePath, count + 1);
    const names = selectionNames.get(match.rulePath);
    if (!match.matchedSelections.every((name) => names?.has(name))) {
      throw new DecodeFailure(
        `${path}.matches[${index}].matchedSelections`,
        "expected selections of the matched rule",
      );
    }
  }
  for (const [index, rule] of rules.entries()) {
    if (rule.matchCount !== counts.get(rule.path)) {
      throw new DecodeFailure(
        `${path}.rules[${index}].matchCount`,
        "expected count of matches for rule",
      );
    }
  }
  return {
    ruleSet: optionalMember(source, "ruleSet", path, decodeRuleSet),
    evaluatedRuleCount: requireCount(source, "evaluatedRuleCount", path),
    evaluatedRecordCount: requireCount(source, "evaluatedRecordCount", path),
    unevaluatedRecordGroups: requireArray(
      source,
      "unevaluatedRecordGroups",
      path,
      decodeRecordGroup,
    ),
    skippedPairCount: requireCount(source, "skippedPairCount", path),
    skippedPairRecordCount: requireCount(
      source,
      "skippedPairRecordCount",
      path,
    ),
    recordsWithoutSemantics: requireCount(
      source,
      "recordsWithoutSemantics",
      path,
    ),
    rules,
    matches,
    outsideGraphMatchCount: requireCount(
      source,
      "outsideGraphMatchCount",
      path,
    ),
    unevaluatedRules: requireArray(
      source,
      "unevaluatedRules",
      path,
      decodeUnevaluated,
    ),
  };
};

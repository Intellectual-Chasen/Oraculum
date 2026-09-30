import {
  decodeRecordField,
  decodeRecordLocator,
  decodeTimestamp,
  type RecordField,
  type RecordLocator,
  type Timestamp,
} from "./common";
import {
  DecodeFailure,
  type Decoder,
  decodeString,
  optionalEnum,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireBoolean,
  requireCount,
  requireEnum,
  requireMember,
  requireString,
} from "./decoding";
import {
  decodeGraphEvidence,
  decodeGraphNode,
  type GraphEvidence,
  type GraphNode,
} from "./graph";

/** 端末のレコードの分類。定義元は `backend/core/terminal_view.go` の `TerminalCategories` である。 */
export const terminalCategories = [
  "remote_logon",
  "account_management",
  "program_execution",
  "task_service_registration",
  "powershell",
  "installation",
  "defense_evasion",
] as const;
export type TerminalCategory = (typeof terminalCategories)[number];

/** 端末の一覧の 1 行。定義元は `TerminalSummary` である。 */
export type TerminalSummary = {
  node: GraphNode;
  names: string[];
  operatingSystem?: string;
  recordCount: number;
  categorizedRecordCount: number;
};

export type TerminalsResponse = { terminals: TerminalSummary[] };

/** 定義元は `TerminalProfileEntry` である。 */
export type TerminalProfileEntry = {
  name: string;
  value: string;
  recordRef: RecordLocator;
};

/** 定義元は `TerminalNameEntry` である。 */
export type TerminalNameEntry = {
  name: string;
  first?: Timestamp;
  last?: Timestamp;
  recordRefs: RecordLocator[];
};

/** 定義元は `TerminalAddressPeriod` である。 */
export type TerminalAddressPeriod = {
  interface: string;
  values: TerminalProfileEntry[];
  from?: Timestamp;
  to?: Timestamp;
};

/** 定義元は `TerminalRemoteLogonSummary` である。 */
export type TerminalRemoteLogonSummary = {
  sourceIp: string;
  failureCount: number;
  successCount: number;
  connectionCount: number;
  attemptedAccounts: string[];
  loggedOnAccounts: string[];
  first?: Timestamp;
  last?: Timestamp;
  firstRecordRef?: RecordLocator;
  lastRecordRef?: RecordLocator;
};

/** 定義元は `TerminalCategoryCount` である。 */
export type TerminalCategoryCount = {
  category: TerminalCategory;
  recordCount: number;
  requiredSources: string[];
  presentSources: string[];
};

/** 定義元は `TerminalDetail` である。 */
export type TerminalDetail = {
  node: GraphNode;
  operatingSystem: TerminalProfileEntry[];
  names: TerminalNameEntry[];
  addresses: TerminalAddressPeriod[];
  timeZone: TerminalProfileEntry[];
  remoteLogons: TerminalRemoteLogonSummary[];
  categories: TerminalCategoryCount[];
};

/** 定義元は `TerminalEvent` である。 */
export type TerminalEvent = GraphEvidence & {
  categories: TerminalCategory[];
  otherEventTimes: Timestamp[];
  fields: RecordField[];
  recordNode?: GraphNode;
  namedNodes: GraphNode[];
  originalFileNameDiffers: boolean;
  logonOutcome?: RemoteLogonOutcome;
};

/** 遠隔のログオンの結果。定義元は `RemoteLogonOutcome` である。 */
export const remoteLogonOutcomes = [
  "failure",
  "success",
  "connection",
] as const;
export type RemoteLogonOutcome = (typeof remoteLogonOutcomes)[number];

/** 定義元は `backend/api/terminals.go` の `terminalEventsResponse` である。 */
export type TerminalEventsResponse = {
  terminalId: string;
  category?: TerminalCategory;
  sourceIp?: string;
  events: TerminalEvent[];
};

const decodeCategory: Decoder<TerminalCategory> = (input, path) => {
  const found = terminalCategories.find((category) => category === input);
  if (found === undefined) {
    throw new DecodeFailure(path, "expected a terminal category");
  }
  return found;
};

const decodeSummary: Decoder<TerminalSummary> = (input, path) => {
  const source = readObject(input, path);
  return {
    node: requireMember(source, "node", path, decodeGraphNode),
    names: requireArray(source, "names", path, decodeString),
    operatingSystem: optionalString(source, "operatingSystem", path),
    recordCount: requireCount(source, "recordCount", path),
    categorizedRecordCount: requireCount(
      source,
      "categorizedRecordCount",
      path,
    ),
  };
};

/** `TerminalsResponse` を検証する。 */
export const decodeTerminalsResponse: Decoder<TerminalsResponse> = (
  input,
  path,
) => ({
  terminals: requireArray(
    readObject(input, path),
    "terminals",
    path,
    decodeSummary,
  ),
});

const decodeProfileEntry: Decoder<TerminalProfileEntry> = (input, path) => {
  const source = readObject(input, path);
  return {
    name: requireString(source, "name", path),
    value: requireString(source, "value", path),
    recordRef: requireMember(source, "recordRef", path, decodeRecordLocator),
  };
};

const decodeNameEntry: Decoder<TerminalNameEntry> = (input, path) => {
  const source = readObject(input, path);
  return {
    name: requireString(source, "name", path),
    first: optionalMember(source, "first", path, decodeTimestamp),
    last: optionalMember(source, "last", path, decodeTimestamp),
    recordRefs: requireArray(source, "recordRefs", path, decodeRecordLocator),
  };
};

const decodeAddress: Decoder<TerminalAddressPeriod> = (input, path) => {
  const source = readObject(input, path);
  return {
    interface: requireString(source, "interface", path),
    values: requireArray(source, "values", path, decodeProfileEntry),
    from: optionalMember(source, "from", path, decodeTimestamp),
    to: optionalMember(source, "to", path, decodeTimestamp),
  };
};

const decodeRemoteLogon: Decoder<TerminalRemoteLogonSummary> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    sourceIp: requireString(source, "sourceIp", path),
    failureCount: requireCount(source, "failureCount", path),
    successCount: requireCount(source, "successCount", path),
    connectionCount: requireCount(source, "connectionCount", path),
    attemptedAccounts: requireArray(
      source,
      "attemptedAccounts",
      path,
      decodeString,
    ),
    loggedOnAccounts: requireArray(
      source,
      "loggedOnAccounts",
      path,
      decodeString,
    ),
    first: optionalMember(source, "first", path, decodeTimestamp),
    last: optionalMember(source, "last", path, decodeTimestamp),
    firstRecordRef: optionalMember(
      source,
      "firstRecordRef",
      path,
      decodeRecordLocator,
    ),
    lastRecordRef: optionalMember(
      source,
      "lastRecordRef",
      path,
      decodeRecordLocator,
    ),
  };
};

const decodeCategoryCount: Decoder<TerminalCategoryCount> = (input, path) => {
  const source = readObject(input, path);
  return {
    category: requireEnum(source, "category", path, terminalCategories),
    recordCount: requireCount(source, "recordCount", path),
    requiredSources: requireArray(
      source,
      "requiredSources",
      path,
      decodeString,
    ),
    presentSources: requireArray(source, "presentSources", path, decodeString),
  };
};

/** `TerminalDetail` を検証する。 */
export const decodeTerminalDetail: Decoder<TerminalDetail> = (input, path) => {
  const source = readObject(input, path);
  return {
    node: requireMember(source, "node", path, decodeGraphNode),
    operatingSystem: requireArray(
      source,
      "operatingSystem",
      path,
      decodeProfileEntry,
    ),
    names: requireArray(source, "names", path, decodeNameEntry),
    addresses: requireArray(source, "addresses", path, decodeAddress),
    timeZone: requireArray(source, "timeZone", path, decodeProfileEntry),
    remoteLogons: requireArray(source, "remoteLogons", path, decodeRemoteLogon),
    categories: requireArray(source, "categories", path, decodeCategoryCount),
  };
};

const decodeEvent: Decoder<TerminalEvent> = (input, path) => {
  const source = readObject(input, path);
  return {
    ...decodeGraphEvidence(input, path),
    categories: requireArray(source, "categories", path, decodeCategory),
    otherEventTimes: requireArray(
      source,
      "otherEventTimes",
      path,
      decodeTimestamp,
    ),
    fields: requireArray(source, "fields", path, decodeRecordField),
    recordNode: optionalMember(source, "recordNode", path, decodeGraphNode),
    namedNodes: requireArray(source, "namedNodes", path, decodeGraphNode),
    originalFileNameDiffers: requireBoolean(
      source,
      "originalFileNameDiffers",
      path,
    ),
    logonOutcome: optionalEnum(
      source,
      "logonOutcome",
      path,
      remoteLogonOutcomes,
    ),
  };
};

/** `TerminalEventsResponse` を検証する。 */
export const decodeTerminalEventsResponse: Decoder<TerminalEventsResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  return {
    terminalId: requireString(source, "terminalId", path),
    category: optionalEnum(source, "category", path, terminalCategories),
    sourceIp: optionalString(source, "sourceIp", path),
    events: requireArray(source, "events", path, decodeEvent),
  };
};

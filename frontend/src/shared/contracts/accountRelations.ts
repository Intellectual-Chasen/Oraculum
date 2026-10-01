import { decodeTimestamp, type Timestamp } from "./common";
import {
  type Decoder,
  decodeString,
  optionalArray,
  optionalCount,
  optionalMember,
  readObject,
  requireArray,
  requireCount,
  requireMember,
  requireString,
} from "./decoding";
import {
  decodeEdgeKind,
  decodeGraphNode,
  decodeRecordSummary,
  type EdgeKind,
  type GraphNode,
  type RecordSummary,
} from "./graph";

export type AccountRelationKey = {
  counterpartId: string;
  originRole: EdgeKind;
  otherRole: EdgeKind;
};
export type AccountRelationEvent = {
  category: string;
  action: string;
  count: number;
};
export type AccountRelationSource = { sourceId: string; count: number };
export type AccountRelation = AccountRelationKey & {
  counterpart: GraphNode;
  recordCount: number;
  firstTime?: Timestamp;
  lastTime?: Timestamp;
  unknownTimeCount: number;
  events: AccountRelationEvent[];
  sources: AccountRelationSource[];
  sigmaRulePaths: string[];
};
export type AccountRelationRecord = {
  nodeId: string;
  originEdgeId: string;
  otherEdgeId: string;
  summary: RecordSummary;
  sigmaRulePaths: string[];
};
export type AccountRelationsResponse = {
  origin: GraphNode;
  groups: AccountRelation[];
  records?: AccountRelationRecord[];
  selectedRecordCount: number;
  nextOffset?: number;
  periodUnjudgedRecordCount: number;
};

const decodeEvent: Decoder<AccountRelationEvent> = (input, path) => {
  const source = readObject(input, path);
  return {
    category: requireString(source, "category", path),
    action: requireString(source, "action", path),
    count: requireCount(source, "count", path),
  };
};
const decodeSource: Decoder<AccountRelationSource> = (input, path) => {
  const source = readObject(input, path);
  return {
    sourceId: requireString(source, "sourceId", path),
    count: requireCount(source, "count", path),
  };
};
const decodeRelation: Decoder<AccountRelation> = (input, path) => {
  const source = readObject(input, path);
  return {
    counterpartId: requireString(source, "counterpartId", path),
    originRole: requireMember(source, "originRole", path, decodeEdgeKind),
    otherRole: requireMember(source, "otherRole", path, decodeEdgeKind),
    counterpart: requireMember(source, "counterpart", path, decodeGraphNode),
    recordCount: requireCount(source, "recordCount", path),
    firstTime: optionalMember(source, "firstTime", path, decodeTimestamp),
    lastTime: optionalMember(source, "lastTime", path, decodeTimestamp),
    unknownTimeCount: requireCount(source, "unknownTimeCount", path),
    events: requireArray(source, "events", path, decodeEvent),
    sources: requireArray(source, "sources", path, decodeSource),
    sigmaRulePaths: requireArray(source, "sigmaRulePaths", path, decodeString),
  };
};
const decodeRecord: Decoder<AccountRelationRecord> = (input, path) => {
  const source = readObject(input, path);
  return {
    nodeId: requireString(source, "nodeId", path),
    originEdgeId: requireString(source, "originEdgeId", path),
    otherEdgeId: requireString(source, "otherEdgeId", path),
    summary: requireMember(source, "summary", path, decodeRecordSummary),
    sigmaRulePaths: requireArray(source, "sigmaRulePaths", path, decodeString),
  };
};
export const decodeAccountRelationsResponse: Decoder<
  AccountRelationsResponse
> = (input, path) => {
  const source = readObject(input, path);
  return {
    origin: requireMember(source, "origin", path, decodeGraphNode),
    groups: requireArray(source, "groups", path, decodeRelation),
    records: optionalArray(source, "records", path, decodeRecord),
    selectedRecordCount: requireCount(source, "selectedRecordCount", path),
    nextOffset: optionalCount(source, "nextOffset", path),
    periodUnjudgedRecordCount: requireCount(
      source,
      "periodUnjudgedRecordCount",
      path,
    ),
  };
};

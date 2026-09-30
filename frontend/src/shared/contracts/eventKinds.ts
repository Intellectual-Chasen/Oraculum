import { type CaseId, decodeCaseId } from "./cases";
import {
  DecodeFailure,
  type Decoder,
  optionalBoolean,
  optionalMember,
  optionalString,
  readObject,
  requireArray,
  requireCount,
  requireString,
} from "./decoding";

/** `backend/api/event_kinds.go` の `eventKindItem`。 */
export type EventKind = {
  category: string;
  /** 動作の欄を持たないレコードの組では出ない。 */
  action?: string;
  recordCount: number;
  /** 分類がプロバイダの名前、動作がイベント ID である Windows イベントログの組か。 */
  windowsEvent: boolean;
};

/** `backend/api/event_kinds.go` の `eventKindsResponse`。 */
export type EventKindsResponse = {
  kinds: EventKind[];
  uncategorizedRecordCount: number;
  case?: CaseId;
  terminal?: string;
  /** 収集元で絞った要求のとき出る。その収集元の取り込めたレコードだけを数える。 */
  sourceId?: string;
};

const decodeEventKind: Decoder<EventKind> = (input, path) => {
  const source = readObject(input, path);
  const recordCount = requireCount(source, "recordCount", path);
  if (recordCount === 0) {
    throw new DecodeFailure(`${path}.recordCount`, "expected at least 1");
  }
  const action = optionalString(source, "action", path);
  return {
    category: requireString(source, "category", path),
    ...(action === undefined ? {} : { action }),
    recordCount,
    windowsEvent: optionalBoolean(source, "windowsEvent", path) ?? false,
  };
};

/** 事象の種別の一覧の応答を検証して画面用の値へ変換する。 */
export const decodeEventKindsResponse: Decoder<EventKindsResponse> = (
  input,
  path,
) => {
  const source = readObject(input, path);
  const kinds = requireArray(source, "kinds", path, decodeEventKind);
  const seen = new Set<string>();
  for (const [index, kind] of kinds.entries()) {
    const key = JSON.stringify([kind.category, kind.action ?? null]);
    if (seen.has(key)) {
      throw new DecodeFailure(
        `${path}.kinds[${index}]`,
        "expected unique category and action",
      );
    }
    seen.add(key);
  }
  const caseId = optionalMember(source, "case", path, decodeCaseId);
  const terminal = optionalString(source, "terminal", path);
  const sourceId = optionalString(source, "sourceId", path);
  return {
    ...(sourceId === undefined ? {} : { sourceId }),
    kinds,
    uncategorizedRecordCount: requireCount(
      source,
      "uncategorizedRecordCount",
      path,
    ),
    ...(caseId === undefined ? {} : { case: caseId }),
    ...(terminal === undefined ? {} : { terminal }),
  };
};

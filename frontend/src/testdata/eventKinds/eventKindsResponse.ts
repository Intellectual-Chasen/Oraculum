/**
 * 事象の種別の一覧の応答。分類 ps の 2 組と net と file の 1 組ずつを持ち、分類を持たない
 * レコードも数える。
 */
export function eventKindsResponseJson() {
  return {
    kinds: [
      { category: "net", action: "con", recordCount: 12 },
      { category: "ps", action: "start", recordCount: 5 },
      { category: "ps", action: "stop", recordCount: 4 },
      { category: "file", action: "create", recordCount: 3 },
    ],
    uncategorizedRecordCount: 2,
  };
}

import type {
  AssertionRecordRef,
  AssertionTarget,
} from "@/shared/contracts/assertions";
import { listKey } from "@/shared/lib/listKey";

/**
 * 対象 1 つを探せる文字列を返す。同じ対象を指す 2 つの組は同じ文字列になる。
 *
 * **判定は `sameAssertionTarget` と同じである。** 対象の種別が要する項目を欠いた組は
 * 文字列を持たない。値を持たない項目どうしの一致で、対象を欠いた組が任意の対象へ一致する
 * 形にしない。位置の値は収集元が持つものをすべて使う
 * (`backend/core/assertion.go` の `AssertionRecordRef`)。
 *
 * **一覧を対象で探せる形にするために置く。** 所見の一覧は全件を含み、画面はノードの
 * 詳細と関係の詳細の双方で同じ一覧を読む。対象ごとに一覧を走査すると、描画のたびに
 * 所見の件数と画面上の対象の個数の積だけ比較が走る。
 */
export function assertionTargetKey(
  target: AssertionTarget,
): string | undefined {
  switch (target.kind) {
    case "node":
      return target.nodeId === undefined
        ? undefined
        : listKey(["node", target.nodeId]);
    case "edge":
      return target.edge === undefined
        ? undefined
        : listKey([
            "edge",
            target.edge.kind,
            target.edge.sourceNodeId,
            target.edge.targetNodeId,
          ]);
    case "record":
      return recordTargetKey(target.record);
    case "source":
      return target.sourceContentSha256 === undefined
        ? undefined
        : listKey(["source", target.sourceContentSha256]);
    default: {
      const exhaustive: never = target.kind;
      throw new Error(`unknown assertion target kind: ${String(exhaustive)}`);
    }
  }
}

function recordTargetKey(
  record: AssertionRecordRef | undefined,
): string | undefined {
  if (record === undefined) {
    return undefined;
  }
  return listKey([
    "record",
    record.sourceContentSha256,
    record.positionKind,
    record.sequenceNumber,
    record.lineNumber,
    record.byteOffset,
  ]);
}

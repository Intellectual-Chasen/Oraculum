import { useEffect, useMemo } from "react";
import { buildFetchFailure } from "@/shared/api/apiFailure";
import type { MatchConditionSelection } from "@/shared/api/matchConditions";
import type { RecordPosition, RecordRequest } from "@/shared/api/records";
import type { RecordLocator } from "@/shared/contracts/common";
import type {
  DerivationTrail,
  RecordResponse,
} from "@/shared/contracts/records";
import { memberAt } from "@/shared/contracts/sources";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import {
  describeSpan,
  recordPositionAbsent,
  recordPositionParts,
} from "@/shared/lib/recordPosition";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { Highlighted } from "@/shared/ui/Highlighted";
import { Hint } from "@/shared/ui/Hint";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { ObservationKindView } from "@/shared/ui/ObservationKindView";
import { RawText } from "@/shared/ui/RawText";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { DerivationTrailView } from "./DerivationTrailView";
import { RecordFieldTable } from "./RecordFieldTable";
import { useRecordDetail } from "./useRecordDetail";

const recordCountUndetermined = "件数未確定";

function RecordIdentity({ record }: { record: RecordResponse }) {
  const { recordRef, sourceIdentity } = record;
  const positionParts = recordPositionParts(recordRef);
  return (
    <table className="fit-table">
      <caption className="sr-only">レコードの収集元と位置</caption>
      <colgroup>
        <col className="w-36" />
        <col />
      </colgroup>
      <tbody>
        <tr>
          <th scope="row">収集元</th>
          <td>
            <RawText text={sourceIdentity.fileName} />
          </td>
        </tr>
        <tr>
          <th scope="row">SHA-256</th>
          <td className="truncate font-mono">
            <Hint text={sourceIdentity.contentSha256}>
              {sourceIdentity.contentSha256}
            </Hint>
          </td>
        </tr>
        <tr>
          <th scope="row">収集元のレコード</th>
          <td>
            {sourceIdentity.recordCount === undefined ? (
              <MissingValue description={recordCountUndetermined} />
            ) : (
              formatCount(sourceIdentity.recordCount)
            )}
          </td>
        </tr>
        <tr>
          <th scope="row">位置</th>
          <td className="font-mono">
            {positionParts.length === 0 ? (
              <MissingValue description={recordPositionAbsent} />
            ) : (
              <KeyValueList
                pairs={positionParts.map((part) => ({
                  name: part.name,
                  value: part.value,
                }))}
              />
            )}
          </td>
        </tr>
        <MemberPositionRow record={record} />
        <tr>
          <th scope="row">イベントの種類</th>
          <td>
            <ObservationKindView observationKind={record.observationKind} />
          </td>
        </tr>
      </tbody>
    </table>
  );
}

/**
 * 複数の file を連結した収集元で、レコードの byte 位置がどの file のどこに該当するかを出す。
 * 構成の file を持たない収集元と、byte 位置を持たないレコードでは行を出さない。
 */
function MemberPositionRow({ record }: { record: RecordResponse }) {
  const { recordRef, sourceIdentity } = record;
  if (recordRef.byteOffset === undefined) {
    return null;
  }
  const member = memberAt(sourceIdentity.members, recordRef.byteOffset);
  if (member === undefined) {
    return null;
  }
  return (
    <tr>
      <th scope="row">位置を持つ file</th>
      <td>
        <RawText text={member.originPath} />{" "}
        <span className="font-mono">
          {describeSpan(
            "位置",
            recordRef.byteOffset - member.byteOffset,
            recordRef.byteLength,
          )}
        </span>
      </td>
    </tr>
  );
}

function RecordBody({ record }: { record: RecordResponse }) {
  return (
    <>
      <RecordIdentity record={record} />
      <div className="flex items-center gap-2">
        <h3>原文</h3>
        {record.sourceIdentity.rawTextConverted === true ? (
          <StatusLabel
            status="pending"
            label="変換した原文"
            details={
              <KeyValueList
                stacked
                pairs={[
                  {
                    name: "原文",
                    value: "収集元の byte 列から読み取りが組み立てた文字列",
                  },
                  { name: "byte 列の位置", value: "位置の行" },
                ]}
              />
            }
          />
        ) : null}
      </div>
      {/* 原文の連続した空白を 1 つにまとめない。 */}
      <pre>
        <Highlighted text={record.rawText} />
      </pre>
      <RecordFieldTable fields={record.fields} eventKind={record.eventKind} />
    </>
  );
}

function positionOf(locator: RecordLocator): RecordPosition {
  return {
    sourceId: locator.sourceId,
    sourceContentSha256: locator.sourceContentSha256,
    sequenceNumber: locator.sequenceNumber,
    lineNumber: locator.lineNumber,
    byteOffset: locator.byteOffset,
  };
}

/** 開いたレコードへ至った経路を求める起点。関連付けの候補から開いたときだけ渡す。 */
export type TrailOrigin = {
  /** 関連付けの起点のレコード。 */
  origin: RecordLocator;
  /** 経路を組む関連付けの条件。 */
  matchConditions: MatchConditionSelection;
  /**
   * 端末の割当と時刻の解釈を記録した回数。backend はそのたびにグラフを組み直すため、
   * 値が変わると経路を取り直す。
   */
  dataVersion: number;
};

const trailFailureSummary = "到達した経路の構築";

/**
 * 経路を求めた応答の状態を、経路の状態へ直す。
 * **失敗を経路の失敗として出す。** レコードの表示は別の要求が持つ。
 */
function trailStateOf(
  state: FetchState<RecordResponse>,
): FetchState<DerivationTrail> {
  switch (state.status) {
    case "failed":
      return {
        status: "failed",
        failure: { ...state.failure, summary: trailFailureSummary },
      };
    case "loaded":
      return state.value.derivationTrail === undefined
        ? {
            status: "failed",
            failure: buildFetchFailure(
              "response_unreadable",
              trailFailureSummary,
            ),
          }
        : { status: "loaded", value: state.value.derivationTrail };
    default:
      return state;
  }
}

/**
 * 起点から開いたレコードへ至った経路を、レコードとは別の要求で取って出す。
 *
 * **経路の失敗でレコードの表示を失わない。** 起点が無い・条件の組み立てに失敗した要求でも、
 * レコードの原文と項目は起点なしの要求で読める。
 */
function TrailSection({
  recordRef,
  trailOrigin,
}: {
  recordRef: RecordLocator;
  trailOrigin: TrailOrigin;
}) {
  const request = useMemo<RecordRequest>(
    () => ({
      record: positionOf(recordRef),
      origin: {
        record: positionOf(trailOrigin.origin),
        matchConditions: trailOrigin.matchConditions,
      },
    }),
    [recordRef, trailOrigin],
  );
  const state = useRecordDetail(request);
  const trailState = useMemo(() => trailStateOf(state), [state]);
  return (
    <FetchStateView
      state={trailState}
      loadingDescription="到達した経路の構築中"
    >
      {(trail) => <DerivationTrailView trail={trail} />}
    </FetchStateView>
  );
}

function SelectedRecord({
  recordRef,
  trailOrigin,
  onRecordLoad,
}: {
  recordRef: RecordLocator;
  trailOrigin: TrailOrigin | undefined;
  onRecordLoad: ((record: RecordResponse) => void) | undefined;
}) {
  const request = useMemo<RecordRequest>(
    () => ({ record: positionOf(recordRef) }),
    [recordRef],
  );
  const state = useRecordDetail(request);
  useEffect(() => {
    if (state.status === "loaded") onRecordLoad?.(state.value);
  }, [state, onRecordLoad]);

  return (
    <>
      <FetchStateView state={state} loadingDescription="レコードの読み込み中">
        {(record) => <RecordBody record={record} />}
      </FetchStateView>
      {trailOrigin === undefined ? null : (
        <TrailSection recordRef={recordRef} trailOrigin={trailOrigin} />
      )}
    </>
  );
}

/**
 * 選んだレコードの原文と項目を出す (操作 3 と操作 7)。
 * 起点を渡したときは、起点と関連付けの条件を付けた別の要求で到達した経路を取って出す。
 */
export function RecordDetail({
  recordRef,
  trailOrigin,
  onRecordLoad,
}: {
  /** 表示するレコードの位置。状態の所有者は上位の画面である。 */
  recordRef: RecordLocator | undefined;
  /** 経路を求める起点。関連付けの候補から開いたときだけ渡す。 */
  trailOrigin?: TrailOrigin;
  /** レコードを読み込んだときに、読んだ応答を知らせる。 */
  onRecordLoad?: (record: RecordResponse) => void;
}) {
  return (
    <section aria-labelledby="record-detail-heading">
      <h2 id="record-detail-heading" className="sr-only">
        レコード
      </h2>
      {recordRef === undefined ? (
        <KeyValueList pairs={[{ name: "レコード", value: "未選択" }]} />
      ) : (
        <SelectedRecord
          recordRef={recordRef}
          trailOrigin={trailOrigin}
          onRecordLoad={onRecordLoad}
        />
      )}
    </section>
  );
}

import { stageKeys } from "@/shared/contracts/candidates";
import type {
  DerivationTrail,
  TrailInputRef,
  TrailStep,
} from "@/shared/contracts/records";
import { listKey, recordLocatorKey } from "@/shared/lib/listKey";
import { stageKeyLabels } from "@/shared/lib/matchLabels";
import { describeRecordLocation } from "@/shared/lib/recordPosition";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";

const inputRefsAbsent = "原文の外の入力のみ";

function inputRefKey(inputRef: TrailInputRef): string {
  return inputRef.kind === "record"
    ? listKey(["record", recordLocatorKey(inputRef.record)])
    : listKey(["source", inputRef.source.sourceId]);
}

function TrailInput({ inputRef }: { inputRef: TrailInputRef }) {
  if (inputRef.kind === "record") {
    return <RawText text={describeRecordLocation(inputRef.record)} />;
  }
  return (
    <KeyValueList
      pairs={[
        { name: "収集元", value: <RawText text={inputRef.source.fileName} /> },
        { name: "範囲", value: "すべてのレコード" },
        {
          name: "SHA-256",
          value: (
            <span className="font-mono">{inputRef.source.contentSha256}</span>
          ),
        },
      ]}
    />
  );
}

function TrailInputCell({ step }: { step: TrailStep }) {
  const inputRefs = step.inputRefs;
  if (inputRefs === undefined) {
    return (
      <td>
        <MissingValue description={inputRefsAbsent} />
      </td>
    );
  }
  return (
    <td>
      <ul>
        {inputRefs.map((inputRef) => (
          <li key={inputRefKey(inputRef)}>
            <TrailInput inputRef={inputRef} />
          </li>
        ))}
      </ul>
    </td>
  );
}

/**
 * 段階の名前を出す。推定の段階の種類は画面の呼び名で出し、それ以外の段階の名前は応答の文字列の
 * まま出す。
 */
function StepKeyText({ stepKey }: { stepKey: string }) {
  const stageKey = stageKeys.find((key) => key === stepKey);
  return stageKey === undefined ? (
    <RawText text={stepKey} />
  ) : (
    stageKeyLabels[stageKey]
  );
}

function TrailStepRow({ step }: { step: TrailStep }) {
  return (
    <tr>
      <th scope="row">
        <StepKeyText stepKey={step.stepKey} />
      </th>
      <TrailInputCell step={step} />
      <td>
        <ul>
          {step.usedIdentifiers.map((identifier) => (
            <li key={identifier}>
              <RawText text={identifier} />
            </li>
          ))}
        </ul>
      </td>
      <td>
        <RawText text={step.output} />
      </td>
    </tr>
  );
}

/**
 * 起点のレコードから開いたレコードまでの各段階を、応答が並べた順に出す。
 * 並び順は `stepKey` の昇順と一致する
 * (定義元は `backend/core/record_locator.go` の `DerivationTrail`)。
 */
export function DerivationTrailView({ trail }: { trail: DerivationTrail }) {
  const { stoppedAt } = trail;
  return (
    <section aria-labelledby="record-derivation-trail-heading">
      <h3 id="record-derivation-trail-heading">到達した経路</h3>
      <KeyValueList
        pairs={[
          {
            name: "起点のレコード",
            value: <RawText text={describeRecordLocation(trail.originRef)} />,
          },
        ]}
      />
      <table>
        <caption className="sr-only">到達した経路の段階</caption>
        <thead>
          <tr>
            <th scope="col">段階</th>
            <th scope="col">入力</th>
            <th scope="col">用いた値</th>
            <th scope="col">出力</th>
          </tr>
        </thead>
        <tbody>
          {trail.steps.map((step) => (
            <TrailStepRow key={step.stepKey} step={step} />
          ))}
        </tbody>
      </table>
      <KeyValueList
        pairs={
          stoppedAt === undefined
            ? [{ name: "進めなかった段階", value: "なし" }]
            : [
                {
                  name: "進めなかった段階",
                  value: <StepKeyText stepKey={stoppedAt.stepKey} />,
                },
                { name: "出力", value: <RawText text={stoppedAt.output} /> },
              ]
        }
      />
    </section>
  );
}

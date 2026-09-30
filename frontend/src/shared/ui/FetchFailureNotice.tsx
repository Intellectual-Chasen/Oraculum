import { type FetchFailure, fetchFailureKindLabels } from "../lib/fetchState";
import { KeyValueList } from "./KeyValueList";
import { RawText } from "./RawText";
import { StatusLabel } from "./StatusLabel";

/**
 * 取得の失敗 1 件を表示する。失敗した操作の名前と、失敗の種類の短いラベルを出し、理由・次の
 * 操作・コード・収集元などの詳細は「名前: 値」の組にしてラベルの tooltip に入れる。
 *
 * 失敗に関わるレコード位置を出さない。`backend/core/api_error.go` の `ApiError` は
 * `recordRef` を省略可として持ち、`recordRef` を載せた失敗の応答を返す経路が backend に
 * 無い。読む側は `decodeApiError` が持ち続ける。
 */
export function FetchFailureNotice({ failure }: { failure: FetchFailure }) {
  const kindLabel =
    failure.failureDescription ?? fetchFailureKindLabels[failure.kind];
  // ラベルには最も詳しい理由を出す。読み込みを拒否された理由と検索式の誤りは種類より詳しい。
  const label =
    failure.rejectionDescription ??
    failure.searchExpressionError?.description ??
    kindLabel;
  return (
    <div role="alert" className="flex flex-wrap items-center gap-x-2">
      <span className="font-medium">{failure.summary}</span>
      <StatusLabel
        status="failed"
        label={label}
        details={
          <KeyValueList
            stacked
            pairs={[
              {
                name: "種類",
                value: label === kindLabel ? undefined : kindLabel,
              },
              { name: "次の操作", value: failure.nextAction },
              { name: "コード", value: failure.failureCode },
              { name: "収集元", value: failure.sourceId },
              { name: "SHA-256", value: failure.sourceContentSha256 },
              {
                name: "path",
                value:
                  failure.originPath === undefined ? undefined : (
                    <RawText text={failure.originPath} />
                  ),
              },
            ]}
          />
        }
      />
    </div>
  );
}

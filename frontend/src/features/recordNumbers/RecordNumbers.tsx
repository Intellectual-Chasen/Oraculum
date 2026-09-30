import { ArrowDownToLine, ArrowUpToLine } from "lucide-react";
import type { RecordLocator } from "@/shared/contracts/common";
import type {
  RecordNumbers as Numbers,
  RecordComparison,
  RecordComparisonKey,
  RecordComparisonUnequalKey,
  RecordNotComparedReason,
  RecordNumberFileHeader,
  RecordNumberGap,
  RecordNumberHeaderComparison,
  RecordNumberNotExaminedReason,
  RecordNumberStream,
  UnequalKeyOutcome,
} from "@/shared/contracts/recordNumbers";
import type { SourceIdentity } from "@/shared/contracts/sources";
import { formatCount } from "@/shared/lib/format";
import {
  describeRecordPosition,
  recordRefKey,
} from "@/shared/lib/recordPosition";
import { DataTable, type DataTableColumn } from "@/shared/ui/DataTable";
import { FetchStateView } from "@/shared/ui/FetchStateView";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList, type KeyValuePair } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import type { Status } from "@/shared/ui/StatusDot";
import { StatusLabel } from "@/shared/ui/StatusLabel";
import { useRecordNumbers } from "./useRecordNumbers";

const notExaminedLabels: Record<RecordNumberNotExaminedReason, string> = {
  record_numbers_not_declared: "入力形式が EventRecordID の確認対象外",
  record_numbers_unreadable: "番号を読み取れたレコードなし",
};

/** ファイルの見出しの番号とレコードの見出しの番号の比較の状態と、比べなかった理由。 */
const headerComparisons: Record<
  RecordNumberHeaderComparison,
  { status: Status; label: string; reason?: string }
> = {
  agrees: { status: "done", label: "一致" },
  differs: { status: "failed", label: "不一致" },
  no_record_numbers: {
    status: "idle",
    label: "未比較",
    reason: "レコードの見出しの番号を読み取れたレコードなし",
  },
  header_unreadable: {
    status: "idle",
    label: "未比較",
    reason: "ファイルの見出しの次のレコード番号の読み取り不可",
  },
};

const comparisonKeyLabels: Record<RecordComparisonKey, string> = {
  channel_computer_record_number: "チャネル・Computer・EventRecordID",
  second_provider_event_id: "秒単位の UTC 時刻・プロバイダ・イベント ID",
};

/** 鍵の補足。鍵の横の help に「名前: 値」の組で出す。 */
const comparisonKeyNotes: Record<RecordComparisonKey, KeyValuePair[]> = {
  channel_computer_record_number: [],
  second_provider_event_id: [
    { name: "秒未満", value: "切り捨て" },
    { name: "タイムゾーン", value: "収集元のタイムゾーン" },
  ],
};

const notComparedLabels: Record<RecordNotComparedReason, string> = {
  comparison_key_not_declared: "入力形式に突き合わせの鍵のフィールドの宣言なし",
  time_offset_undetermined: "UTC 時刻を持つレコードのない収集元あり",
};

/** 比べなかった理由ごとの、比べられるようにする操作。 */
const notComparedNextActions: Partial<Record<RecordNotComparedReason, string>> =
  {
    time_offset_undetermined: "収集元のタイムゾーンの記録",
  };

/** 鍵ごとの、相手の記録期間の定義。記録期間の help に「名前: 値」の組で出す。 */
const rangeDefinitions: Record<RecordComparisonKey, KeyValuePair[]> = {
  channel_computer_record_number: [
    { name: "範囲", value: "相手のチャネルごとの番号の最小-最大" },
    { name: "両端", value: "範囲に含む" },
    { name: "端末の名前", value: "区別なし" },
    { name: "最小と最大", value: "相手の収集元のレコードの番号の表と同じ値" },
  ],
  second_provider_event_id: [
    {
      name: "範囲",
      value: "相手の鍵を持つレコードのプロバイダごとの最初の秒-最後の秒",
    },
    { name: "両端", value: "範囲に含む" },
  ],
};

type RecordNumbersProps = {
  /** 収集元の一覧で選んでいる収集元。 */
  selectedSource: SourceIdentity | undefined;
  /** 比べる相手の選択肢。取り込んだ収集元の全件。 */
  sources: SourceIdentity[];
  /**
   * 比べる相手に選んでいる収集元の `sourceId`。空文字列は比べないことを表す。状態の所有者は
   * 上位の画面であり、本欄を閉じて開き直しても選択が残る。
   */
  comparedSourceId: string;
  onSelectComparedSource: (sourceId: string) => void;
  /** タイムゾーンと端末の割り当てを記録した回数。変わると取り直す。 */
  dataVersion: number;
  onSelectRecord: (recordRef: RecordLocator) => void;
};

/** 選んだ収集元の EventRecordID の抜けと、別の収集元との突き合わせを出す。 */
export function RecordNumbers({
  selectedSource,
  sources,
  comparedSourceId,
  onSelectComparedSource,
  dataVersion,
  onSelectRecord,
}: RecordNumbersProps) {
  const compared = sources.find(
    (source) =>
      source.sourceId === comparedSourceId &&
      source.sourceId !== selectedSource?.sourceId,
  );
  const state = useRecordNumbers(selectedSource, compared, dataVersion);
  return (
    <section aria-labelledby="record-numbers-heading">
      <h2 id="record-numbers-heading">レコードの番号</h2>
      {/* state は収集元を選んでいないときだけ undefined になる。選んだ直後は loading を持つ。 */}
      {selectedSource === undefined || state === undefined ? (
        <div className="flex items-center gap-1">
          <KeyValueList pairs={[{ name: "収集元", value: "未選択" }]} />
          <HelpPopover label="レコードの番号">
            収集元の一覧で選択した収集元の番号の抜け
          </HelpPopover>
        </div>
      ) : (
        <>
          <label>
            比べる収集元{" "}
            <select
              value={compared?.sourceId ?? ""}
              onChange={(event) => onSelectComparedSource(event.target.value)}
            >
              <option value="">比べない</option>
              {sources
                .filter((source) => source.sourceId !== selectedSource.sourceId)
                .map((source) => (
                  <option key={source.sourceId} value={source.sourceId}>
                    {source.originPath}
                  </option>
                ))}
            </select>
          </label>
          <FetchStateView
            state={state}
            loadingDescription="レコードの番号の確認中"
          >
            {(response) => (
              <>
                <NumbersView
                  numbers={response.recordNumbers}
                  onSelectRecord={onSelectRecord}
                />
                {response.comparison !== undefined && compared !== undefined ? (
                  <ComparisonView
                    comparison={response.comparison}
                    source={selectedSource}
                    compared={compared}
                    onSelectRecord={onSelectRecord}
                  />
                ) : null}
              </>
            )}
          </FetchStateView>
        </>
      )}
    </section>
  );
}

/**
 * 一覧に出す要素の上限。応答は backend の上限 (`listLimit`) で切った配列と全件数を含み、
 * 画面は応答の配列のうち先頭の本上限の件数までを描く。
 */
const shownListLimit = 200;

/** 全件数が表示した件数を超えるときの「表示: 表示した件数 / 全件数」の組。 */
function limitPair(shown: number, total: number): KeyValuePair {
  return {
    name: "表示",
    value:
      total > shown
        ? `${formatCount(shown)} / ${formatCount(total)}`
        : undefined,
  };
}

/** 番号の範囲を「最小-最大」で書く。1 つの番号のときは番号だけを書く。 */
function numberSpan(first: number | string, last: number | string): string {
  return first === last ? `${first}` : `${first}-${last}`;
}

function NumbersView({
  numbers,
  onSelectRecord,
}: {
  numbers: Numbers;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  return (
    <>
      <div role="status">
        <KeyValueList
          pairs={[
            {
              name: "番号の抜け",
              value: <ExaminationLabel numbers={numbers} />,
            },
            {
              name: "番号を読み取れなかったレコード",
              value:
                numbers.unreadableRecordCount !== undefined &&
                numbers.unreadableRecordCount > 0
                  ? formatCount(numbers.unreadableRecordCount)
                  : undefined,
            },
          ]}
        />
      </div>
      {numbers.streams.length > 0 ? (
        <table>
          <caption className="sr-only">チャネルごとの番号</caption>
          <thead>
            <tr>
              <th scope="col">チャネル</th>
              <th scope="col">端末ごとの件数</th>
              <th scope="col">番号を読み取れたレコード</th>
              <th scope="col">番号の範囲</th>
              <th scope="col">抜けた番号</th>
              <th scope="col">同じ番号を持つレコード</th>
            </tr>
          </thead>
          <tbody>
            {numbers.streams.map((stream) => (
              <StreamRow
                key={stream.channel ?? ""}
                stream={stream}
                onSelectRecord={onSelectRecord}
              />
            ))}
          </tbody>
        </table>
      ) : null}
      {numbers.fileHeader === undefined ? null : (
        <FileHeaderView header={numbers.fileHeader} />
      )}
    </>
  );
}

function ExaminationLabel({ numbers }: { numbers: Numbers }) {
  switch (numbers.examination) {
    case "gaps_found":
      return (
        <StatusLabel
          status="failed"
          label="あり"
          details={
            <KeyValueList
              stacked
              pairs={[{ name: "抜けの理由", value: "未判定" }]}
            />
          }
        />
      );
    case "no_gaps":
      return <StatusLabel status="done" label="なし" />;
    case "not_examined":
      return (
        <StatusLabel
          status="idle"
          label="未確認"
          details={
            <KeyValueList
              stacked
              pairs={[
                {
                  name: "理由",
                  value:
                    numbers.notExaminedReason === undefined
                      ? "応答に理由なし"
                      : notExaminedLabels[numbers.notExaminedReason],
                },
              ]}
            />
          }
        />
      );
    default: {
      const exhaustive: never = numbers.examination;
      throw new Error(`unknown examination: ${String(exhaustive)}`);
    }
  }
}

function StreamRow({
  stream,
  onSelectRecord,
}: {
  stream: RecordNumberStream;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const namedComputerCount = stream.computers.filter(
    (computer) => computer.computer !== undefined,
  ).length;
  return (
    <tr>
      <td>
        {stream.channel === undefined ? (
          <MissingValue description="チャネルのフィールドなし" />
        ) : (
          <RawText text={stream.channel} />
        )}
      </td>
      <td>
        <ul aria-label="端末ごとの件数">
          {stream.computers.map((computer) => (
            <li key={computer.computer ?? ""}>
              {computer.computer === undefined ? (
                <MissingValue description="端末のフィールドなし" />
              ) : (
                <RawText text={computer.computer} />
              )}
              : {formatCount(computer.recordCount)}
            </li>
          ))}
        </ul>
      </td>
      <td className="text-right tabular-nums">
        {formatCount(stream.recordCount)}
      </td>
      <td className="font-mono">
        {numberSpan(stream.lowestNumber, stream.highestNumber)}
      </td>
      <td>
        {stream.gapCount === 0 ? (
          "なし"
        ) : (
          <>
            <KeyValueList
              pairs={[
                {
                  name: "抜けた番号",
                  value: formatCount(stream.missingNumberCount),
                },
                { name: "範囲", value: formatCount(stream.gapCount) },
                limitPair(
                  Math.min(stream.gaps.length, shownListLimit),
                  stream.gapCount,
                ),
              ]}
            />
            <ul aria-label="抜けた番号の範囲">
              {stream.gaps.slice(0, shownListLimit).map((gap) => (
                <GapItem
                  key={gap.firstMissingNumber}
                  gap={gap}
                  onSelectRecord={onSelectRecord}
                />
              ))}
            </ul>
          </>
        )}
      </td>
      <td>
        <span className="tabular-nums">
          {formatCount(stream.duplicatedRecordCount)}
        </span>
        {stream.duplicatedRecordCount > 0 && namedComputerCount >= 2 ? (
          <StatusLabel
            className="ml-2"
            status="pending"
            label="複数の端末の混在の可能性"
            details={
              <KeyValueList
                stacked
                pairs={[
                  { name: "Computer", value: "2 種類以上" },
                  { name: "番号", value: "重複あり" },
                ]}
              />
            }
          />
        ) : null}
      </td>
    </tr>
  );
}

function GapItem({
  gap,
  onSelectRecord,
}: {
  gap: RecordNumberGap;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  return (
    <li>
      <span className="font-mono">
        {numberSpan(gap.firstMissingNumber, gap.lastMissingNumber)}
      </span>{" "}
      <IconButton
        label="直前のレコードを表示"
        onPress={() => onSelectRecord(gap.precedingRecordRef)}
      >
        <ArrowUpToLine size={14} aria-hidden="true" />
      </IconButton>
      <IconButton
        label="直後のレコードを表示"
        onPress={() => onSelectRecord(gap.followingRecordRef)}
      >
        <ArrowDownToLine size={14} aria-hidden="true" />
      </IconButton>
      {gap.failureRecordCount > 0 ? (
        <KeyValueList
          stacked
          pairs={[
            {
              name: "取り込みの失敗",
              value: formatCount(gap.failureRecordCount),
            },
            limitPair(
              Math.min(gap.failureRecordRefs.length, shownListLimit),
              gap.failureRecordCount,
            ),
            {
              name: "失敗の位置",
              value: gap.failureRecordRefs
                .slice(0, shownListLimit)
                .map((ref) => describeRecordPosition(ref))
                // 位置の文字列は「、」を含むため、レコードの区切りに「; 」を使う。
                .join("; "),
            },
          ]}
        />
      ) : null}
    </li>
  );
}

function FileHeaderView({ header }: { header: RecordNumberFileHeader }) {
  const comparison = headerComparisons[header.comparison];
  return (
    <div role="status">
      <KeyValueList
        pairs={[
          {
            name: "ファイルの見出しの次のレコード番号",
            value: header.nextRecordNumber ?? "読み取り不可",
          },
          {
            name: "レコードの見出しの番号の最大",
            value: header.highestRecordNumber ?? "なし",
          },
          {
            name: "2 つの番号",
            value: (
              <StatusLabel
                status={comparison.status}
                label={comparison.label}
                details={comparison.reason}
              />
            ),
          },
          {
            name: "レコードの見出しの番号を読み取れなかったレコード",
            value:
              header.unreadableRecordHeaderCount > 0
                ? formatCount(header.unreadableRecordHeaderCount)
                : undefined,
          },
          {
            name: "dirty の印",
            value:
              header.dirty === "true"
                ? "あり"
                : header.dirty === "false"
                  ? "なし"
                  : undefined,
          },
        ]}
      />
    </div>
  );
}

/** 突き合わせの表の 1 行。収集元と相手の件数を並べる。 */
type ComparisonRow = {
  key: string;
  label: string;
  keys?: number;
  source?: number;
  compared?: number;
};

const countCell = (value: number | undefined) =>
  value === undefined ? "" : formatCount(value);

const comparisonColumns: DataTableColumn<ComparisonRow>[] = [
  { key: "label", header: "項目", rowHeader: true, cell: (row) => row.label },
  {
    key: "keys",
    header: "鍵",
    numeric: true,
    cell: (row) => countCell(row.keys),
  },
  {
    key: "source",
    header: "収集元",
    numeric: true,
    cell: (row) => countCell(row.source),
  },
  {
    key: "compared",
    header: "相手",
    numeric: true,
    cell: (row) => countCell(row.compared),
  },
];

function ComparisonView({
  comparison,
  source,
  compared,
  onSelectRecord,
}: {
  comparison: RecordComparison;
  source: SourceIdentity;
  compared: SourceIdentity;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const heading = (
    <>
      <h3>突き合わせ</h3>
      <KeyValueList
        pairs={[
          { name: "収集元", value: <RawText text={source.fileName} /> },
          { name: "相手", value: <RawText text={compared.fileName} /> },
        ]}
      />
    </>
  );
  if (comparison.state === "not_compared") {
    const reason = comparison.notComparedReason;
    return (
      <>
        {heading}
        <div role="status">
          <KeyValueList
            pairs={[
              {
                name: "突き合わせ",
                value: (
                  <StatusLabel
                    status="idle"
                    label="未比較"
                    details={
                      <KeyValueList
                        stacked
                        pairs={[
                          {
                            name: "理由",
                            value:
                              reason === undefined
                                ? "応答に理由なし"
                                : notComparedLabels[reason],
                          },
                          {
                            name: "次の操作",
                            value:
                              reason === undefined
                                ? undefined
                                : notComparedNextActions[reason],
                          },
                        ]}
                      />
                    }
                  />
                ),
              },
            ]}
          />
        </div>
      </>
    );
  }
  const undetermined = (total: number, outside: number, inside: number) =>
    total - outside - inside;
  const rows: ComparisonRow[] = [
    {
      key: "oneToOne",
      label: "両方に 1 件ずつ",
      keys: comparison.oneToOneKeyCount,
    },
    {
      key: "undetermined",
      label: "1 対 1 に決まらない",
      keys: comparison.undeterminedKeyCount,
      source: comparison.undeterminedSourceRecordCount,
      compared: comparison.undeterminedComparedRecordCount,
    },
    {
      key: "unequal",
      label: "件数の違う鍵の多い分",
      keys: comparison.unequalKeyCount,
      source: comparison.sourceSurplusRecordCount,
      compared: comparison.comparedSurplusRecordCount,
    },
    {
      key: "unkeyed",
      label: "鍵のフィールドの読み取り不可",
      source: comparison.sourceUnkeyedRecordCount,
      compared: comparison.comparedUnkeyedRecordCount,
    },
    {
      key: "onlyIn",
      label: "片方だけにある",
      source: comparison.onlyInSourceRecordCount,
      compared: comparison.onlyInComparedRecordCount,
    },
    {
      key: "outside",
      label: "片方だけ・相手の記録期間の外",
      source: comparison.onlyInSourceOutsideComparedRangeRecordCount,
      compared: comparison.onlyInComparedOutsideSourceRangeRecordCount,
    },
    {
      key: "inside",
      label: "片方だけ・相手の記録期間の内",
      source: comparison.onlyInSourceInsideComparedRangeRecordCount,
      compared: comparison.onlyInComparedInsideSourceRangeRecordCount,
    },
    {
      key: "rangeUndetermined",
      label: "片方だけ・記録期間の決定不可",
      source: undetermined(
        comparison.onlyInSourceRecordCount,
        comparison.onlyInSourceOutsideComparedRangeRecordCount,
        comparison.onlyInSourceInsideComparedRangeRecordCount,
      ),
      compared: undetermined(
        comparison.onlyInComparedRecordCount,
        comparison.onlyInComparedOutsideSourceRangeRecordCount,
        comparison.onlyInComparedInsideSourceRangeRecordCount,
      ),
    },
  ];
  const keyNotes =
    comparison.key === undefined ? [] : comparisonKeyNotes[comparison.key];
  return (
    <>
      {heading}
      <div className="flex items-center gap-1">
        <KeyValueList
          pairs={[
            {
              name: "比べた鍵",
              value:
                comparison.key === undefined
                  ? "応答に鍵なし"
                  : comparisonKeyLabels[comparison.key],
            },
          ]}
        />
        {keyNotes.length === 0 ? null : (
          <HelpPopover label="比べた鍵">
            <KeyValueList stacked pairs={keyNotes} />
          </HelpPopover>
        )}
      </div>
      <div className="flex items-start gap-1">
        <DataTable
          label="突き合わせの件数"
          columns={comparisonColumns}
          rows={rows}
          rowKey={(row) => row.key}
        />
        {comparison.key === undefined ? null : (
          <HelpPopover label="相手の記録期間">
            <KeyValueList stacked pairs={rangeDefinitions[comparison.key]} />
          </HelpPopover>
        )}
      </div>
      <UnequalKeysView
        unequalKeys={comparison.unequalKeys}
        total={comparison.unequalKeyCount}
        onSelectRecord={onSelectRecord}
      />
      <OnlyInList
        label="収集元だけにある鍵のレコード"
        total={comparison.onlyInSourceRecordCount}
        refs={comparison.onlyInSourceRecordRefs}
        onSelectRecord={onSelectRecord}
      />
      <OnlyInList
        label="収集元だけにあり相手の記録期間の内のレコード"
        total={comparison.onlyInSourceInsideComparedRangeRecordCount}
        refs={comparison.onlyInSourceInsideComparedRangeRecordRefs}
        onSelectRecord={onSelectRecord}
      />
      <OnlyInList
        label="相手だけにある鍵のレコード"
        total={comparison.onlyInComparedRecordCount}
        refs={comparison.onlyInComparedRecordRefs}
        onSelectRecord={onSelectRecord}
      />
      <OnlyInList
        label="相手だけにあり収集元の記録期間の内のレコード"
        total={comparison.onlyInComparedInsideSourceRangeRecordCount}
        refs={comparison.onlyInComparedInsideSourceRangeRecordRefs}
        onSelectRecord={onSelectRecord}
      />
    </>
  );
}

/** 件数の違う鍵をフィールドの値で比べた結果のラベルと、特定できなかった理由と候補。 */
const unequalKeyOutcomes: Record<
  UnequalKeyOutcome,
  { status: Status; label: string; details?: KeyValuePair[] }
> = {
  identified: { status: "done", label: "特定" },
  no_compared_values: {
    status: "idle",
    label: "比較不可",
    details: [
      { name: "理由", value: "両方の収集元で同じ文字列を持つフィールドなし" },
      { name: "候補", value: "鍵を持つすべてのレコード" },
    ],
  },
  undecided: {
    status: "pending",
    label: "未決定",
    details: [
      { name: "理由", value: "フィールドの値で 1 件に決定不可" },
      {
        name: "候補",
        value: "対にならないレコードと、鍵を持つすべてのレコード",
      },
    ],
  },
};

/**
 * 件数の違う鍵ごとに、鍵に入れていないフィールドの値まで比べた結果を出す。片方にだけあると
 * 特定したレコードと、候補のレコードを開ける。
 */
function UnequalKeysView({
  unequalKeys,
  total,
  onSelectRecord,
}: {
  unequalKeys: RecordComparisonUnequalKey[];
  total: number;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  if (unequalKeys.length === 0) return null;
  const label = "件数の違う鍵のフィールドの値による比較";
  return (
    <section aria-label={label}>
      <h4>{label}</h4>
      <KeyValueList pairs={[limitPair(unequalKeys.length, total)]} />
      <ol>
        {unequalKeys.map((key, index) => {
          const outcome = unequalKeyOutcomes[key.outcome];
          return (
            // biome-ignore lint/suspicious/noArrayIndexKey: 鍵の並びは応答が決め、同じ応答の中で変わらない。
            <li key={index}>
              <KeyValueList
                pairs={[
                  {
                    name: "収集元",
                    value: formatCount(key.sourceRecordRefs.length),
                  },
                  {
                    name: "相手",
                    value: formatCount(key.comparedRecordRefs.length),
                  },
                  {
                    name: "結果",
                    value: (
                      <StatusLabel
                        status={outcome.status}
                        label={outcome.label}
                        details={
                          outcome.details === undefined ? undefined : (
                            <KeyValueList stacked pairs={outcome.details} />
                          )
                        }
                      />
                    ),
                  },
                  {
                    name: "比べたフィールド",
                    value:
                      key.comparedSemantics.length === 0 ? undefined : (
                        <code>{key.comparedSemantics.join(", ")}</code>
                      ),
                  },
                ]}
              />
              <RecordButtons
                label="収集元だけの対にならないレコード"
                refs={key.onlyInSourceRecordRefs}
                onSelectRecord={onSelectRecord}
              />
              <RecordButtons
                label="相手だけの対にならないレコード"
                refs={key.onlyInComparedRecordRefs}
                onSelectRecord={onSelectRecord}
              />
              {key.outcome === "identified" ? null : (
                <>
                  <RecordButtons
                    label="候補: 収集元の鍵を持つレコード"
                    refs={key.sourceRecordRefs}
                    onSelectRecord={onSelectRecord}
                  />
                  <RecordButtons
                    label="候補: 相手の鍵を持つレコード"
                    refs={key.comparedRecordRefs}
                    onSelectRecord={onSelectRecord}
                  />
                </>
              )}
            </li>
          );
        })}
      </ol>
    </section>
  );
}

/** レコードの位置 1 件の値。押すと Record に表示する。 */
function RecordPositionButton({
  recordRef,
  onSelectRecord,
}: {
  recordRef: RecordLocator;
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  const position = describeRecordPosition(recordRef);
  return (
    <button
      type="button"
      className="value-link font-mono"
      aria-label={`Record に表示: ${position}`}
      onClick={() => onSelectRecord(recordRef)}
    >
      {position}
    </button>
  );
}

/** レコードの位置ごとに開く値を並べる。位置が無いときは何も出さない。 */
function RecordButtons({
  label,
  refs,
  onSelectRecord,
}: {
  label: string;
  refs: RecordLocator[];
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  if (refs.length === 0) return null;
  return (
    <>
      <p>{label}</p>
      <ul aria-label={label}>
        {refs.map((ref) => (
          <li key={recordRefKey(ref)}>
            <RecordPositionButton
              recordRef={ref}
              onSelectRecord={onSelectRecord}
            />
          </li>
        ))}
      </ul>
    </>
  );
}

function OnlyInList({
  label,
  total,
  refs,
  onSelectRecord,
}: {
  label: string;
  total: number;
  refs: RecordLocator[];
  onSelectRecord: (recordRef: RecordLocator) => void;
}) {
  if (total === 0) return null;
  return (
    <>
      <h4>{label}</h4>
      <KeyValueList
        pairs={[
          { name: "件数", value: formatCount(total) },
          limitPair(Math.min(refs.length, shownListLimit), total),
        ]}
      />
      {refs.length === 0 ? null : (
        <ul aria-label={label}>
          {refs.slice(0, shownListLimit).map((ref) => (
            <li key={recordRefKey(ref)}>
              <RecordPositionButton
                recordRef={ref}
                onSelectRecord={onSelectRecord}
              />
            </li>
          ))}
        </ul>
      )}
    </>
  );
}

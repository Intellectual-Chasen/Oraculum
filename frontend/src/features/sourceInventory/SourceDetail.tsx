import { SquareArrowOutUpRight } from "lucide-react";
import type { ReactNode } from "react";
import type {
  RecordField,
  RecordLocator,
  Timestamp,
} from "@/shared/contracts/common";
import type { SourceMember } from "@/shared/contracts/sources";
import type { TerminalAssignment } from "@/shared/contracts/terminalAssignments";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { readRecordFieldRawText } from "@/shared/lib/recordField";
import { timestampPrecisionLabels } from "@/shared/lib/recordLabels";
import {
  describeRecordPosition,
  describeRecordRange,
  recordRefKey,
} from "@/shared/lib/recordPosition";
import { formatKeyLabel } from "@/shared/lib/sourceLabels";
import { DataTable } from "@/shared/ui/DataTable";
import { LocalTimeNote } from "@/shared/ui/DisplayOffset";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { MissingValue } from "@/shared/ui/MissingValue";
import { RawText } from "@/shared/ui/RawText";
import { TimestampOffsetNote } from "@/shared/ui/TimestampOffsetNote";
import { ImportFailureTable } from "./ImportFailureTable";
import type { SourceInventoryRow } from "./inventory";
import { lineEndingLabels } from "./labels";

const fieldAbsent = "フィールドなし";

/** サーバーの起動で指定した端末の行の見出し。 */
const specifiedTerminalLabel = "サーバーの起動で指定した端末";

function DetailRow({
  label,
  help,
  children,
}: {
  label: string;
  /** 値の横の help の吹き出しに置く説明。 */
  help?: ReactNode;
  children: ReactNode;
}) {
  return (
    <tr>
      <th scope="row">{label}</th>
      <td>
        {children}
        {help === undefined ? null : (
          <HelpPopover label={label}>{help}</HelpPopover>
        )}
      </td>
    </tr>
  );
}

/** 記録期間の片側の時刻。比較に用いる正規化値を出し、タイムゾーンの出どころを次の行に添える。 */
function PeriodEnd({ timestamp }: { timestamp: Timestamp | undefined }) {
  if (timestamp === undefined) {
    return <MissingValue description="UTC 時刻なし" />;
  }
  return (
    <>
      {timestamp.normalized === undefined ? (
        <MissingValue description={fieldAbsent} />
      ) : (
        <RawText text={timestamp.normalized} />
      )}
      <TimestampOffsetNote timestamp={timestamp} />
    </>
  );
}

/** 記録期間の片側の原文と精度。help の吹き出しに入れる組を返す。 */
function periodEndFacts(name: string, timestamp: Timestamp | undefined) {
  return timestamp === undefined
    ? []
    : [
        {
          name: `${name}の原文`,
          value:
            timestamp.rawText === undefined ? (
              fieldAbsent
            ) : (
              <RawText text={timestamp.rawText} />
            ),
        },
        {
          name: `${name}の精度`,
          value: timestampPrecisionLabels[timestamp.precision],
        },
      ];
}

/** 収集元が記録した最初と最後のレコードの時刻を 1 行に出し、原文と精度を help に入れる。 */
function RecordPeriodRow({
  from,
  to,
}: {
  from: Timestamp | undefined;
  to: Timestamp | undefined;
}) {
  const facts = [
    ...periodEndFacts("始まり", from),
    ...periodEndFacts("終わり", to),
  ];
  return (
    <DetailRow
      label="記録期間"
      help={
        facts.length === 0 ? undefined : <KeyValueList stacked pairs={facts} />
      }
    >
      <KeyValueList
        stacked
        pairs={[
          { name: "始まり", value: <PeriodEnd timestamp={from} /> },
          { name: "終わり", value: <PeriodEnd timestamp={to} /> },
        ]}
      />
    </DetailRow>
  );
}

/** ファイルヘッダの値の名前ごとの行の見出し。名前の定義元は backend の読み取りである。 */
const fileHeaderLabels: Record<string, string> = {
  NextRecordID: "次のレコード番号",
  Dirty: "Dirty フラグ",
  ChunkCount: "chunk の数",
  SignedChunkCount: "ElfChnk で始まる chunk の数",
  HiveFileName: "hive のファイル名",
  PrimarySequenceNumber: "primary sequence number",
  SecondarySequenceNumber: "secondary sequence number",
  LastWrittenTime: "最終書き込みの時刻",
  FormatVersion: "hive の形式のバージョン",
  RootCellOffset: "root key の位置",
  HiveBinsDataSize: "key と値の領域の byte 数",
  BaseBlockChecksum: "ヘッダの checksum",
  Recovery: "transaction log の適用",
  AppliedSequenceRange: "適用した transaction log の番号",
  RecoveredHiveBinsDataSize: "適用後の key と値の領域の byte 数",
};

/** 行の見出しの help に置く、見出しだけでは値の求め方が分からない値の説明。 */
const fileHeaderHelps: Record<string, string> = {
  SignedChunkCount: "走査で数えた値",
  RootCellOffset: "key と値の領域の先頭から数えた byte 数",
};

/** transaction log の適用の結果の表示。 */
const recoveryLabels: Record<string, string> = {
  not_needed: "不要",
  applied: "適用済み",
  not_applied: "適用できない",
};

/** transaction log の適用の結果ごとの、値の横の help に置く説明。 */
const recoveryHelps: Record<string, string> = {
  not_needed: "主ファイルの書き込みの完了",
  not_applied: "key と値の状態: 主ファイルの書き込みの途中",
};

/** transaction log のファイルの状態のうち、1 語の値の表示。 */
const logStatusLabels: Record<string, string> = {
  absent: "主ファイルと同じ directory になし",
  empty: "0 byte",
};

/** 見出しの値の名前の行の見出し。transaction log ごとの状態はファイル名を添える。 */
function fileHeaderLabel(name: string): string {
  if (name.startsWith("Log.")) {
    return `transaction log ${name.slice("Log.".length)} の状態`;
  }
  return fileHeaderLabels[name] ?? name;
}

/** 収集元を構成するファイルを表で出す。構成のファイルを持たない収集元では何も出さない。 */
function MemberRows({ members }: { members: SourceMember[] | undefined }) {
  if (members === undefined || members.length === 0) {
    return null;
  }
  return (
    <DetailRow label="構成するファイル" help="収集元: この順に連結した byte 列">
      <DataTable
        label="構成するファイル"
        columns={[
          {
            key: "path",
            header: "ファイル",
            cell: (member) => <RawText text={member.originPath} />,
          },
          {
            key: "position",
            header: "位置",
            mono: true,
            cell: (member) =>
              `${member.byteOffset}-${member.byteOffset + member.sizeBytes}`,
          },
          {
            key: "sha256",
            header: "SHA-256",
            mono: true,
            cell: (member) => member.contentSha256,
          },
        ]}
        rows={members}
        rowKey={(member) => member.originPath}
      />
    </DetailRow>
  );
}

/** dirty の印の値の表示。 */
const dirtyLabels: Record<string, string> = {
  true: "あり",
  false: "なし",
};

/** ファイルヘッダの値 1 件の表示。 */
function FileHeaderValue({ field }: { field: RecordField }) {
  const value = readRecordFieldRawText(field);
  if ("absence" in value) {
    return <MissingValue description={value.absence} />;
  }
  if (field.name === "Dirty" && value.text in dirtyLabels) {
    return <>{dirtyLabels[value.text]}</>;
  }
  if (field.name === "Recovery" && value.text in recoveryLabels) {
    const help = recoveryHelps[value.text];
    return (
      <>
        {recoveryLabels[value.text]}
        {help === undefined ? null : (
          <HelpPopover label={recoveryLabels[value.text]}>{help}</HelpPopover>
        )}
      </>
    );
  }
  if (field.name.startsWith("Log.") && value.text in logStatusLabels) {
    return <>{logStatusLabels[value.text]}</>;
  }
  if (field.kind === "timestamp" && field.timestamp?.normalized !== undefined) {
    // 原文 (FILETIME などの経過の数) は tooltip に入れ、正規化値と表示のタイムゾーンの時刻を出す。
    return (
      <>
        <Hint text={`原文: ${toVisibleRawText(value.text)}`}>
          <RawText text={field.timestamp.normalized} />
        </Hint>
        <LocalTimeNote timestamp={field.timestamp} />
      </>
    );
  }
  return <RawText text={value.text} />;
}

/**
 * 収集元のファイルヘッダが記録した値を 1 件ずつ行に出す。ヘッダを持たない入力形式では
 * 行を出さない。
 */
function FileHeaderRows({ fields }: { fields: RecordField[] | undefined }) {
  if (fields === undefined) {
    return null;
  }
  return (
    <>
      {fields.map((field) => (
        <DetailRow
          key={field.name}
          label={fileHeaderLabel(field.name)}
          help={fileHeaderHelps[field.name]}
        >
          <FileHeaderValue field={field} />
        </DetailRow>
      ))}
    </>
  );
}

/** 選んだ収集元 1 件の識別と、取り込みの範囲を出す。 */
export function SourceDetail({
  row,
  onSelectRecord,
  terminalAssignments,
}: {
  row: SourceInventoryRow | undefined;
  /** レコードを開く。渡さないときは開く操作を出さない。 */
  onSelectRecord?: (recordRef: RecordLocator) => void;
  /** 分析者が記録した端末の割り当ての取得の状態。サーバーの起動で指定した端末を出す。 */
  terminalAssignments?: FetchState<readonly TerminalAssignment[]>;
}) {
  if (row === undefined) {
    return <KeyValueList pairs={[{ name: "収集元", value: "未選択" }]} />;
  }
  const specified: FetchState<readonly TerminalAssignment[]> | undefined =
    terminalAssignments?.status === "loaded"
      ? {
          status: "loaded",
          value: terminalAssignments.value.filter(
            (assignment) =>
              assignment.origin === "import_specified" &&
              assignment.appliesToSourceId === row.source.sourceId,
          ),
        }
      : terminalAssignments;
  return (
    <>
      <SourceIdentityTable
        row={row}
        onSelectRecord={onSelectRecord}
        specified={specified}
      />
      {/* 収集元を選び直したとき、閉じた欄の開閉とスクロールの位置を引き継がない。 */}
      <ImportFailureTable
        key={row.source.sourceId}
        failures={row.importStatus.failures}
        onSelectRecord={onSelectRecord}
      />
    </>
  );
}

/** 説明を生成できなかったレコードの件数と、位置ごとにレコードを開く操作を出す。 */
function MessageUnrenderedRows({
  count,
  recordRefs,
  onSelectRecord,
}: {
  count: number;
  recordRefs: RecordLocator[];
  onSelectRecord?: (recordRef: RecordLocator) => void;
}) {
  if (recordRefs.length === 0) {
    return <>{formatCount(count)}</>;
  }
  return (
    <>
      <KeyValueList
        pairs={[
          { name: "件数", value: formatCount(count) },
          {
            name: "表示",
            value:
              recordRefs.length < count
                ? formatCount(recordRefs.length)
                : undefined,
          },
        ]}
      />
      <ul aria-label="説明を生成できなかったレコード">
        {recordRefs.map((recordRef) => (
          <li key={recordRefKey(recordRef)}>
            {describeRecordPosition(recordRef)}{" "}
            {onSelectRecord === undefined ? null : (
              <IconButton
                label="レコードを開く"
                onPress={() => onSelectRecord(recordRef)}
              >
                <SquareArrowOutUpRight size={14} aria-hidden="true" />
              </IconButton>
            )}
          </li>
        ))}
      </ul>
    </>
  );
}

/** サーバーの起動で指定した端末 1 件の「名前: 値」の組。 */
function SpecifiedTerminal({ assignment }: { assignment: TerminalAssignment }) {
  const raw = (text: string | undefined) =>
    text === undefined ? undefined : <RawText text={text} />;
  return (
    <KeyValueList
      pairs={[
        { name: "端末 ID", value: raw(assignment.terminalId) },
        { name: "ホスト名", value: raw(assignment.terminalHostname) },
        { name: "IP", value: raw(assignment.clientIp) },
      ]}
    />
  );
}

function SourceIdentityTable({
  row,
  onSelectRecord,
  specified,
}: {
  row: SourceInventoryRow;
  onSelectRecord?: (recordRef: RecordLocator) => void;
  /** この収集元にサーバーの起動で指定した端末の割り当ての取得の状態。 */
  specified?: FetchState<readonly TerminalAssignment[]>;
}) {
  const { source, importStatus } = row;
  // backend は、原文から記録期間の両端が定まらない収集元にだけ、タイムゾーンを適用した期間を返す。
  const observedRange = row.interpretedObservedRange ?? {
    from: source.observedRangeFirst,
    to: source.observedRangeLast,
  };
  const hasSpecified =
    specified?.status === "loaded" && specified.value.length > 0;
  return (
    <table>
      <caption>
        選択中の収集元: <RawText text={source.fileName} />
      </caption>
      <tbody>
        <DetailRow label="path">
          <RawText text={source.originPath} />
        </DetailRow>
        <DetailRow label="SHA-256">{source.contentSha256}</DetailRow>
        <MemberRows members={source.members} />
        <DetailRow label="案件">
          {source.caseId ?? <MissingValue description="案件の指定なし" />}
        </DetailRow>
        <DetailRow label="入力形式">
          <RawText text={formatKeyLabel(source.formatKey)} />
        </DetailRow>
        <DetailRow label="入力形式のバージョン">
          {source.formatVersion ?? (
            <MissingValue description="原文にバージョンなし" />
          )}
        </DetailRow>
        <DetailRow label="フィールドの並びの指定">
          {source.formatSpec === undefined ? (
            <MissingValue description="指定を受け付けない入力形式" />
          ) : (
            <RawText text={source.formatSpec} />
          )}
        </DetailRow>
        <DetailRow label="byte 数">{formatCount(source.sizeBytes)}</DetailRow>
        <DetailRow label="改行の個数">
          {formatCount(source.newlineCount)}
        </DetailRow>
        <DetailRow label="末尾のレコードの後の改行">
          {source.endsWithNewline ? "あり" : "なし"}
        </DetailRow>
        <DetailRow label="行末の byte 列">
          {lineEndingLabels[source.lineEnding]}
        </DetailRow>
        <DetailRow label="レコード件数">
          {source.recordCount === undefined ? (
            <MissingValue description="未確定" />
          ) : (
            formatCount(source.recordCount)
          )}
        </DetailRow>
        <DetailRow label="取り込めなかったレコード">
          {formatCount(importStatus.failures.length)}
        </DetailRow>
        <DetailRow label="件数の範囲">
          {describeRecordRange(importStatus.scope)}
        </DetailRow>
        <DetailRow label="解析の実行 ID">
          {importStatus.analysisRunRef}
        </DetailRow>
        <RecordPeriodRow from={observedRange.from} to={observedRange.to} />
        <DetailRow
          label="説明を生成できなかったレコード"
          help={
            source.messageUnrenderedCount === undefined ||
            source.messageUnrenderedCount === 0
              ? undefined
              : "原因: 値にフィールド名なし"
          }
        >
          {source.messageUnrenderedCount === undefined ? (
            <MissingValue description="記録しない入力形式" />
          ) : (
            <MessageUnrenderedRows
              count={source.messageUnrenderedCount}
              recordRefs={row.messageUnrenderedRecordRefs ?? []}
              onSelectRecord={onSelectRecord}
            />
          )}
        </DetailRow>
        {specified?.status === "loading" && (
          <DetailRow label={specifiedTerminalLabel}>
            <MissingValue description="端末の割り当ての読み込み中" />
          </DetailRow>
        )}
        {specified?.status === "failed" && (
          <DetailRow label={specifiedTerminalLabel}>
            <MissingValue description="端末の割り当ての読み込み失敗" />
          </DetailRow>
        )}
        {hasSpecified && (
          <DetailRow label={specifiedTerminalLabel}>
            {specified.value.map((assignment) => (
              <SpecifiedTerminal
                key={`${assignment.terminalId}\u0000${assignment.terminalHostname}\u0000${assignment.clientIp}`}
                assignment={assignment}
              />
            ))}
          </DetailRow>
        )}
        <DetailRow
          // サーバーの起動で指定した端末がある収集元は、レコードをその端末に置く。レコードが記録した
          // 名前は端末の候補ではない。
          label={hasSpecified ? "レコードが記録した端末の名前" : "端末の候補"}
        >
          {source.terminalCandidates === undefined ? (
            <MissingValue description="候補なし" />
          ) : (
            <DataTable
              label="記録した端末の候補"
              columns={[
                {
                  key: "name",
                  header: "名前",
                  rowHeader: true,
                  cell: (candidate) => <RawText text={candidate.name} />,
                },
                {
                  key: "records",
                  header: "レコード",
                  numeric: true,
                  cell: (candidate) => formatCount(candidate.recordCount),
                },
              ]}
              rows={source.terminalCandidates}
              rowKey={(candidate) => toVisibleRawText(candidate.name)}
            />
          )}
        </DetailRow>
        <FileHeaderRows fields={source.fileHeader} />
      </tbody>
    </table>
  );
}

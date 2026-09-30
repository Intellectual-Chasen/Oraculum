import { Check, X } from "lucide-react";
import {
  type FormEvent,
  type KeyboardEvent,
  type MouseEvent,
  type ReactNode,
  useRef,
  useState,
} from "react";
import {
  ComboBox,
  Input,
  Label,
  ListBox,
  ListBoxItem,
  Popover,
  Text,
} from "react-aria-components";
import type { GraphTimeFilter } from "@/shared/api/graph";
import {
  type MatchConditionSelection,
  type SelectedMatchCondition,
  takesTolerance,
} from "@/shared/api/matchConditions";
import {
  type ConditionKey,
  conditionKeys,
} from "@/shared/contracts/candidates";
import {
  type FilterUnit,
  filterUnits,
  type GraphBoundPrecision,
  type NodeKind,
} from "@/shared/contracts/graph";
import { conditionKeyLabels } from "@/shared/lib/conditionLabels";
import type { FetchState } from "@/shared/lib/fetchState";
import { formatCount } from "@/shared/lib/format";
import { toVisibleRawText } from "@/shared/lib/rawText";
import { Button, type DisabledReason } from "@/shared/ui/Button";
import { cn } from "@/shared/ui/cn";
import { useDisplayOffset } from "@/shared/ui/DisplayOffset";
import { FetchFailureNotice } from "@/shared/ui/FetchFailureNotice";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { Hint } from "@/shared/ui/Hint";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { usePortalContainer } from "@/shared/ui/portalContainer";
import { RawText } from "@/shared/ui/RawText";
import { TextField } from "@/shared/ui/TextField";
import { TimestampField } from "@/shared/ui/TimestampField";
import { filterFieldLabels, timeUnitLabels } from "./labels";

/** 自由に書いた 1 つの文字列を値にする条件の種類。 */
type TextKind =
  | "contains"
  | "excludes"
  | "field"
  | "expression"
  | "countBy"
  | "addressInCidr"
  | "addressNotInCidr";
/** 欄と文字列の組を値にする条件の種類。 */
type PairKind = "fieldContains" | "fieldEquals";
/** 候補の一覧から 1 つを選ぶ条件の種類。 */
export type ChoiceKind =
  | "terminal"
  | "source"
  | "eventKind"
  | "caseId"
  | "nodeKind"
  | "depth"
  | "edgeKind";
/** 値を持たず、選んだ時点で足す条件の種類。 */
type FlagKind = "conditionsOnOriginsOnly" | "endpointRecordsInPeriod";

export type ConditionKind =
  | TextKind
  | PairKind
  | ChoiceKind
  | FlagKind
  | "eventActionRange"
  | "timeFilter"
  | "matchConditions";

/** 入力欄が確定した条件 1 つ。`kind` ごとに値の形が決まる。 */
export type ConditionValue =
  | { kind: TextKind; text: string }
  | { kind: PairKind; field: string; text: string }
  | { kind: ChoiceKind; id: string }
  | { kind: FlagKind }
  | { kind: "eventActionRange"; from?: number; to?: number }
  | { kind: "timeFilter"; filter: GraphTimeFilter | undefined }
  | { kind: "matchConditions"; selection: MatchConditionSelection };

/** 候補 1 つ。`section` を持つ候補は、同じ `section` の見出しの下に並ぶ。 */
export type Choice = {
  id: string;
  label: string;
  /** 一覧の右に出す件数。 */
  count?: number;
  section?: string;
};

/** 適用している条件 1 つの表示。 */
export type ConditionChip = {
  key: string;
  label: string;
  value: string;
  /** ノードの種類と結び付く条件だけが持つ。chip の左端にその種類の色の線を引く。 */
  nodeKind?: NodeKind;
  excluded?: boolean;
  /** 外せない理由。渡すと外す button を押せなくし、理由を出す。 */
  removeDisabledReason?: DisabledReason;
  /** 値の意味の説明。chip には値だけを出し、説明は pointer を重ねたときに出す。 */
  description?: string;
};

type KindSpec = { kind: ConditionKind; name: string; section: string };

/** 種類の一覧。区分ごとに、使う頻度の高い順に並べる。 */
const kindSpecs: readonly KindSpec[] = [
  { kind: "contains", name: "含む", section: "文字列" },
  { kind: "excludes", name: "含まない", section: "文字列" },
  { kind: "field", name: "フィールドを指定", section: "文字列" },
  {
    kind: "fieldContains",
    name: "フィールドの値が文字列を含む",
    section: "文字列",
  },
  { kind: "fieldEquals", name: "フィールドの値が等しい", section: "文字列" },
  { kind: "expression", name: "検索式", section: "文字列" },
  { kind: "terminal", name: "Host", section: "レコード" },
  { kind: "source", name: "Artifact", section: "レコード" },
  { kind: "eventKind", name: "イベントの種類", section: "レコード" },
  { kind: "eventActionRange", name: "イベント ID の範囲", section: "レコード" },
  { kind: "timeFilter", name: "期間", section: "レコード" },
  { kind: "caseId", name: "案件", section: "レコード" },
  { kind: "nodeKind", name: "ノードの種類", section: "グラフ" },
  { kind: "depth", name: "ホップ数", section: "グラフ" },
  { kind: "edgeKind", name: "エッジの種類", section: "グラフ" },
  { kind: "matchConditions", name: "推定条件", section: "グラフ" },
  { kind: "countBy", name: filterFieldLabels.countBy, section: "グラフ" },
  {
    kind: "addressInCidr",
    name: filterFieldLabels.addressInCidr,
    section: "グラフ",
  },
  {
    kind: "addressNotInCidr",
    name: filterFieldLabels.addressNotInCidr,
    section: "グラフ",
  },
  {
    kind: "conditionsOnOriginsOnly",
    name: "条件を一致ノードだけに適用",
    section: "グラフ",
  },
  {
    kind: "endpointRecordsInPeriod",
    name: "両端のレコードが期間内",
    section: "グラフ",
  },
];

const kindSections = ["文字列", "レコード", "グラフ"] as const;

const choiceKinds: ReadonlySet<ConditionKind> = new Set<ChoiceKind>([
  "terminal",
  "source",
  "eventKind",
  "caseId",
  "nodeKind",
  "depth",
  "edgeKind",
]);

function isChoiceKind(kind: ConditionKind): kind is ChoiceKind {
  return choiceKinds.has(kind);
}

/** 1 つの文字列を入れる種類の、入力欄の名前と、入力欄の下に出す値の組。 */
const textFields: Record<
  TextKind,
  { label: string; description: string; placeholder?: string; mono?: boolean }
> = {
  contains: {
    label: filterFieldLabels.valueContains,
    description: "一致: どれかのフィールドが文字列を含むレコード",
    placeholder: "203.0.113.15",
    mono: true,
  },
  excludes: {
    label: filterFieldLabels.valueExcludes,
    description: "一致: どのフィールドも含まないレコード",
    mono: true,
  },
  field: {
    label: "文字列を探すフィールド",
    description: "名前: 共通フィールド名か原資料のフィールド名",
    placeholder: "process.command_line",
    mono: true,
  },
  expression: {
    label: "検索式",
    description: "形式: フィールド 演算子 値 · and · or · not · ( )",
    placeholder: "TargetUserName contains admin and not LogonType == 3",
    mono: true,
  },
  countBy: {
    label: filterFieldLabels.countBy,
    description: "空の入力: 集計の解除",
    placeholder: "http.user_agent",
    mono: true,
  },
  addressInCidr: {
    label: filterFieldLabels.addressInCidr,
    description: "形式: CIDR",
    placeholder: "198.51.100.0/24",
    mono: true,
  },
  addressNotInCidr: {
    label: filterFieldLabels.addressNotInCidr,
    description: "形式: CIDR",
    placeholder: "198.51.100.0/24",
    mono: true,
  },
};

/** 空の文字列で足せる種類。空で足すと、その条件を外す。 */
const blankAllowed: ReadonlySet<TextKind> = new Set<TextKind>(["countBy"]);

/**
 * アドレスの範囲の文字列を判定する。
 *
 * **client 側で判定するのはアドレスの範囲だけである。** 定義元は backend の要求の読み取りで
 * ある。本判定は定義元より緩い側へだけずれる。本判定が通して backend が退ける文字列はあるが、
 * 本判定が退けて backend が通す文字列は無い。
 */
export function isAddressRange(text: string): boolean {
  const [address, length, ...rest] = text.split("/");
  if (address === undefined || length === undefined || rest.length > 0) {
    return false;
  }
  if (!/^[0-9]+$/.test(length)) {
    return false;
  }
  const bits = Number(length);
  // IPv4 は 32 bit、IPv6 は 128 bit までを取る。
  const maximumBits = address.includes(":") ? 128 : 32;
  if (bits > maximumBits) {
    return false;
  }
  return URL.canParse(
    address.includes(":") ? `http://[${address}]` : `http://${address}`,
  );
}

/** 1 つの文字列の欄の誤り。誤りが無いときは undefined を返す。 */
function textError(kind: TextKind, text: string): string | undefined {
  if (
    (kind === "addressInCidr" || kind === "addressNotInCidr") &&
    text !== "" &&
    !isAddressRange(text)
  ) {
    return "CIDR の形式の誤り";
  }
  return undefined;
}

/**
 * 期間の端の文字列を読み、精度を返す。秒の小数部が無い文字列は秒、3 桁の文字列は
 * ミリ秒の精度である。形が合わない文字列は undefined を返す。
 */
function boundPrecisionOf(text: string): GraphBoundPrecision | undefined {
  const matched =
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.(\d+))?(?:Z|[+-]\d{2}:\d{2})$/.exec(
      text,
    );
  if (matched === null) return undefined;
  const digits = matched[1]?.length ?? 0;
  if (digits === 0) return "second";
  if (digits === 3) return "millisecond";
  return undefined;
}

/** 期間の端の時刻の形式。秒の小数部はなしか 3 桁、末尾に Z か UTC オフセットを付ける。 */
const boundFormPairs = [
  { name: "形式", value: "2031-10-08T10:20:35+09:00" },
  { name: "秒の小数部", value: "なしか 3 桁" },
];

const boundFormError = "時刻の形式の誤り";

/** 10 進の数の文字列を読む。数でない文字列は undefined である。 */
function readInteger(text: string): number | undefined {
  return /^\d+$/.test(text) ? Number(text) : undefined;
}

type EditorProps = {
  kind: ConditionKind;
  name: string;
  onAdd: (value: ConditionValue) => void;
  onCancel: () => void;
};

/** 種類ごとに、値の入力を閉じる button。 */
function EditorActions({
  submitLabel = "条件に追加",
  onCancel,
}: {
  submitLabel?: string;
  onCancel: () => void;
}) {
  return (
    <div className="flex gap-1.5">
      <IconButton type="submit" variant="primary" label={submitLabel}>
        <Check size={14} aria-hidden="true" />
      </IconButton>
      <IconButton label="キャンセル" onPress={onCancel}>
        <X size={14} aria-hidden="true" />
      </IconButton>
    </div>
  );
}

/** Escape で値の入力を閉じる。form の中のどの欄からでも閉じられる。 */
function cancelOnEscape(onCancel: () => void) {
  return (event: KeyboardEvent<HTMLFormElement>) => {
    if (event.key === "Escape" && !event.defaultPrevented) {
      event.preventDefault();
      onCancel();
    }
  };
}

function TextEditor({
  kind,
  initial,
  onAdd,
  onCancel,
}: Omit<EditorProps, "kind" | "name"> & {
  kind: TextKind;
  initial: string;
}) {
  const spec = textFields[kind];
  const [text, setText] = useState(initial);
  const [shownError, setShownError] = useState<string | undefined>(undefined);
  const submit = (event: FormEvent) => {
    event.preventDefault();
    const trimmed = text.trim();
    if (trimmed === "" && !blankAllowed.has(kind)) {
      setShownError("入力なし");
      return;
    }
    const rejected = textError(kind, trimmed);
    setShownError(rejected);
    if (rejected !== undefined) return;
    onAdd({ kind, text: trimmed });
  };
  return (
    <form
      onSubmit={submit}
      onKeyDown={cancelOnEscape(onCancel)}
      className="flex flex-col gap-1.5"
    >
      <TextField
        label={spec.label}
        description={spec.description}
        placeholder={spec.placeholder}
        mono={spec.mono}
        value={text}
        onChange={(next) => {
          setText(next);
          setShownError(undefined);
        }}
        errorMessage={shownError}
        autoFocus
        spellCheck="false"
        autoComplete="off"
      />
      <EditorActions onCancel={onCancel} />
    </form>
  );
}

function PairEditor({
  kind,
  onAdd,
  onCancel,
}: Omit<EditorProps, "kind" | "name"> & { kind: PairKind }) {
  const [field, setField] = useState("");
  const [text, setText] = useState("");
  const [submitted, setSubmitted] = useState(false);
  const fieldError =
    field.trim() === ""
      ? "フィールド名なし"
      : field.includes("=")
        ? "フィールド名の中の「=」"
        : undefined;
  const textErrorMessage = text.trim() === "" ? "値なし" : undefined;
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        setSubmitted(true);
        if (fieldError !== undefined || textErrorMessage !== undefined) return;
        onAdd({ kind, field: field.trim(), text: text.trim() });
      }}
      onKeyDown={cancelOnEscape(onCancel)}
      className="flex flex-col gap-1.5"
    >
      <TextField
        label="フィールド"
        description="名前: 共通フィールド名か原資料のフィールド名"
        placeholder="TargetUserName"
        mono
        value={field}
        onChange={setField}
        errorMessage={submitted ? fieldError : undefined}
        autoFocus
        spellCheck="false"
        autoComplete="off"
      />
      <TextField
        label="値"
        description={
          kind === "fieldEquals"
            ? "一致: フィールドの値の全体が等しいレコード"
            : "一致: フィールドの値が文字列を含むレコード"
        }
        mono
        value={text}
        onChange={setText}
        errorMessage={submitted ? textErrorMessage : undefined}
        spellCheck="false"
        autoComplete="off"
      />
      <EditorActions onCancel={onCancel} />
    </form>
  );
}

function EventActionRangeEditor({
  initial,
  onAdd,
  onCancel,
}: Omit<EditorProps, "kind" | "name"> & {
  initial: { from?: number; to?: number };
}) {
  // 適用している範囲を入れて開く。片側だけを直して足したときに、もう片側を黙って消さない。
  const [fromText, setFromText] = useState(initial.from?.toString() ?? "");
  const [toText, setToText] = useState(initial.to?.toString() ?? "");
  const [submitted, setSubmitted] = useState(false);
  const from = readInteger(fromText.trim());
  const to = readInteger(toText.trim());
  const fromError =
    fromText.trim() !== "" && from === undefined
      ? "0 以上の整数ではない値"
      : undefined;
  const toError =
    toText.trim() !== "" && to === undefined
      ? "0 以上の整数ではない値"
      : undefined;
  const orderError =
    from !== undefined && to !== undefined && from > to
      ? "最大より大きい最小"
      : undefined;
  const blankError =
    submitted &&
    from === undefined &&
    to === undefined &&
    !fromError &&
    !toError
      ? "最小と最大の入力なし"
      : undefined;
  return (
    <form
      aria-label="イベント ID の範囲"
      onSubmit={(event) => {
        event.preventDefault();
        setSubmitted(true);
        if (
          fromError ||
          toError ||
          orderError ||
          (from === undefined && to === undefined)
        ) {
          return;
        }
        onAdd({ kind: "eventActionRange", from, to });
      }}
      onKeyDown={cancelOnEscape(onCancel)}
      className="flex flex-col gap-1.5"
    >
      <div className="grid grid-cols-2 gap-1.5">
        <TextField
          label="イベント ID の最小"
          mono
          inputMode="numeric"
          placeholder="4624"
          value={fromText}
          onChange={setFromText}
          errorMessage={fromError ?? orderError}
          autoFocus
        />
        <TextField
          label="イベント ID の最大"
          mono
          inputMode="numeric"
          placeholder="4634"
          value={toText}
          onChange={setToText}
          errorMessage={toError}
          isInvalid={toError !== undefined || orderError !== undefined}
        />
      </div>
      <KeyValueList
        className="text-xs text-muted"
        pairs={[
          { name: "範囲", value: "最小と最大を含む" },
          { name: "片側だけの入力", value: "可" },
        ]}
      />
      {blankError === undefined ? null : (
        <p role="alert" className="text-xs text-danger">
          {blankError}
        </p>
      )}
      <EditorActions onCancel={onCancel} />
    </form>
  );
}

function TimeFilterEditor({
  initial,
  onAdd,
  onCancel,
}: Omit<EditorProps, "kind" | "name"> & {
  initial: GraphTimeFilter | undefined;
}) {
  const [fromText, setFromText] = useState(initial?.from?.text ?? "");
  const displayOffset = useDisplayOffset();
  const [toText, setToText] = useState(initial?.to?.text ?? "");
  const [submitted, setSubmitted] = useState(false);
  // 利用者が選んだ単位。単位を変えると、端と同じ秒の中のレコードを含むかが変わる。
  const [chosenUnit, setChosenUnit] = useState<FilterUnit>();
  const from = fromText.trim();
  const to = toText.trim();
  const fromPrecision = from === "" ? undefined : boundPrecisionOf(from);
  const toPrecision = to === "" ? undefined : boundPrecisionOf(to);
  // 選んでいないときは、開いた条件の単位と両端の精度のうち、細かい方で比べる。開いた条件の
  // ミリ秒単位を黙って秒単位に戻さず、ミリ秒の端を書いたのに秒単位で比べることもしない。
  const unit: FilterUnit =
    chosenUnit ??
    (initial?.unit === "millisecond" ||
    fromPrecision === "millisecond" ||
    toPrecision === "millisecond"
      ? "millisecond"
      : "second");
  const fromError =
    submitted && from !== "" && fromPrecision === undefined
      ? boundFormError
      : undefined;
  const toError =
    submitted && to !== "" && toPrecision === undefined
      ? boundFormError
      : undefined;
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        setSubmitted(true);
        if (
          (from !== "" && fromPrecision === undefined) ||
          (to !== "" && toPrecision === undefined)
        ) {
          return;
        }
        onAdd({
          kind: "timeFilter",
          filter:
            fromPrecision === undefined && toPrecision === undefined
              ? undefined
              : {
                  from:
                    fromPrecision === undefined
                      ? undefined
                      : { text: from, precision: fromPrecision },
                  to:
                    toPrecision === undefined
                      ? undefined
                      : { text: to, precision: toPrecision },
                  unit,
                },
        });
      }}
      onKeyDown={cancelOnEscape(onCancel)}
      className="flex flex-col gap-1.5"
    >
      <TimestampField
        label="始まりの時刻"
        defaultOffset={displayOffset || "Z"}
        mono
        placeholder="2031-10-08T10:20:35+09:00"
        value={fromText}
        onChange={(next) => {
          setSubmitted(false);
          setFromText(next);
        }}
        errorMessage={fromError}
        autoFocus
      />
      <TimestampField
        label="終わりの時刻"
        defaultOffset={displayOffset || "Z"}
        mono
        placeholder="2031-10-08T11:05:48+09:00"
        value={toText}
        onChange={(next) => {
          setSubmitted(false);
          setToText(next);
        }}
        errorMessage={toError}
      />
      <label className="flex flex-col gap-1 text-sm font-medium text-ink">
        比較の単位
        <select
          className="font-normal"
          value={unit}
          onChange={(event) => {
            const next = filterUnits.find(
              (candidate) => candidate === event.target.value,
            );
            if (next !== undefined) setChosenUnit(next);
          }}
        >
          {filterUnits.map((candidate) => (
            <option key={candidate} value={candidate}>
              {timeUnitLabels[candidate]}
            </option>
          ))}
        </select>
      </label>
      <KeyValueList
        stacked
        className="text-xs text-muted"
        pairs={[
          ...boundFormPairs,
          { name: "適用先", value: "根拠のレコードと Timeline" },
          { name: "両方が空の追加", value: "期間の解除" },
        ]}
      />
      <EditorActions onCancel={onCancel} />
    </form>
  );
}

function MatchConditionsEditor({
  initial,
  onAdd,
  onCancel,
}: Omit<EditorProps, "kind" | "name"> & {
  initial: MatchConditionSelection;
}) {
  const tolerated = initial.conditions.find((condition) =>
    takesTolerance(condition.conditionKey),
  );
  const [selectedKeys, setSelectedKeys] = useState<ConditionKey[]>(() =>
    initial.conditions.map((condition) => condition.conditionKey),
  );
  const [toleranceText, setToleranceText] = useState(
    tolerated?.toleranceSeconds === undefined
      ? "0"
      : String(tolerated.toleranceSeconds),
  );
  const [error, setError] = useState<string | undefined>(undefined);
  const selectsSecondOfTime = selectedKeys.some((key) => takesTolerance(key));
  return (
    <form
      onSubmit={(event) => {
        event.preventDefault();
        // **条件を 1 つ以上選んだ適用だけを送る。** 条件を 1 つも比べない段階は、終点の側の
        // レコードをそのまま候補に並べ、根拠の無い組を候補として示すことになる。
        if (selectedKeys.length === 0) {
          setError("推定条件の選択なし");
          return;
        }
        let toleranceSeconds: number | undefined;
        if (selectsSecondOfTime) {
          const text = toleranceText.trim();
          const parsed = Number(text);
          if (!/^[0-9]+$/.test(text) || !Number.isSafeInteger(parsed)) {
            setError("0 以上の整数ではない許容幅");
            return;
          }
          toleranceSeconds = parsed;
        }
        setError(undefined);
        const conditions: SelectedMatchCondition[] = conditionKeys
          .filter((key) => selectedKeys.includes(key))
          .map((key) =>
            takesTolerance(key)
              ? { conditionKey: key, toleranceSeconds }
              : { conditionKey: key },
          );
        onAdd({ kind: "matchConditions", selection: { conditions } });
      }}
      onKeyDown={cancelOnEscape(onCancel)}
      className="flex flex-col gap-1.5"
    >
      <fieldset className="flex flex-col gap-0.5">
        <legend className="mb-1 text-sm font-medium">推定条件</legend>
        {conditionKeys.map((conditionKey, index) => (
          <label
            key={conditionKey}
            className="flex items-center gap-1.5 text-sm"
          >
            <input
              type="checkbox"
              className="accent-accent"
              // biome-ignore lint/a11y/noAutofocus: 種類を選んだ直後に値の入力へ移る。
              autoFocus={index === 0}
              checked={selectedKeys.includes(conditionKey)}
              onChange={() => {
                setError(undefined);
                setSelectedKeys((current) =>
                  current.includes(conditionKey)
                    ? current.filter((key) => key !== conditionKey)
                    : [...current, conditionKey],
                );
              }}
            />
            {conditionKeyLabels[conditionKey]}
          </label>
        ))}
      </fieldset>
      {selectsSecondOfTime ? (
        <TextField
          label="秒単位の時刻の許容幅の秒数"
          description="0: 同じ秒の候補"
          mono
          inputMode="numeric"
          value={toleranceText}
          onChange={(next) => {
            setError(undefined);
            setToleranceText(next);
          }}
        />
      ) : null}
      {error === undefined ? null : (
        <p role="alert" className="text-xs text-danger">
          {error}
        </p>
      )}
      <EditorActions submitLabel="推定条件を適用" onCancel={onCancel} />
    </form>
  );
}

const listBoxItemClass =
  "flex h-7 cursor-default items-center justify-between gap-3 px-2 text-sm text-ink outline-none data-[focused]:bg-accent-soft data-[selected]:font-medium";
const listBoxHeaderClass = "px-2 pt-2 pb-0.5 text-xs text-muted";
const popoverClass =
  "max-h-80 min-w-[var(--trigger-width)] overflow-auto rounded-md border border-line bg-surface py-1 shadow-float outline-none";

/** 候補の一覧の 1 行。名前と、右に添える文字列。 */
function ListRow({ name, aside }: { name: string; aside?: string }) {
  return (
    <ListBoxItem id={name} textValue={name} className={listBoxItemClass}>
      <span className="truncate">{name}</span>
      {aside === undefined ? null : (
        <span className="shrink-0 text-xs text-muted">{aside}</span>
      )}
    </ListBoxItem>
  );
}

/** 候補のある種類の値を、候補の一覧から選ぶ。入れた文字列で候補を絞る。 */
function ChoiceEditor({
  name,
  choices,
  description,
  onChoose,
  onCancel,
}: {
  name: string;
  choices: FetchState<readonly Choice[]>;
  description?: string;
  onChoose: (id: string) => void;
  onCancel: () => void;
}) {
  const portalContainer = usePortalContainer();
  const [text, setText] = useState("");
  if (choices.status === "failed") {
    return <FetchFailureNotice failure={choices.failure} />;
  }
  const items = choices.status === "loaded" ? choices.value : [];
  const sections: { name: string | undefined; items: Choice[] }[] = [];
  for (const item of items) {
    const last = sections.at(-1);
    if (last !== undefined && last.name === item.section) {
      last.items.push(item);
    } else {
      sections.push({ name: item.section, items: [item] });
    }
  }
  const renderItem = (item: Choice) => (
    <ListBoxItem
      key={item.id}
      id={item.id}
      textValue={toVisibleRawText(item.label)}
      className={listBoxItemClass}
    >
      <span className="truncate">
        <RawText text={item.label} />
      </span>
      {item.count === undefined ? null : (
        <span className="shrink-0 text-xs text-muted tabular-nums">
          {formatCount(item.count)}
        </span>
      )}
    </ListBoxItem>
  );
  return (
    <ComboBox
      className="flex flex-col gap-1"
      menuTrigger="focus"
      isDisabled={choices.status !== "loaded"}
      defaultFilter={() => true}
      inputValue={text}
      onInputChange={setText}
      selectedKey={null}
      onSelectionChange={(key) => {
        if (key !== null) onChoose(String(key));
      }}
      onKeyDown={(event) => {
        if (event.key === "Escape") onCancel();
      }}
    >
      <Label className="text-sm font-medium text-ink">{name}</Label>
      <Input
        autoFocus
        placeholder={
          choices.status === "loaded" ? "候補のフィルタ" : "読み込み中"
        }
        className="h-(--control-height) w-full rounded-sm border border-line bg-surface px-2 text-base outline-none placeholder:text-faint data-[focused]:border-accent data-[focused]:ring-2 data-[focused]:ring-accent/20"
      />
      <Text slot="description" className="text-xs text-muted">
        {description ?? `候補: ${formatCount(items.length)}`}
      </Text>
      <Popover
        UNSTABLE_portalContainer={portalContainer}
        className={popoverClass}
      >
        <ListBox
          className="outline-none"
          renderEmptyState={() => (
            <p className="px-2 py-1 text-sm text-muted">一致する候補なし</p>
          )}
        >
          {sections.flatMap((section) => {
            const shown = section.items.filter((item) =>
              toVisibleRawText(item.label)
                .toLowerCase()
                .includes(text.toLowerCase()),
            );
            if (shown.length === 0) return [];
            const rows = shown.map(renderItem);
            // 区分の見出しは、種類の一覧と同じく押せない行として並べる。
            return section.name === undefined
              ? rows
              : [
                  <ListBoxItem
                    key={`section:${section.name}`}
                    id={`section:${section.name}`}
                    textValue={toVisibleRawText(section.name)}
                    isDisabled
                    className={listBoxHeaderClass}
                  >
                    <RawText text={section.name} />
                  </ListBoxItem>,
                  ...rows,
                ];
          })}
        </ListBox>
      </Popover>
    </ComboBox>
  );
}

export type ConditionInputProps = {
  chips: readonly ConditionChip[];
  /** 候補のある種類の候補。渡さない種類は、種類の一覧に出さない。 */
  choices: Partial<Record<ChoiceKind, FetchState<readonly Choice[]>>>;
  /** 種類の一覧の右に添える文字列。渡さない候補のある種類は、候補の件数を添える。 */
  hints?: Partial<Record<ConditionKind, string>>;
  /** 候補のある種類の、値の入力の下に出す説明。渡さない種類は候補の件数を書く。 */
  descriptions?: Partial<Record<ChoiceKind, string | undefined>>;
  /** 種類の値を入れ始めるときの値。適用している値を直すときに使う。 */
  initial: {
    expression: string | undefined;
    eventActionRange: { from?: number; to?: number };
    timeFilter: GraphTimeFilter | undefined;
    matchConditions: MatchConditionSelection;
  };
  /** 値の入力の下に出す補足。検索式の誤りの詳しい表示など。 */
  children?: ReactNode;
  /** 入力欄の「?」の説明に足す内容。 */
  help?: ReactNode;
  onAdd: (value: ConditionValue) => void;
  onRemove: (chip: ConditionChip) => void;
  /** chip のコンテキストメニューを開く操作。 */
  chipMenu?: {
    onContextMenu: (event: MouseEvent, key: string) => void;
    onKeyDown: (event: KeyboardEvent, key: string) => void;
  };
};

/**
 * 検索の条件を chip の並びとして出し、種類を選んで値を入れる入力欄。値を持たない。
 *
 * focus すると種類の一覧を出し、種類を選ぶと値の入力に移る。値を確定すると `onAdd` を呼び、
 * 種類の欄へ focus を戻す。chip は × か、chip の button で Backspace・Delete を押して外す。
 * 種類の欄が空のまま Backspace を押すと、最後の外せる chip の × へ focus を移す。組み直しの
 * 手間が大きい条件を 1 回の key で失わないよう、外すのはもう一度押したときにする。
 */
export function ConditionInput({
  chips,
  choices,
  hints = {},
  descriptions = {},
  initial,
  children,
  help,
  onAdd,
  onRemove,
  chipMenu,
}: ConditionInputProps) {
  const portalContainer = usePortalContainer();
  const [kind, setKind] = useState<ConditionKind | undefined>(undefined);
  // 種類を選ぶたびに値の入力を作り直し、同じ種類を選び直したときも値の入力へ focus を移す。
  const [openCount, setOpenCount] = useState(0);
  // 値の要らない種類を足すたびに種類の欄を作り直し、開いた一覧を閉じる。
  const [kindBoxVersion, setKindBoxVersion] = useState(0);
  const [kindText, setKindText] = useState("");
  const kindInput = useRef<HTMLInputElement>(null);
  const chipList = useRef<HTMLUListElement>(null);
  // **画面が focus を戻すときは、種類の一覧を開かない。** 開くと一覧が結果を覆い、一覧の外を
  // 読み上げから外す。分析者が欄を離れるまで、一覧は ↓ か文字の入力で開く。
  const [returnedFocus, setReturnedFocus] = useState(false);
  const shownSpecs = kindSpecs.filter(
    (spec) => !isChoiceKind(spec.kind) || choices[spec.kind] !== undefined,
  );
  const chosen = shownSpecs.find((spec) => spec.kind === kind);

  const returnFocus = () => {
    setReturnedFocus(true);
    // 値の入力が閉じ、menuTrigger を替えた描画の後に focus を戻す。
    queueMicrotask(() => kindInput.current?.focus());
  };
  const close = () => {
    setKind(undefined);
    returnFocus();
  };
  const add = (value: ConditionValue) => {
    onAdd(value);
    close();
  };

  const hintOf = (spec: KindSpec): string | undefined => {
    const hint = hints[spec.kind];
    if (hint !== undefined) return hint;
    if (!isChoiceKind(spec.kind)) return undefined;
    const list = choices[spec.kind];
    return list?.status === "loaded"
      ? formatCount(list.value.length)
      : undefined;
  };

  const editor = (() => {
    if (chosen === undefined) return null;
    const common = { onAdd: add, onCancel: close };
    const k = chosen.kind;
    if (isChoiceKind(k)) {
      const list = choices[k];
      return list === undefined ? null : (
        <ChoiceEditor
          name={chosen.name}
          choices={list}
          description={descriptions[k]}
          onChoose={(id) => add({ kind: k, id })}
          onCancel={close}
        />
      );
    }
    switch (k) {
      case "fieldContains":
      case "fieldEquals":
        return <PairEditor kind={k} {...common} />;
      case "eventActionRange":
        return (
          <EventActionRangeEditor
            initial={initial.eventActionRange}
            {...common}
          />
        );
      case "timeFilter":
        return <TimeFilterEditor initial={initial.timeFilter} {...common} />;
      case "matchConditions":
        return (
          <MatchConditionsEditor
            initial={initial.matchConditions}
            {...common}
          />
        );
      case "conditionsOnOriginsOnly":
      case "endpointRecordsInPeriod":
        return null;
      case "expression":
        return (
          <TextEditor kind={k} initial={initial.expression ?? ""} {...common} />
        );
      default:
        return <TextEditor kind={k} initial="" {...common} />;
    }
  })();

  const lastRemovable = chips.findLast(
    (chip) => chip.removeDisabledReason === undefined,
  );

  return (
    <div className="flex flex-col gap-1">
      <div
        className={cn(
          // 1 行のときの高さを入力欄と同じにする。中の欄と chip は枠の線の内側に収める。
          "flex min-h-(--control-height) flex-wrap items-center gap-1 rounded-sm border border-line bg-surface px-1",
          "focus-within:border-accent focus-within:ring-2 focus-within:ring-accent/20",
        )}
      >
        {chips.length === 0 ? null : (
          <ul
            ref={chipList}
            aria-label="適用している検索の条件"
            className="flex max-w-full flex-wrap gap-1"
          >
            {chips.map((chip) => (
              <li
                key={chip.key}
                className={cn(
                  "inline-flex h-[calc(var(--control-height)-4px)] max-w-full items-center rounded-sm bg-ground pr-0.5 pl-1.5 text-sm",
                  chip.nodeKind !== undefined && "border-l-2",
                  chip.excluded && "bg-surface ring-1 ring-line ring-inset",
                )}
                style={
                  chip.nodeKind === undefined
                    ? undefined
                    : { borderLeftColor: `var(--kind-${chip.nodeKind})` }
                }
                onContextMenu={(event) =>
                  chipMenu?.onContextMenu(event, chip.key)
                }
                onKeyDown={(event) => chipMenu?.onKeyDown(event, chip.key)}
              >
                <span className="mr-1 whitespace-nowrap text-muted">
                  {chip.label}
                  <span className="visually-hidden">: </span>
                </span>
                {chip.description === undefined ? (
                  <span className="min-w-0 truncate font-mono">
                    <RawText text={chip.value} />
                  </span>
                ) : (
                  <Hint
                    text={chip.description}
                    className="min-w-0 truncate font-mono"
                  >
                    <RawText text={chip.value} />
                  </Hint>
                )}
                <Button
                  variant="ghost"
                  size="iconSm"
                  className="ml-0.5 size-5 min-h-0"
                  aria-label={`${chip.label} ${toVisibleRawText(chip.value)} を削除`}
                  data-chip-key={chip.key}
                  isDisabled={chip.removeDisabledReason !== undefined}
                  disabledReason={chip.removeDisabledReason}
                  onPress={() => onRemove(chip)}
                  onKeyDown={(event) => {
                    if (
                      (event.key === "Backspace" || event.key === "Delete") &&
                      chip.removeDisabledReason === undefined
                    ) {
                      event.preventDefault();
                      // 外した chip の button は消えるため、種類の欄へ focus を移す。
                      onRemove(chip);
                      returnFocus();
                      return;
                    }
                    // React Aria は keydown の伝播を止める。chip のメニューの操作へ渡す。
                    event.continuePropagation();
                  }}
                >
                  <X className="size-3" aria-hidden="true" />
                </Button>
              </li>
            ))}
          </ul>
        )}
        {/* 「?」を種類の欄と同じ行に置き、chip の並びの折り返しで欄から離さない。 */}
        <div className="flex min-w-24 flex-1 items-center">
          <ComboBox
            key={kindBoxVersion}
            aria-label="条件の種類"
            className="min-w-0 flex-1"
            menuTrigger={returnedFocus ? "input" : "focus"}
            // 区分の見出しを残して絞るため、候補は本 component が絞る。
            defaultFilter={() => true}
            selectedKey={null}
            inputValue={kindText}
            onInputChange={setKindText}
            onSelectionChange={(key) => {
              if (key === null) return;
              const spec = shownSpecs.find(
                (candidate) => candidate.name === key,
              );
              // focus のある間に欄の文字列を替えると、React Aria は一覧を開き直す。先に focus を外す。
              kindInput.current?.blur();
              setKindText("");
              if (spec === undefined) return;
              if (
                spec.kind === "conditionsOnOriginsOnly" ||
                spec.kind === "endpointRecordsInPeriod"
              ) {
                // pointer で選ぶと、React Aria は種類の欄に focus を残し、一覧を開いたままにする。
                // 種類の欄を作り直して一覧を閉じ、一覧を開かない状態で focus を戻す。
                setKindBoxVersion((version) => version + 1);
                add({ kind: spec.kind });
                return;
              }
              setKind(spec.kind);
              setOpenCount((count) => count + 1);
            }}
          >
            <Input
              ref={kindInput}
              placeholder="条件を追加"
              className="h-[calc(var(--control-height)-2px)] w-full border-0 bg-transparent px-1 text-sm shadow-none outline-none placeholder:text-faint"
              onBlur={() => setReturnedFocus(false)}
              onKeyDown={(event) => {
                if (
                  event.key === "Backspace" &&
                  kindText === "" &&
                  lastRemovable !== undefined
                ) {
                  event.preventDefault();
                  chipList.current
                    ?.querySelector<HTMLElement>(
                      `[data-chip-key="${CSS.escape(lastRemovable.key)}"]`,
                    )
                    ?.focus();
                }
              }}
            />
            <Popover
              UNSTABLE_portalContainer={portalContainer}
              className={cn(popoverClass, "w-72")}
            >
              <ListBox
                className="outline-none"
                renderEmptyState={() => (
                  <p className="px-2 py-1 text-sm text-muted">
                    一致する種類なし
                  </p>
                )}
              >
                {/* 区分の見出しは、押せない行として並べる。React Aria 1.21 の ListBoxSection は、
                  一覧を開いたまま文字を打つと 2 文字目から入力を捨てる。 */}
                {kindSections.flatMap((section) => {
                  const specs = shownSpecs.filter(
                    (spec) =>
                      spec.section === section &&
                      spec.name.toLowerCase().includes(kindText.toLowerCase()),
                  );
                  return specs.length === 0
                    ? []
                    : [
                        <ListBoxItem
                          key={section}
                          id={`section:${section}`}
                          textValue={section}
                          isDisabled
                          className={listBoxHeaderClass}
                        >
                          {section}
                        </ListBoxItem>,
                        ...specs.map((spec) => (
                          <ListRow
                            key={spec.kind}
                            name={spec.name}
                            aside={hintOf(spec)}
                          />
                        )),
                      ];
                })}
              </ListBox>
            </Popover>
          </ComboBox>
          <HelpPopover label="条件の入力">
            <KeyValueList
              stacked
              pairs={[
                { name: "追加", value: "種類の選択と値の入力" },
                {
                  name: "最後の条件の削除",
                  value: "空の入力欄で Backspace を 2 回",
                },
              ]}
            />
            {help}
          </HelpPopover>
        </div>
      </div>
      {editor === null ? null : (
        <section
          key={openCount}
          aria-label={`${chosen?.name ?? ""}の値`}
          className="mt-1 rounded-sm border border-line bg-surface p-2"
        >
          {editor}
        </section>
      )}
      {children}
    </div>
  );
}

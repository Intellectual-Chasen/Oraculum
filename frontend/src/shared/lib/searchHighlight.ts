import type { TimeRange, Timestamp } from "../contracts/common";
import type { FilterUnit } from "../contracts/graph";
import type { SearchTerms } from "./searchTerms";
import { absoluteNormalizedOf } from "./timestampInstant";

/**
 * 期間。両端は日付と UTC からのずれを持つ RFC 3339 の文字列で、比較の単位 `unit` で切り捨てて比べる。
 * 根拠のレコードを絞る条件の期間 (`shared/api/graph.ts` の `GraphTimeFilter`) をそのまま渡せる。
 */
export type GraphTimeFilter = {
  from?: { text: string };
  to?: { text: string };
  unit: FilterUnit;
};

/** 欄を指定した文字列の条件 1 つ。`whole` が真の組は値の全体が文字列と等しい欄だけに当てる。 */
export type FieldHighlightTerm = {
  field: string;
  text: string;
  whole: boolean;
};

/**
 * 各ビューで強調する検索の条件。期間と文字列の条件だけを持ち、検索式と `excludes` は持たない。
 */
export type SearchHighlight = {
  /** 欄を問わずに当てる文字列。欄を指定しない `contains` である。 */
  anyField: readonly string[];
  /** 欄を指定した文字列。欄を指定した `contains` と `fieldContains`・`fieldEquals` の組である。 */
  fieldTerms: readonly FieldHighlightTerm[];
  timeFilter?: GraphTimeFilter;
};

/** 強調する条件を持たない状態。描画ごとに別の値を作らない。 */
export const noSearchHighlight: SearchHighlight = {
  anyField: [],
  fieldTerms: [],
};

/** 文字列の条件と期間から、強調する条件を組む。 */
export function searchHighlightOf(
  terms: SearchTerms,
  timeFilter: GraphTimeFilter | undefined,
): SearchHighlight {
  const { field } = terms;
  const pairs = (held: readonly string[] | undefined, whole: boolean) =>
    (held ?? []).map((pair) => {
      // handler と同じく最初の `=` で欄と文字列を分ける。
      const at = pair.indexOf("=");
      return { field: pair.slice(0, at), text: pair.slice(at + 1), whole };
    });
  return {
    anyField: field === undefined ? terms.contains : [],
    fieldTerms: [
      ...(field === undefined
        ? []
        : terms.contains.map((text) => ({ field, text, whole: false }))),
      ...pairs(terms.fieldContains, false),
      ...pairs(terms.fieldEquals, true),
    ],
    ...(timeFilter === undefined ? {} : { timeFilter }),
  };
}

/** 強調する条件を 1 つでも持つかを返す。 */
export function highlightsAnything(highlight: SearchHighlight): boolean {
  return (
    highlight.anyField.length > 0 ||
    highlight.fieldTerms.length > 0 ||
    highlight.timeFilter?.from !== undefined ||
    highlight.timeFilter?.to !== undefined
  );
}

/** 文字列を当てる欄の名前。原資料の key と、語彙の項目である。 */
export type HighlightField = { name: string; semantic?: string };

/**
 * 欄が、文字列の条件の欄の指定 `designated` に当たるかを返す。
 * 語彙の項目か、原資料の key そのものか、key を `.` で区切った末尾の部分に一致する欄に当てる。
 */
// ponytail: backend は同じ名前の欄をグラフが持つときに末尾の部分で当てない。frontend はグラフ全体の
// 欄の名前を持たないため、その場合も末尾の部分で当てる。欄の名前の一覧を応答が返すようになったら合わせる。
export function fieldDesignated(
  field: HighlightField,
  designated: string,
): boolean {
  return (
    field.semantic === designated ||
    field.name === designated ||
    field.name.endsWith(`.${designated}`)
  );
}

const asciiOnly = /^\p{ASCII}*$/u;

/** ASCII の大文字だけを小文字にする。長さを変えない。 */
function asciiLower(text: string): string {
  return text.replace(/[A-Z]/g, (letter) => letter.toLowerCase());
}

/** 一致した範囲。`start` を含み `end` を含まない、UTF-16 の位置である。 */
export type MatchRange = { start: number; end: number };

/**
 * `text` の中で `token` に一致する範囲をすべて返す。
 * ASCII の文字列は ASCII の大文字と小文字をそろえて探す。ASCII の外の文字を含む文字列は、
 * 完全に一致した部分だけを返す。
 */
function tokenRanges(text: string, token: string): MatchRange[] {
  if (token === "") return [];
  const ascii = asciiOnly.test(token);
  const haystack = ascii ? asciiLower(text) : text;
  const needle = ascii ? asciiLower(token) : token;
  const ranges: MatchRange[] = [];
  for (
    let at = haystack.indexOf(needle);
    at >= 0;
    at = haystack.indexOf(needle, at + 1)
  ) {
    ranges.push({ start: at, end: at + needle.length });
  }
  return ranges;
}

/** 値の全体が文字列と等しいかを返す。大文字と小文字は ASCII の文字だけをそろえる。 */
function equalsWhole(text: string, token: string): boolean {
  return asciiOnly.test(token)
    ? asciiLower(text) === asciiLower(token)
    : text === token;
}

/**
 * `text` の中で強調する範囲を、始まりの順に重なりをまとめて返す。
 * `field` を渡すと、その欄を指定した条件も当てる。渡さない文字列 (ラベル) には、欄を問わない
 * 文字列だけを当てる。
 */
export function matchRangesOf(
  highlight: SearchHighlight,
  text: string,
  field?: HighlightField,
): MatchRange[] {
  const ranges = highlight.anyField.flatMap((token) =>
    tokenRanges(text, token),
  );
  if (field !== undefined) {
    for (const term of highlight.fieldTerms) {
      if (!fieldDesignated(field, term.field)) continue;
      if (!term.whole) {
        ranges.push(...tokenRanges(text, term.text));
      } else if (equalsWhole(text, term.text)) {
        ranges.push({ start: 0, end: text.length });
      }
    }
  }
  ranges.sort((left, right) => left.start - right.start);
  const merged: MatchRange[] = [];
  for (const range of ranges) {
    const last = merged.at(-1);
    if (last !== undefined && range.start <= last.end) {
      last.end = Math.max(last.end, range.end);
    } else {
      merged.push({ ...range });
    }
  }
  return merged;
}

const rfc3339Pattern =
  /^(\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2})(?:\.(\d+))?(Z|[+-]\d{2}:\d{2})$/;

/** RFC 3339 の文字列を epoch からの ms で返す。ms より下の桁は切り捨てる。読めない文字列は undefined である。 */
function epochMsOf(text: string): number | undefined {
  const match = rfc3339Pattern.exec(text);
  if (match === null) return undefined;
  const [, seconds, fraction = "", designator] = match;
  const ms = Date.parse(seconds + designator);
  return Number.isNaN(ms)
    ? undefined
    : ms + Number(fraction.slice(0, 3).padEnd(3, "0"));
}

/** epoch からの ms を、期間の比較の単位で切り捨てる。 */
function truncated(ms: number, filter: GraphTimeFilter): number {
  return filter.unit === "second" ? Math.floor(ms / 1000) * 1000 : ms;
}

/** 期間の端を、比較の単位で切り捨てた ms で返す。端が無いか読めないときは undefined である。 */
function boundMs(
  bound: GraphTimeFilter["from"],
  filter: GraphTimeFilter,
): number | undefined {
  const ms = bound === undefined ? undefined : epochMsOf(bound.text);
  return ms === undefined ? undefined : truncated(ms, filter);
}

/**
 * RFC 3339 の時点 `instant` が期間の内にあるかを返す。時点と両端を期間の比較の単位で切り捨て、
 * 両端を含めて比べる。読めない時点は期間の外である。
 */
export function instantInPeriod(
  instant: string,
  filter: GraphTimeFilter,
): boolean {
  const ms = epochMsOf(instant);
  if (ms === undefined) return false;
  const observed = truncated(ms, filter);
  const from = boundMs(filter.from, filter);
  const to = boundMs(filter.to, filter);
  return (
    (from === undefined || observed >= from) &&
    (to === undefined || observed <= to)
  );
}

/** 期間に対する時刻の判定。`unjudged` は、UTC からのずれが決まらず期間の内か外かを決められない時刻である。 */
export type PeriodJudgement = "inside" | "outside" | "unjudged";

/** 期間が端を 1 つでも持つかを返す。 */
function hasPeriod(
  filter: GraphTimeFilter | undefined,
): filter is GraphTimeFilter {
  return filter?.from !== undefined || filter?.to !== undefined;
}

/**
 * 時刻が期間の内にあるかを返す。期間が無いときと、値を持たない時刻には undefined を返す。
 * UTC からのずれが決まらない地方時と、時点を一意に決められない部分精度の時刻は `unjudged` である。
 */
export function periodJudgementOf(
  timestamp: Timestamp | undefined,
  filter: GraphTimeFilter | undefined,
): PeriodJudgement | undefined {
  if (!hasPeriod(filter) || timestamp?.valueState !== "present") {
    return undefined;
  }
  const instant = absoluteNormalizedOf(timestamp);
  if (instant !== undefined) {
    return instantInPeriod(instant, filter) ? "inside" : "outside";
  }
  return timestamp.normalizedForm === "partial_date_time" ||
    (timestamp.normalized !== undefined &&
      timestamp.normalizedForm === "local_without_offset")
    ? "unjudged"
    : undefined;
}

/**
 * エッジの適用期間が期間と重なるかを返す。期間が無いときは偽である。両端の時点が定まらない
 * 適用期間は重ならない。
 */
export function rangeOverlapsPeriod(
  range: TimeRange | undefined,
  filter: GraphTimeFilter | undefined,
): boolean {
  if (range === undefined || !hasPeriod(filter)) return false;
  const fromText = absoluteNormalizedOf(range.from);
  const toText = absoluteNormalizedOf(range.to);
  const rangeFrom = fromText === undefined ? undefined : epochMsOf(fromText);
  const rangeTo = toText === undefined ? undefined : epochMsOf(toText);
  if (rangeFrom === undefined || rangeTo === undefined) return false;
  const periodFrom = boundMs(filter.from, filter);
  const periodTo = boundMs(filter.to, filter);
  return (
    (periodTo === undefined || truncated(rangeFrom, filter) <= periodTo) &&
    (periodFrom === undefined || truncated(rangeTo, filter) >= periodFrom)
  );
}

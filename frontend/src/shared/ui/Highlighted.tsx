import { createContext, type ReactNode, useContext } from "react";
import type { Timestamp } from "../contracts/common";
import {
  type HighlightField,
  matchRangesOf,
  noSearchHighlight,
  periodJudgementOf,
  type SearchHighlight,
} from "../lib/searchHighlight";
import { RawText } from "./RawText";

/** 画面全体で強調する検索の条件。条件が無い状態は `noSearchHighlight` である。 */
export const SearchHighlightContext =
  createContext<SearchHighlight>(noSearchHighlight);

/** 画面全体で強調する検索の条件を返す。 */
export function useSearchHighlight(): SearchHighlight {
  return useContext(SearchHighlightContext);
}

/**
 * 原資料由来の文字列を描き、検索の文字列に一致した部分を `<mark>` で包む。一致の範囲は原文の
 * 文字列で求め、区切った部分ごとに `RawText` で描く。`field` を渡すと、その欄を指定した条件も当てる。
 */
export function Highlighted({
  text,
  field,
}: {
  text: string;
  field?: HighlightField;
}) {
  const ranges = matchRangesOf(useSearchHighlight(), text, field);
  if (ranges.length === 0) {
    return <RawText text={text} />;
  }
  const parts: ReactNode[] = [];
  let at = 0;
  for (const { start, end } of ranges) {
    if (start > at) {
      parts.push(<RawText key={`t${at}`} text={text.slice(at, start)} />);
    }
    parts.push(
      <mark key={`m${start}`} className="search-mark">
        <RawText text={text.slice(start, end)} />
      </mark>,
    );
    at = end;
  }
  if (at < text.length) {
    parts.push(<RawText key={`t${at}`} text={text.slice(at)} />);
  }
  return <>{parts}</>;
}

/**
 * 時刻の表示 `children` を、時刻が期間に入るときに `<mark>` で包む。期間の内か外かを決められない
 * 地方時には、決められないことを示す枠を付ける。
 */
export function PeriodMark({
  timestamp,
  children,
}: {
  timestamp: Timestamp | undefined;
  children: ReactNode;
}) {
  const { timeFilter } = useSearchHighlight();
  switch (periodJudgementOf(timestamp, timeFilter)) {
    case "inside":
      return <mark className="search-mark">{children}</mark>;
    case "unjudged": {
      const reason =
        timestamp?.normalizedForm === "partial_date_time"
          ? "時刻の精度不足"
          : "タイムゾーン不明";
      return (
        <span className="search-unjudged" title={`期間の判定: ${reason}`}>
          {children}
        </span>
      );
    }
    default:
      return <>{children}</>;
  }
}

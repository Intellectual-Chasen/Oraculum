import { ArrowDown, ArrowUp, Search } from "lucide-react";
import { type FormEvent, useEffect, useId, useState } from "react";
import type { TimelineEntry } from "@/shared/contracts/timeline";
import { formatCount } from "@/shared/lib/format";
import { describeRecordPosition } from "@/shared/lib/recordPosition";
import { DataTable } from "@/shared/ui/DataTable";
import { HelpPopover } from "@/shared/ui/HelpPopover";
import { IconButton } from "@/shared/ui/IconButton";
import { KeyValueList } from "@/shared/ui/KeyValueList";
import { RawText } from "@/shared/ui/RawText";

/** 探す向き。下方と上方は今の行から次の一致へ移り、全体は一致をすべて並べる。 */
export type FindDirection = "down" | "up" | "all";

/** 原文から探す文字列と、大文字と小文字を区別するか。 */
export type FindQuery = { text: string; caseSensitive: boolean };

/**
 * 今の行 focused から direction の向きにある最も近い一致の位置を返す。無ければ undefined。
 * 今の行が無いときは、下方は先頭から、上方は末尾から探す。
 */
export function nextMatch(
  matches: readonly number[],
  focused: number | undefined,
  direction: "down" | "up",
): number | undefined {
  if (direction === "down") {
    return matches.find((index) => index > (focused ?? -1));
  }
  return matches.findLast(
    (index) => index < (focused ?? Number.POSITIVE_INFINITY),
  );
}

/**
 * 時系列の行の原文から文字列を探す欄。文字列と、大文字と小文字の区別と、向きを選ぶ。
 * 下方と上方は次の一致へ移り、全体は一致した行を一覧にする。
 */
export function TimelineFind({
  entries,
  matches,
  focused,
  onSubmit,
  onFocus,
}: {
  entries: readonly TimelineEntry[];
  /** 今の応答が返した一致の位置。文字列を送っていないときは undefined。 */
  matches: readonly number[] | undefined;
  focused: number | undefined;
  onSubmit: (query: FindQuery) => void;
  onFocus: (index: number) => void;
}) {
  const inputId = useId();
  const [text, setText] = useState("");
  const [caseSensitive, setCaseSensitive] = useState(false);
  const [direction, setDirection] = useState<FindDirection>("down");
  const [notice, setNotice] = useState<string | undefined>(undefined);
  // 送った文字列の応答が届いたら、最初の一致へ移る。
  const [pendingJump, setPendingJump] = useState(false);

  // biome-ignore lint/correctness/useExhaustiveDependencies: 応答の一致が届いたときだけ動かす。
  useEffect(() => {
    if (!pendingJump || matches === undefined) return;
    setPendingJump(false);
    if (direction !== "all") step(direction);
  }, [matches]);

  const step = (toward: "down" | "up") => {
    if (matches === undefined) return;
    const next = nextMatch(matches, focused, toward);
    if (next === undefined) {
      setNotice(toward === "down" ? "下に一致なし" : "上に一致なし");
      return;
    }
    setNotice(undefined);
    onFocus(next);
  };

  const submit = (event: FormEvent) => {
    event.preventDefault();
    if (text === "") return;
    setNotice(undefined);
    setPendingJump(true);
    onSubmit({ text, caseSensitive });
  };

  return (
    <section aria-label="原文の検索" className="timeline-find">
      <form onSubmit={submit} className="timeline-find-form">
        <label htmlFor={inputId}>原文から探す文字列</label>
        <HelpPopover label="原文から探す文字列">
          <KeyValueList
            stacked
            pairs={[
              { name: "対象", value: "レコードの原文" },
              {
                name: "registry と Prefetch",
                value: "フィールドの名前と復号した値",
              },
            ]}
          />
        </HelpPopover>{" "}
        <input
          id={inputId}
          value={text}
          onChange={(event) => setText(event.target.value)}
        />{" "}
        <label>
          <input
            type="checkbox"
            checked={caseSensitive}
            onChange={(event) => setCaseSensitive(event.target.checked)}
          />
          大文字と小文字を区別する
        </label>{" "}
        <label>
          探す向き{" "}
          <select
            value={direction}
            onChange={(event) =>
              setDirection(event.target.value as FindDirection)
            }
          >
            <option value="down">下へ</option>
            <option value="up">上へ</option>
            <option value="all">すべて</option>
          </select>
        </label>{" "}
        <IconButton type="submit" label="検索">
          <Search size={14} aria-hidden="true" />
        </IconButton>
        {matches === undefined || direction === "all" ? null : (
          <IconButton
            label={direction === "down" ? "次の一致" : "前の一致"}
            onPress={() => step(direction)}
            isDisabled={matches.length === 0}
          >
            {direction === "down" ? (
              <ArrowDown size={14} aria-hidden="true" />
            ) : (
              <ArrowUp size={14} aria-hidden="true" />
            )}
          </IconButton>
        )}
      </form>
      {matches === undefined ? null : (
        <p role="status">
          {`一致した行: ${formatCount(matches.length)}`}
          {notice === undefined ? null : ` ${notice}`}
        </p>
      )}
      {matches === undefined || direction !== "all" ? null : (
        <DataTable
          label="文字列を含む行"
          className="timeline-find-all"
          rows={matches.flatMap((index) => {
            const entry = entries[index];
            return entry === undefined ? [] : [{ index, entry }];
          })}
          rowKey={(row) => String(row.index)}
          columns={[
            {
              key: "row",
              header: "行",
              numeric: true,
              cell: (row) => (
                <button
                  type="button"
                  aria-current={row.index === focused ? "true" : undefined}
                  aria-label={`行: ${formatCount(row.index + 1)}`}
                  onClick={() => onFocus(row.index)}
                >
                  {formatCount(row.index + 1)}
                </button>
              ),
            },
            {
              key: "source",
              header: "収集元",
              cell: (row) => (
                <RawText text={row.entry.recordRef.sourceFileName} />
              ),
            },
            {
              key: "position",
              header: "位置",
              mono: true,
              cell: (row) => describeRecordPosition(row.entry.recordRef),
            },
          ]}
        />
      )}
    </section>
  );
}

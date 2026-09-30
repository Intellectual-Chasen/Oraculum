/**
 * 表の行を CSV の文字列にし、保存させる。
 *
 * **表計算の式として読まれる値の前に `'` を付ける。** 先頭が `=` `+` `-` `@` とタブと復帰の
 * 値は、表計算のソフトが式として実行しうる。原資料の文字列は攻撃の命令を含むため、書き出す値を
 * そのまま置かない。付けた値の件数は、呼び出し側が利用者に示す。
 */

/** 表計算の式として読まれうる値の先頭の字。 */
const formulaLeaders = ["=", "+", "-", "@", "\t", "\r"];

/** 値 1 つを CSV の欄にする。式として読まれうる値は前に `'` を付ける。 */
function csvCell(value: string): { text: string; guarded: boolean } {
  const guarded = formulaLeaders.some((leader) => value.startsWith(leader));
  const body = guarded ? `'${value}` : value;
  return { text: `"${body.replaceAll('"', '""')}"`, guarded };
}

/**
 * 見出しと行を CSV の文字列にする。欄はすべて二重引用符で囲み、行は CRLF で区切り、先頭に BOM を
 * 置く。guardedCount は前に `'` を付けた値の件数である。
 */
export function toCsv(
  header: readonly string[],
  rows: readonly (readonly string[])[],
): { text: string; guardedCount: number } {
  let guardedCount = 0;
  const lines = [header, ...rows].map((row) =>
    row
      .map((value) => {
        const cell = csvCell(value);
        if (cell.guarded) guardedCount++;
        return cell.text;
      })
      .join(","),
  );
  return { text: `${byteOrderMark}${lines.join("\r\n")}\r\n`, guardedCount };
}

/** 表計算のソフトが UTF-8 と読む印 (U+FEFF)。 */
export const byteOrderMark = String.fromCharCode(0xfeff);

/** CSV の文字列を、名前 fileName の file として保存させる。 */
export function downloadCsv(fileName: string, text: string): void {
  const url = URL.createObjectURL(
    new Blob([text], { type: "text/csv;charset=utf-8" }),
  );
  const link = document.createElement("a");
  link.href = url;
  link.download = fileName;
  link.click();
  URL.revokeObjectURL(url);
}

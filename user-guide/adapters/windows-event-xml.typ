#import "../lib/theme.typ": console, ui

== Windows イベントログの XML とイベントビューアーの CSV

Windows イベントログを書き出したファイルを読みます。
書き出しの形式ごとに、形式の名前があります。

#table(
  columns: (auto, 1fr),
  [書き出しの形式], [説明],
  [XML], [`<Event>` 要素を並べたファイルです。1 つの `<Event>` 要素を 1 件のレコードとして取り込みます。],
  [CSV], [イベントビューアーが CSV で書き出したファイルです。見出しの行は、`レベル` または `キーワード` の後ろに `日付と時刻,ソース,イベント ID,タスクのカテゴリ` が続きます。1 件のイベントを 1 件のレコードとして取り込みます。],
)

=== 形式の名前

形式の名前は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。エラーの出力では、形式の名前を `formatKey` と表記します。

#table(
  columns: (auto, 1fr),
  [書き出しの形式], [形式の名前],
  [XML], [`windows_event_xml`],
  [CSV], [`windows_event_viewer_csv`],
)

=== 起動の例

パスは、調査の基準のディレクトリからの相対パスで書きます。調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。

#console("oraculum-server --investigation ./case1 windows_event_xml:export/security.xml")

#console("oraculum-server --investigation ./case1 --time-offset +09:00 windows_event_viewer_csv:export/security.csv")

=== 付けられるフラグ

#table(
  columns: (auto, auto, 1fr),
  [フラグ], [適用される範囲], [説明],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [イベントを記録した端末を指定します。収集元ごとに繰り返して書きます。],
  [`--time-offset <+hh:mm>`], [直後の収集元 1 件], [タイムゾーンが記録されていない時刻を、指定したタイムゾーンの時刻として読みます。CSV の時刻はタイムゾーンを含まないため、書き出した端末のタイムゾーンが分かるときに指定します。],
  [`--case <caseId>`], [後ろに書いたすべての収集元], [収集元を、指定した案件に含めます。],
)

`--logformat` か `--logformat-file` を付けると、取り込み全体がエラーで終了します。

=== 時刻の扱い

#table(
  columns: (auto, 1fr),
  [書き出しの形式], [時刻の扱い],
  [XML], [`TimeCreated` 要素の `SystemTime` 属性をイベントの時刻として読みます。ISO 8601 の日時で、日付と時刻の区切りは `T` と空白を受け付けます。`Z` や `+09:00` の形のタイムゾーンを含む値は、UTC 時刻が 1 つに定まります。秒未満の 7 桁の値はマイクロ秒へ切り捨て、原文の文字列はそのまま保持されます。],
  [CSV], [2 列目の `日付と時刻` の列をイベントの時刻として読みます。`2000/2/1 11:22:33` の形の、イベントビューアーが端末のタイムゾーンで表示した時刻で、精度は秒です。CSV の時刻にはタイムゾーンが記録されていません。],
)

タイムゾーンが記録されていない時刻は、`--time-offset` を指定するか、画面で時刻の解釈を記録したときに、エッジの推定に使う UTC 時刻が定まります。

=== 取り込めなかったレコードの表示

取り込めなかったレコードは、Artifacts のビューで収集元を選んだときの詳細にある#ui[取り込めなかったレコード]の表に表示されます。
表の列は#ui[レコードの範囲]、#ui[失敗した位置]、#ui[失敗した処理段階]、#ui[期待した内容]、#ui[読み取った結果]、#ui[原因の分類]、#ui[操作]です。
#ui[期待した内容]、#ui[読み取った結果]と、原因の分類とともに表示される理由は、英語の文言のまま表示されます。

XML の主な#ui[読み取った結果]は次の表のとおりです。

#table(
  columns: (auto, 1fr),
  [ファイルの状態], [#ui[読み取った結果]],
  [`<Event>` 要素が 1 つもありません], [`the file carries no <Event> start tag`],
  [`<Event>` 要素が `</Event>` で閉じていません], [`no </Event> end tag before the next <Event> start tag or the end of the file`],
  [`<Event>` 要素の外に、空白・宣言・コメント・`<Events>` のタグのいずれにも該当しないバイト列があります], [`bytes outside any <Event> element that are none of them`],
  [要素の XML の文法に誤りがあります], [`XML syntax error on line <行>: <内容>`],
)

CSV の主な#ui[読み取った結果]は次の表のとおりです。

#table(
  columns: (auto, 1fr),
  [ファイルの状態], [#ui[読み取った結果]],
  [見出しの行が期待した見出しと一致しません], [`the first line is "<見出しの行>"`],
  [フィールドの数が 6 以外です], [`the record on line <行> has <数> fields, not 6`],
  [CSV の文法に誤りがあります], [`CSV syntax error on line <行>: <内容>`],
  [レコードの始まりに続かない行があります], [`lines that follow no record start`],
)

時刻を読めなかったレコードの原因の分類は#ui[未判定]で、次の理由が表示されます。

`the value alone does not distinguish an unsupported writer, inconsistent input, and parser defects`

=== 読める範囲

- XML は UTF-8 のファイルを読みます。UTF-16 で書いた XML のファイルは、`<Event>` 要素の外のバイト列の失敗になるため、UTF-8 に変換してから指定します。失敗の#ui[読み取った結果]は `bytes outside any <Event> element that are none of them` です。
- CSV は、UTF-8 で、見出しが日本語の `レベル` または `キーワード` で始まるファイルを読みます。ほかの見出しのファイルは、見出しの行の失敗になります。

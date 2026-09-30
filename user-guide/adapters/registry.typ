#import "../lib/theme.typ": console, ui

== Windows のレジストリのハイブ

Windows のレジストリのハイブのファイルを読みます。対象は、`SYSTEM`、`SOFTWARE`、`NTUSER.DAT` などの、`regf` の形式のファイルです。
ハイブのキー 1 つを 1 件のレコードとして取り込みます。レコードには、キーのパス、最終書き込み時刻、値の名前・型・内容が含まれます。
キーと値は、原文の名前で検索して探します。

=== 形式の名前

形式の名前は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。エラーの出力では、形式の名前を `formatKey` と表記します。

`windows_registry_hive`

=== 起動の例

パスは、調査の基準のディレクトリからの相対パスで書きます。調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。

#console("oraculum-server --investigation ./case1 --terminal-name PC01 windows_registry_hive:hives/SYSTEM")

同じディレクトリに、主ファイルの名前の後ろへ `.LOG1`、`.LOG2` を付けたファイルがあるときは、Oraculum は `.LOG1` と `.LOG2` のファイルをトランザクションログとして主ファイルと一緒に読みます。上の例では、`hives/SYSTEM.LOG1` と `hives/SYSTEM.LOG2` です。起動の引数には主ファイルだけを書きます。

端末 1 台から収集したファイルを 1 つのディレクトリに保存したときは、`windows_collection:<directory>` でディレクトリの下のファイルをまとめて取り込めます。
Oraculum はディレクトリの下を再帰的に読み、ファイルの先頭のバイト列から、レジストリのハイブ、Prefetch、EVTX のファイルを判別して取り込みます。ハイブのトランザクションログは、主ファイルと一緒に読みます。
直前に付けた端末のフラグと `--case` は、ディレクトリの下のすべてのファイルに適用されます。
判別できなかったファイルと、主ファイルの無いトランザクションログは、Artifacts のビューの#ui[取り込まなかったファイル]の表に理由とともに表示されます。

#console("oraculum-server --investigation ./case1 --terminal-name PC01 windows_collection:collected/PC01")

=== 付けられるフラグ

#table(
  columns: (auto, auto, 1fr),
  [フラグ], [適用される範囲], [説明],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [ハイブを保存していた端末を指定します。収集元ごとに繰り返して書きます。],
  [`--case <caseId>`], [後ろに書いたすべての収集元], [収集元を、指定した案件に含めます。],
  [`--time-offset`], [直後の収集元 1 件], [ハイブの時刻は UTC の FILETIME であるため、`--time-offset` を付けても時刻の読み方は変わりません。],
)

`--logformat` か `--logformat-file` を付けると、取り込み全体がエラーで終了します。

=== 時刻の扱い

キーの最終書き込み時刻は FILETIME であり、UTC 時刻が 1 つに定まります。FILETIME は、1601 年からの 100 ナノ秒の数で表した UTC 時刻です。
精度はマイクロ秒へ切り捨てます。

=== 書き出しの途中のハイブとトランザクションログ

主ファイルの見出しが、書き出しの途中を表す dirty の状態を示すときは、Windows 8.1 以降の HvLE 形式のトランザクションログを適用した後のキーと値を取り込みます。
トランザクションログから読んだ部分の位置は、トランザクションログのファイルの中のバイトの位置を指します。
トランザクションログの状態は、ハイブの見出しのレコードの `Log.<file 名>` のフィールドに表示されます。見つからないトランザクションログの状態は `absent` です。

=== 取り込めなかった部分の表示

取り込めなかった部分は、Artifacts のビューで収集元を選んだときの詳細にある#ui[取り込めなかったレコード]の表に表示されます。
表の列は#ui[レコードの範囲]、#ui[失敗した位置]、#ui[失敗した処理段階]、#ui[期待した内容]、#ui[読み取った結果]、#ui[原因の分類]、#ui[操作]です。
#ui[期待した内容]、#ui[読み取った結果]と、原因の分類とともに表示される理由は、英語の文言のまま表示されます。

主な#ui[期待した内容]は次の表のとおりです。

#table(
  columns: (auto, 1fr),
  [ハイブの状態], [#ui[期待した内容]],
  [主ファイルの先頭に `regf` の見出しがありません], [`a primary file starting with a 4096-byte regf base block`],
  [主ファイルの種類が 0 以外です], [`a primary file (file type 0), found file type <番号>`],
  [dirty のハイブに適用できるトランザクションログがありません], [`a dirty hive recovered from its transaction logs; <理由>; the keys that follow are those of the primary file alone`],
)

適用できるトランザクションログが無いときの `<理由>` は、`no transaction log holds a readable entry` や `no transaction log entry could be applied` などです。Oraculum は、主ファイルだけから読んだキーと値を続けて取り込みます。

#ui[文字列の分割]の失敗の原因の分類は#ui[未判定]で、次の理由が表示されます。

`the bytes alone do not distinguish a damaged copy, an unsupported variant, and parser defects`

=== 読める範囲

- トランザクションログは、Windows 8.1 以降の HvLE 形式を適用します。
- 取り込むのは、ハイブのルートのキーから到達できるキーと値と、キーごとの最終書き込み時刻です。削除済みのセル、キーのクラス名、sk のセキュリティ記述子は取り込みの範囲の外です。

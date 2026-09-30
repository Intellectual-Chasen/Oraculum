#import "../lib/theme.typ": console, ui

== Windows イベントログの EVTX ファイル

Windows が `.evtx` の拡張子で保存するイベントログのファイルを、変換せずに読みます。
Security、System、Sysmon などのチャネルのファイルを 1 つずつ指定します。
1 件のイベントを 1 件のレコードとして取り込みます。レコードの原文は、読んだ構造から組み立てた `<Event>` 要素の XML です。

=== 形式の名前

形式の名前は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。エラーの出力では、形式の名前を `formatKey` と表記します。

`windows_evtx`

=== 起動の例

パスは、調査の基準のディレクトリからの相対パスで書きます。調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。

#console("oraculum-server --investigation ./case1 windows_evtx:logs/Security.evtx")

イベントログを記録した端末が分かっているときは、収集元の直前に端末のフラグを付けます。

#console("oraculum-server --investigation ./case1 --terminal-name PC01 --terminal-ip 192.0.2.10 windows_evtx:logs/Security.evtx")

端末 1 台から収集したファイルを 1 つのディレクトリに保存したときは、`windows_collection:<directory>` でディレクトリの下のファイルをまとめて取り込めます。
Oraculum はディレクトリの下を再帰的に読み、ファイルの先頭のバイト列から、EVTX、Prefetch、レジストリのハイブのファイルを判別して取り込みます。
直前に付けた端末のフラグと `--case` は、ディレクトリの下のすべてのファイルに適用されます。
判別できなかったファイルは、Artifacts のビューの#ui[取り込まなかったファイル]の表に理由とともに表示されます。

#console("oraculum-server --investigation ./case1 --terminal-name PC01 windows_collection:collected/PC01")

=== 付けられるフラグ

#table(
  columns: (auto, auto, 1fr),
  [フラグ], [適用される範囲], [説明],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [イベントを記録した端末を指定します。収集元ごとに繰り返して書きます。],
  [`--time-offset <+hh:mm>`], [直後の収集元 1 件], [タイムゾーンが記録されていない時刻を、指定したタイムゾーンの時刻として読みます。EVTX の時刻はタイムゾーンを含むため、`--time-offset` を付けても EVTX の時刻の読み方は変わりません。],
  [`--case <caseId>`], [後ろに書いたすべての収集元], [収集元を、指定した案件に含めます。],
)

`--logformat` か `--logformat-file` を付けると、取り込み全体がエラーで終了します。EVTX のファイルは、フィールドの並びをファイル自身で定めています。

=== 時刻の扱い

`TimeCreated` 要素の `SystemTime` 属性をイベントの時刻として読みます。
EVTX の `SystemTime` はタイムゾーンを表す `Z` を含むため、UTC 時刻が 1 つに定まります。
秒未満の 7 桁の値はマイクロ秒へ切り捨てます。原文の文字列はそのまま保持されます。

=== 取り込めなかったイベントの表示

EVTX のファイルは、先頭の見出しの後ろに、64 KiB ごとのチャンクを並べた形式です。1 つのチャンクには、複数のイベントのレコードが含まれます。

取り込めなかった部分は、Artifacts のビューで収集元を選んだときの詳細にある#ui[取り込めなかったレコード]の表に表示されます。
表の列は#ui[レコードの範囲]、#ui[失敗した位置]、#ui[失敗した処理段階]、#ui[期待した内容]、#ui[読み取った結果]、#ui[原因の分類]、#ui[操作]です。
EVTX のファイルは行の区切りを持たない形式のため、#ui[レコードの範囲]はバイトの範囲で表示されます。
#ui[期待した内容]、#ui[読み取った結果]と、原因の分類とともに表示される理由は、英語の文言のまま表示されます。

主な失敗の#ui[期待した内容]と#ui[読み取った結果]は次の表のとおりです。

#table(
  columns: (auto, 1fr, 1fr),
  [ファイルの状態], [#ui[期待した内容]], [#ui[読み取った結果]],
  [ファイルの先頭が EVTX の見出しと一致しません], [`an EVTX file header of 128 bytes starting with the file magic`], [`the file does not start with an EVTX file header`],
  [ファイルのバージョンが対応の範囲の外です], [`an EVTX file of version 3.0 to 3.2`], [`the file header names the version <バージョン>`],
  [チャンクの先頭がチャンクの識別のバイト列と一致しません], [`a chunk starting with the chunk magic`], [`the chunk starts with other bytes`],
  [チャンクの中の 1 件のレコードの見出しが壊れています], [`the record <番号> of <件数> the chunk header counts`], [`the record does not start with the record magic`、`the record header runs past the end of the chunk` など],
  [レコードからイベントを読めません], [`an event the EVTX library read from the record`], [`the library returned no event for the record <番号>`],
)

構造の失敗の原因の分類は#ui[未判定]で、次の理由が表示されます。

`the bytes alone do not distinguish a damaged copy, a file closed while it was written, and parser defects`

=== 読める範囲

- バージョン 3.0 から 3.2 の EVTX のファイルを読みます。
- 原文の XML は、読んだ構造から組み立て直したもので、`xmlns` の属性と EventData の `<Binary>` を除いた内容です。

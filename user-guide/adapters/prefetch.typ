#import "../lib/theme.typ": console, ui

== Windows の Prefetch

Windows がプログラムの実行時に書く、拡張子が `.pf` の Prefetch のファイルを読みます。
1 つのファイルを 1 件のレコードとして取り込みます。レコードには、実行ファイルの名前、実行の回数、実行の時刻、参照したファイルとボリュームの情報が含まれます。

=== 形式の名前

形式の名前は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。エラーの出力では、形式の名前を `formatKey` と表記します。

`windows_prefetch`

=== 起動の例

パスは、調査の基準のディレクトリからの相対パスで書きます。調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。

#console("oraculum-server --investigation ./case1 --terminal-name PC01 windows_prefetch:prefetch/CMD.EXE-0BD30981.pf")

`windows_prefetch` はファイルを 1 つずつ指定します。複数のファイルは、`windows_prefetch:<path>` を続けて書きます。

端末 1 台から収集したファイルを 1 つのディレクトリに保存したときは、`windows_collection:<directory>` でディレクトリの下のファイルをまとめて取り込めます。
Oraculum はディレクトリの下を再帰的に読み、ファイルの先頭のバイト列から、Prefetch、レジストリのハイブ、EVTX のファイルを判別して取り込みます。
直前に付けた端末のフラグと `--case` は、ディレクトリの下のすべてのファイルに適用されます。
判別できなかったファイルは、Artifacts のビューの#ui[取り込まなかったファイル]の表に理由とともに表示されます。

#console("oraculum-server --investigation ./case1 --terminal-name PC01 windows_collection:collected/PC01")

=== 付けられるフラグ

#table(
  columns: (auto, auto, 1fr),
  [フラグ], [適用される範囲], [説明],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [ファイルを保存していた端末を指定します。収集元ごとに繰り返して書きます。],
  [`--case <caseId>`], [後ろに書いたすべての収集元], [収集元を、指定した案件に含めます。],
  [`--time-offset`], [直後の収集元 1 件], [Prefetch の時刻は UTC の FILETIME であるため、`--time-offset` を付けても時刻の読み方は変わりません。],
)

`--logformat` か `--logformat-file` を付けると、取り込み全体がエラーで終了します。

=== 時刻の扱い

実行の時刻は FILETIME で記録されており、UTC 時刻が 1 つに定まります。FILETIME は、1601 年からの 100 ナノ秒の数で表した UTC 時刻です。
精度はマイクロ秒へ切り捨てます。最新の実行時刻をレコードの時刻にします。
実行時刻の記録領域がすべて 0 のファイルのレコードには、時刻が記録されていません。

=== 取り込めなかったファイルの表示

取り込めなかったファイルは、Artifacts のビューで収集元を選んだときの詳細にある#ui[取り込めなかったレコード]の表に表示されます。
表の列は#ui[レコードの範囲]、#ui[失敗した位置]、#ui[失敗した処理段階]、#ui[期待した内容]、#ui[読み取った結果]、#ui[原因の分類]、#ui[操作]です。
#ui[期待した内容]、#ui[読み取った結果]と、原因の分類とともに表示される理由は、英語の文言のまま表示されます。

ファイルの中身を解釈できないとき、#ui[失敗した処理段階]は#ui[文字列の分割]、#ui[期待した内容]は `a Prefetch file` です。主な#ui[読み取った結果]は次の表のとおりです。

#table(
  columns: (auto, 1fr),
  [ファイルの状態], [#ui[読み取った結果]],
  [先頭のバイト列が Prefetch の識別のバイト列と異なります], [`the file does not start with a Prefetch signature`],
  [形式の番号が対応の範囲の外です], [`version <番号>: the Prefetch format version is not supported`],
  [圧縮の方式が LZXpress Huffman 以外です], [`method 0x<16進>: the compression method is not LZXpress Huffman`],
  [圧縮したファイルの CRC32 が一致しません], [`the CRC32 of the compressed file does not match`],
  [表の位置がファイルの外を指します], [`a table of the file points outside the file` で終わる文言],
)

中身を解釈できないファイルの失敗の原因の分類は#ui[未判定]で、次の理由が表示されます。

`the bytes alone do not distinguish a damaged copy, an unsupported variant, and parser defects`

ファイルを読み込めないとき、#ui[失敗した処理段階]は#ui[読み込み]、#ui[期待した内容]は `a readable Prefetch file of at most 8 MiB` です。
8 MiB を超えるファイルの#ui[読み取った結果]は `prefetch: file exceeds the byte limit` です。読み込みの途中で止まったときは、読み込みが止まった原因の文言が表示されます。
読み込めないファイルの失敗の原因の分類は#ui[未判定]で、次の理由が表示されます。

`reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect`

=== 読める範囲

- 形式の番号 17、23、26、30、31 のファイルを読みます。`MAM` で始まる圧縮したファイルは、展開してから読みます。
- 参照したファイルと実行ファイルのパスは、`\VOLUME{…}\…` の形のまま保持します。ボリュームとドライブ文字の対応は、レジストリの `MountedDevices` の値で確かめられます。

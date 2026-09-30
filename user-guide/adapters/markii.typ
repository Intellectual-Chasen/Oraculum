#import "../lib/theme.typ": console

== InfoTrace Mark II のクライアントログ

InfoTrace Mark II Recorder が出力するクライアントログを読みます。
1 件のレコードは、固定長 29 文字のヘッダー、半角空白 1 個、`key=value` の並びからなります。
Oraculum は、レコードの種別 (`evt` と `subEvt`) の値に関わりなく、すべてのレコードを取り込みます。
プロセスの開始の記録と通信の記録は、プロセスと接続の情報としても読みます。

=== 形式の名前 (formatKey)

形式の名前 (formatKey) は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。

`infotrace_mark_ii`

対応を確かめた形式のバージョンは V3.0 と V3.2 です。

=== 起動の例

path は、調査の基準の directory (調査を作るときに server を起動した directory。`--source-root` を付けたときは指定した directory) からの相対 path で書きます。

#console("oraculum-server --investigation ./case1 --terminal-name PC01 infotrace_mark_ii:markii/client.log")

=== 付けられる flag

#table(
  columns: (auto, auto, 1fr),
  [flag], [付く範囲], [説明],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [ログを記録した端末を指定します。収集元ごとに繰り返して書きます。],
  [`--case <caseId>`], [後ろに並ぶすべての収集元], [収集元を、指定した事案に入れます。],
  [`--time-offset`], [直後の収集元 1 件], [付けられます。ヘッダーの時刻は UTC からのずれを値の中に持つため、時刻の読み方は変わりません。],
)

`--logformat` と `--logformat-file` を付けると、取り込み全体が error になり、どの収集元も取り込みません。レコードが欄の key を持つため、欄の並びは指定しません。

=== 時刻の扱い

ヘッダーの先頭 23 文字が日時 (月 2 桁 / 日 2 桁 / 年 4 桁、時分秒、ミリ秒 3 桁)、25 文字目からの 5 文字が UTC からのずれです。
ヘッダーの時刻はずれを値の中に持つため、時点が 1 つに定まります。精度はミリ秒です。
時刻は、ログを書いた端末のシステム時刻です。

=== 取り込めなかったレコードの表示

取り込めなかったレコードは、Artifacts の区画で収集元を選んだときの詳細にある「取り込めなかったレコード」の表に出ます。
表の列は「レコードの範囲」「失敗した位置」「失敗した処理段階」「期待した内容」「読み取った結果」「原因の分類」「操作」です。
「期待した内容」「読み取った結果」と、原因の分類に添える理由は、英語の文言のまま出ます。

主な失敗の「期待した内容」と「読み取った結果」は次の表のとおりです。

#table(
  columns: (auto, 1fr, 1fr),
  [レコードの状態], [期待した内容], [読み取った結果],
  [レコードの長さが 30 byte 未満です], [`a header of 29 bytes followed by one space`], [`a record of <byte 数> bytes`],
  [ヘッダーの後ろ (byte の位置 29) に半角空白以外の byte があります], [`one space at byte offset 29`], [`the byte <16進>`],
  [ヘッダーの日時とずれの間 (byte の位置 23) に半角空白以外の byte があります], [`one space at byte offset 23, between the date and time and the zone`], [`the byte <16進>`],
  [ヘッダーを日時として読めません], [`a header that is a local date and time with the offset in the value`], [`a header that cannot be read as a date and time`],
  [`sn` (レコードの連番を持つ key) の値を数として読めません], [`a non-negative decimal integer as the value of sn`], [`a value that cannot be interpreted as a non-negative decimal integer`],
  [file の末尾の 1 byte が 0x1a です],[`a Mark II record with a header and key=value fields`], [`a single DOS EOF marker byte 0x1a, not a Mark II record`],
)

file の末尾の 0x1a の失敗の原因の分類は「不整合」です。「文字列の分割」のほかの失敗の原因の分類は「未判定」で、次の理由が添えられます。

`the record does not follow the expected lexical structure. a malformed input and a defect of this tokenizer are not told apart by the structure alone`

=== 読める範囲

- 文字符号化は UTF-8 (BOM 無し)、改行は CR LF と LF を読みます。
- 確かめたバージョンは V3.0 と V3.2 です。レコードは形式のバージョンを表す key を持たないため、Oraculum はバージョンを判定しません。
- 値が `-` の欄は、原資料の文字列 `-` のまま保持します。

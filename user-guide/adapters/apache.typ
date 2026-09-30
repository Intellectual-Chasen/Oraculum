#import "../lib/theme.typ": console, ui

== Apache HTTP Server のログ

Apache HTTP Server のアクセスログとエラーログを読みます。1 行を 1 件のレコードとして取り込みます。
アクセスログとエラーログは、どちらもフィールドの並びが決まっています。

=== 形式の名前

形式の名前は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。エラーの出力では、形式の名前を `formatKey` と表記します。

#table(
  columns: (auto, 1fr),
  [形式の名前], [説明],
  [`apache_access_combined`], [アクセスログです。`combined` の並び `%h %l %u %t "%r" %>s %b "%{Referer}i" "%{User-Agent}i"` の 9 フィールドを読みます。],
  [`apache_error`], [エラーログです。`[%{u}t] [%-m:%l] [pid %P:tid %T] [client %a] %M` の並びを読みます。],
)

=== 起動の例

パスは、調査の基準のディレクトリからの相対パスで書きます。調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。

#console("oraculum-server --investigation ./case1 apache_access_combined:web/access_log")

#console("oraculum-server --investigation ./case1 --time-offset +09:00 apache_error:web/error_log")

=== 付けられるフラグ

#table(
  columns: (auto, auto, 1fr),
  [フラグ], [適用される範囲], [説明],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [ログを記録した Web サーバの端末を指定します。収集元ごとに繰り返して書きます。],
  [`--time-offset <+hh:mm>`], [直後の収集元 1 件], [タイムゾーンが記録されていない時刻を、指定したタイムゾーンの時刻として読みます。エラーログの時刻に使います。],
  [`--case <caseId>`], [後ろに書いたすべての収集元], [収集元を、指定した案件に含めます。],
)

`--logformat` か `--logformat-file` を付けると、取り込み全体がエラーで終了します。

=== 時刻の扱い

#table(
  columns: (auto, 1fr),
  [ログ], [時刻の扱い],
  [アクセスログ], [`%t` は要求を受け取った時刻です。`01/Feb/2000:11:20:10 +0900` の形で、秒の精度とタイムゾーンを含むため、UTC 時刻が 1 つに定まります。],
  [エラーログ], [`Tue Feb 01 11:22:33.123456 2000` の形で、精度はマイクロ秒です。タイムゾーンが記録されていないため、`--time-offset` を指定するか、画面で時刻の解釈を記録したときに、エッジの推定に使う UTC 時刻が定まります。],
)

時刻は、Apache が動作する端末のシステム時刻です。

=== 取り込めなかった行の表示

取り込めなかった行は、Artifacts のビューで収集元を選んだときの詳細にある#ui[取り込めなかったレコード]の表に表示されます。
表の列は#ui[レコードの範囲]、#ui[失敗した位置]、#ui[失敗した処理段階]、#ui[期待した内容]、#ui[読み取った結果]、#ui[原因の分類]、#ui[操作]です。
#ui[期待した内容]、#ui[読み取った結果]と、原因の分類とともに表示される理由は、英語の文言のまま表示されます。

#ui[失敗した処理段階]が#ui[文字列の分割]のとき、#ui[読み取った結果]は `the byte 0x<16進> at byte offset <位置>` または `the end of the record at byte offset <位置>` の形で、並びと一致しなかった位置を示します。

アクセスログの主な#ui[期待した内容]は次の表のとおりです。

#table(
  columns: (auto, 1fr),
  [行の状態], [#ui[期待した内容]],
  [フィールドの間の空白の位置に、別のバイトがあります], [`a space before the next item`],
  [9 フィールドの後ろに余りがあります], [`the end of the record after 9 items`],
  [時刻が形式と一致しません], [`a bracketed local timestamp with second precision and a numeric offset`],
  [要求行が `method target HTTP/version` の形と一致しません], [`method, request target, and HTTP/version separated by spaces`],
)

エラーログの主な#ui[期待した内容]は次の表のとおりです。

#table(
  columns: (auto, 1fr),
  [行の状態], [#ui[期待した内容]],
  [角括弧が閉じていません], [`a closing ] for the module and severity item` など],
  [`[client …]` のフィールドがありません], [`the client item`。`[client …]` が無い行は、`the client item` の失敗になります。],
  [時刻が形式と一致しません], [`a timestamp with microsecond precision and no UTC offset`],
  [本文がありません], [`the message item`],
)

1 行の長さの上限は 1 MiB です。上限を超えた行は#ui[読み込み]の失敗になり、#ui[期待した内容]は `a complete record within the byte limit`、#ui[読み取った結果]は `apache: record exceeds the byte limit` です。Oraculum は、上限を超えた行の位置で読み込みを終えます。

原因の分類は#ui[未判定]です。#ui[文字列の分割]の失敗には次の理由が表示されます。

`the lexical structure alone does not distinguish unsupported format, inconsistent input, and tokenizer defects`

=== 読める範囲

- アクセスログは、`combined` の `LogFormat` で書いたファイルを読みます。
- エラーログは、形式の名前の表に示した `ErrorLogFormat` の並びで書いたファイルを読みます。
- バイト列は、文字コードを判定せずにそのまま保持します。改行は LF と CR LF を読みます。

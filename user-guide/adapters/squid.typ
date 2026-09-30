#import "../lib/theme.typ": console, ui

== Squid のアクセスログ

Proxy の Squid が書く `access.log` を読みます。1 行を 1 件のレコードとして取り込みます。
フィールドの並びは、Squid の設定の `logformat` が決めます。Oraculum は、起動の引数で選んだ並びでファイルを読みます。

=== 形式の名前

形式の名前は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。エラーの出力では、形式の名前を `formatKey` と表記します。ログのフィールドの並びに合わせて選びます。

#table(
  columns: (auto, 1fr),
  [形式の名前], [説明],
  [`squid_combined`], [Squid の組み込みの `combined` の並びです。ident の位置を `%[ui` として読みます。\ `%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %<st "%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh`],
  [`squid_combined_request_bytes`], [`squid_combined` の状態コードの後ろに、要求のバイト数の `%>st` を追加した並びです。\ `%>a %[ui %[un [%tl] "%rm %ru HTTP/%rv" %>Hs %>st %<st "%{Referer}>h" "%{User-Agent}>h" %Ss:%Sh`],
  [`squid_logformat`], [`--logformat` か `--logformat-file` で並びを渡します。`squid_combined` と `squid_combined_request_bytes` のどちらにも該当しない並びのログに使います。],
)

=== 起動の例

パスは、調査の基準のディレクトリからの相対パスで書きます。調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。

#console("oraculum-server --investigation ./case1 squid_combined:proxy/access.log")

`squid.conf` の `logformat` の行をファイルに切り出して渡す例:

#console("oraculum-server --investigation ./case1 --logformat-file proxy/logformat.txt squid_logformat:proxy/access.log")

=== 付けられるフラグ

#table(
  columns: (auto, auto, 1fr),
  [フラグ], [適用される範囲], [説明],
  [`--logformat <spec>`], [後ろに書いたすべての収集元], [フィールドの並びの文字列を渡します。`squid_logformat` には必須です。`logformat <名前> ` で始まる `squid.conf` の 1 行を渡すと、並びの部分を読みます。組み込みの名前 `squid`、`common`、`combined`、`referrer`、`useragent`、`icap_squid` も渡せます。],
  [`--logformat-file <path>`], [後ろに書いたすべての収集元], [並びを書いたファイルを渡します。並びの文字列は `%` と引用符を含むため、シェルの引用で文字列が変わるときは `--logformat-file` を使います。行末の改行は取り除きます。],
  [`--case <caseId>`], [後ろに書いたすべての収集元], [収集元を、指定した案件に含めます。],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [ログを記録した Proxy のサーバの端末を指定します。収集元ごとに繰り返して書きます。],
  [`--time-offset <+hh:mm>`], [直後の収集元 1 件], [タイムゾーンが記録されていない時刻を、指定したタイムゾーンの時刻として読みます。],
)

収集元ごとに別の並びを渡すときは、`--logformat` または `--logformat-file` を収集元の直前に付けて繰り返します。

`squid_combined` と `squid_combined_request_bytes` に `--logformat` か `--logformat-file` を付けたとき、または `squid_logformat` に並びを渡さなかったときは、取り込み全体がエラーで終了します。

=== 時刻の扱い

時刻は、Proxy が要求を処理し終えた時点の、ログを書いた端末のシステム時刻です。並びの時刻のコードで扱いが決まります。

#table(
  columns: (auto, 1fr),
  [時刻のコード], [扱い],
  [角括弧で囲んだ既定の形の `%tl`], [秒の精度で、数値のタイムゾーンを値の中に含みます。],
  [UNIX epoch 秒の `%ts` と、`%ts.%03tu` の形], [UTC 時刻が 1 つに定まります。],
  [`%tg`], [Squid が UTC で書く定義に従い、UTC 時刻として読みます。],
  [strftime の `%z` を含まない書式の `%tl`], [タイムゾーンが記録されていません。`--time-offset` を指定するか、画面で時刻の解釈を記録したときに、エッジの推定に使う UTC 時刻が定まります。],
)

`%tu` の桁数は、`%.6tu` のように `.` の後ろに書いた最大幅です。最大幅は 1 から 6 です。最大幅を書かない `%03tu` などは 3 桁です。
精度は、6 桁のときマイクロ秒、3〜5 桁のときミリ秒、1〜2 桁のとき秒です。

=== 取り込めなかった行の表示

取り込めなかった行は、Artifacts のビューで収集元を選んだときの詳細にある#ui[取り込めなかったレコード]の表に表示されます。
表の列は#ui[レコードの範囲]、#ui[失敗した位置]、#ui[失敗した処理段階]、#ui[期待した内容]、#ui[読み取った結果]、#ui[原因の分類]、#ui[操作]です。
#ui[期待した内容]、#ui[読み取った結果]と、原因の分類とともに表示される理由は、英語の文言のまま表示されます。

#ui[失敗した処理段階]が#ui[文字列の分割]のとき、#ui[読み取った結果]は `the byte 0x<16進> at byte offset <位置>` または `the end of the record at byte offset <位置>` の形で、並びと一致しなかった位置を示します。

主な#ui[期待した内容]は次の表のとおりです。

#table(
  columns: (auto, 1fr),
  [行の状態], [#ui[期待した内容]],
  [フィールドを区切る文字列がありません], [`the literal "<文字列>" before the <欄> item`],
  [フィールドの値がありません], [`a value for the <欄> item`],
  [引用符が閉じていません], [`a closing quote`],
  [行が並びより長くなっています], [`the end of the record after the last item`],
  [`%tl` の時刻を読めません], [`a bracketed local timestamp with second precision and a numeric offset`],
)

1 行の長さの上限は 1 MiB です。上限を超えた行は#ui[読み込み]の失敗になり、#ui[期待した内容]は `a complete record within the byte limit`、#ui[読み取った結果]は `squid: record exceeds the byte limit` です。Oraculum は、上限を超えた行の位置で読み込みを終えます。

原因の分類は#ui[未判定]です。#ui[文字列の分割]の失敗には次の理由が表示されます。

`the lexical structure alone does not distinguish unsupported format, inconsistent input, and tokenizer defects`

=== 読める範囲

- 並びは、Squid が定義するフォーマットコードだけで書きます。Squid が定義していないコードを含む並びは、ログを読む前にエラーになります。
- 隣り合う 2 つのコードの間には、区切りの文字列が必要です。区切りの文字列を挟まない並びは、ログを読む前にエラーになります。
- 区切りの文字列を値の中に含むフィールドがある行は、行が並びより長くなり、#ui[文字列の分割]の失敗になります。区切りの文字列を値の中に含むフィールドには、エンコーディングを指定しない `%ssl::>cert_subject` などがあります。
- バイト列は、文字コードを判定せずにそのまま保持します。改行は LF と CR LF を読みます。

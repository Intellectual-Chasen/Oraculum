#import "../lib/theme.typ": console, ui

== Linux の auditd の監査ログ

Linux の auditd が書く監査ログの `audit.log` を読みます。
auditd は 1 つのイベントを複数の行に分けて書き、同じイベントの行は `msg=audit(<秒>.<ミリ秒>:<連番>)` の値が一致します。Oraculum は、`msg=audit(...)` の値を、同じイベントの行を集める鍵として使います。
同じイベントの行の間に、別のイベントの行が書かれることがあります。
Oraculum は、イベントの先頭の行から 256 行の範囲にある同じ鍵の行を集め、1 件のレコードとして取り込みます。

=== 形式の名前

形式の名前は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。エラーの出力では、形式の名前を `formatKey` と表記します。

`linux_auditd`

=== 起動の例

パスは、調査の基準のディレクトリからの相対パスで書きます。調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。

#console("oraculum-server --investigation ./case1 --terminal-name web01 linux_auditd:server/audit.log")

=== 付けられるフラグ

#table(
  columns: (auto, auto, 1fr),
  [フラグ], [適用される範囲], [説明],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [ログを記録した端末を指定します。収集元ごとに繰り返して書きます。],
  [`--case <caseId>`], [後ろに書いたすべての収集元], [収集元を、指定した案件に含めます。],
  [`--time-offset`], [直後の収集元 1 件], [監査ログの時刻は UNIX epoch 秒であるため、`--time-offset` を付けても監査ログの時刻の読み方は変わりません。],
)

`--logformat` か `--logformat-file` を付けると、取り込み全体がエラーで終了します。

=== 時刻の扱い

`msg=audit(...)` の秒とミリ秒をレコードの時刻にします。UNIX epoch 秒であるため、UTC 時刻が 1 つに定まります。精度はミリ秒です。
画面に表示する正規化値は UTC で書きます。
時刻は、ログを書いた Linux の端末のシステム時刻です。

=== 取り込めなかった行の表示

取り込めなかった行は、Artifacts のビューで収集元を選んだときの詳細にある#ui[取り込めなかったレコード]の表に表示されます。
表の列は#ui[レコードの範囲]、#ui[失敗した位置]、#ui[失敗した処理段階]、#ui[期待した内容]、#ui[読み取った結果]、#ui[原因の分類]、#ui[操作]です。
#ui[期待した内容]、#ui[読み取った結果]と、原因の分類とともに表示される理由は、英語の文言のまま表示されます。

主な失敗の#ui[期待した内容]と#ui[読み取った結果]は次の表のとおりです。

#table(
  columns: (auto, 1fr, 1fr),
  [行の状態], [#ui[期待した内容]], [#ui[読み取った結果]],
  [`key=value` を 1 つも含まない行], [`an auditd line holding at least one key=value pair`], [`a line carrying no key=value pair`],
  [`msg=audit(...)` を含まない行], [`an auditd line holding msg=audit(<seconds>.<milliseconds>:<serial>)`], [`a line carrying no msg key outside the interpreted part`],
  [空のキー], [`a key of one byte or more before '='`], [`an empty key at byte offset <位置>`],
)

#ui[文字列の分割]の失敗の原因の分類は#ui[未判定]で、次の理由が表示されます。

`the line does not follow the expected lexical structure. a malformed input and a defect of this tokenizer are not told apart by the structure alone`

収集元の先頭がイベントの途中から始まるとき、原因の分類は#ui[不整合]になり、#ui[読み取った結果]は次の形で表示されます。

`the first event <鍵> carries <行数> lines of "<種別>" and no SYSCALL line, so the source begins inside an event`

=== 読める範囲

- 同じイベントの行を集める範囲は、イベントの先頭の行から 256 行までです。257 行目より後ろに現れた同じ鍵の行は、別のレコードになります。
- 1 行の長さの上限は 1 MiB です。
- フィールドの意味は、`SYSCALL`、`EXECVE`、`PATH`、`PROCTITLE`、`CWD` の行で定まります。ほかの種類の行も `key=value` の並びとして読み、原文のキーをフィールドの名前にします。
- `key=` の値は原文の文字列として保持します。値と監査規則の `audit.rules` の対応は、分析者が確かめます。
- 0x1D の後ろの値は、原文の文字列として保持します。

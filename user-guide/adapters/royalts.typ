#import "../lib/theme.typ": console, ui

== Royal TS の接続の文書

リモート接続の管理ツール Royal TS/TSX の、拡張子が `.rtsz` の文書から、リモートデスクトップの接続項目を読みます。
文書の中の `<RoyalRDSConnection>` 要素 1 つを 1 件のレコードとして取り込みます。
レコードには、Royal TS の内部の識別子の `id`、表示名の `name`、接続先の `uri`、接続に使う利用者の名前の `credentialUsername` が含まれます。
接続先は、IP アドレスとホスト名を区別して扱います。

=== 形式の名前

形式の名前は、起動の引数で収集元を `<形式の名前>:<path>` の形で書くときの前半です。エラーの出力では、形式の名前を `formatKey` と表記します。

`royalts_rds_connection`

=== 起動の例

パスは、調査の基準のディレクトリからの相対パスで書きます。調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。

#console("oraculum-server --investigation ./case1 --terminal-name PC01 royalts_rds_connection:documents/connections.rtsz")

=== 付けられるフラグ

#table(
  columns: (auto, auto, 1fr),
  [フラグ], [適用される範囲], [説明],
  [`--terminal-id`、`--terminal-name`、`--terminal-ip`], [直後の収集元 1 件], [文書を保存していた端末を指定します。収集元ごとに繰り返して書きます。],
  [`--case <caseId>`], [後ろに書いたすべての収集元], [収集元を、指定した案件に含めます。],
)

`--logformat` か `--logformat-file` を付けると、取り込み全体がエラーで終了します。

=== 時刻の扱い

取り込む接続項目には、時刻のフィールドがありません。

=== 取り込めなかった文書の表示

取り込めなかった部分は、Artifacts のビューで収集元を選んだときの詳細にある#ui[取り込めなかったレコード]の表に表示されます。
表の列は#ui[レコードの範囲]、#ui[失敗した位置]、#ui[失敗した処理段階]、#ui[期待した内容]、#ui[読み取った結果]、#ui[原因の分類]、#ui[操作]です。
#ui[期待した内容]、#ui[読み取った結果]と、原因の分類とともに表示される理由は、英語の文言のまま表示されます。

XML として読めないとき、#ui[失敗した処理段階]は#ui[文字列の分割]で、#ui[読み取った結果]には XML の読み取りのエラーの文言が表示されます。主な#ui[期待した内容]は次の表のとおりです。

#table(
  columns: (auto, 1fr),
  [文書の状態], [#ui[期待した内容]],
  [XML の文法に誤りがあります], [`well-formed XML`],
  [接続項目の要素が壊れています], [`a well-formed RoyalRDSConnection element`],
)

Oraculum は、XML の失敗の位置で接続項目の取り込みを終えます。XML の失敗の原因の分類は#ui[未判定]で、次の理由が表示されます。

`the lexical structure alone does not distinguish unsupported format, inconsistent input, and parser defects`

文書を読み込めないとき、#ui[失敗した処理段階]は#ui[読み込み]、#ui[期待した内容]は `a readable Royal TS document within the byte limit` です。
文書の上限は 8 MiB です。8 MiB を超える文書の#ui[読み取った結果]は `royalts: document exceeds the byte limit` です。読み込みの途中で止まったときは、読み込みが止まった原因の文言が表示されます。
読み込めない文書の失敗の原因の分類は#ui[未判定]で、次の理由が表示されます。

`reading stopped; the I/O failure alone does not establish input inconsistency or a parser defect`

=== 読める範囲

- 取り込む接続の種類は、リモートデスクトップの `<RoyalRDSConnection>` です。
- `<CredentialPassword>` の値は、暗号文も含めて読まず、保持も出力もしません。原文では `<CredentialPassword>[redacted]</CredentialPassword>` に置き換えて表示します。

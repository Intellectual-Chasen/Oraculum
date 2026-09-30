#import "../lib/theme.typ": console, note, ui

= 準備と起動

== サーバと画面を用意する

次のバージョンの Go、Node.js、pnpm が必要です。

#table(
  columns: (auto, 1fr),
  [ツール], [バージョン],
  [Go], [1.26],
  [Node.js], [24.15 以上 25 未満],
  [pnpm], [12.2.1],
)

Oraculum のリポジトリのディレクトリで、サーバをビルドし、画面が依存するパッケージをインストールします。

#console("go -C backend build -o ../oraculum-server ./cmd/oraculum-server\npnpm -C frontend install --frozen-lockfile")

リポジトリのディレクトリに `oraculum-server` が作られます。
本書の例は、`oraculum-server` を `PATH` に含まれるディレクトリに配置したものとして書いています。
`PATH` に含まれるディレクトリに配置しないときは、`oraculum-server` を絶対パスで実行します。

サーバは起動のときに ATT&CK のルールを読み込みます。
ルールのディレクトリは `--attack-rules` で指定します。指定しないときは、`oraculum-server` のファイルのディレクトリから見た `../share/oraculum/attack-rules` を読み込みます。
ルールのディレクトリが無いと、サーバは `load ATT&CK rules:` で始まるエラーを出力して終了します。
上のビルドの手順で作った `oraculum-server` を起動するときは、`--attack-rules` にリポジトリの `backend/rules/attack` のディレクトリを指定します。
本章のコマンドの例は、`--attack-rules` を省略した形です。

== 調査を作ってサーバを起動する

ログのファイルがあるディレクトリに移り、調査のディレクトリと収集元を指定してサーバを起動します。
収集元は `形式の名前:path` の形で続けて書きます。
パスは、調査の基準のディレクトリからの相対パスで書きます。
調査の基準のディレクトリは、調査を作るときにサーバを起動したディレクトリです。`--source-root` を付けたときは、指定したディレクトリです。
収集元に付けられるフラグは、`oraculum-server --help` が出力する `usage:` の行で確かめます。

#table(
  columns: (auto, 1fr),
  [形式の名前], [ログ],
  [`windows_evtx`], [Windows イベントログの EVTX ファイル],
  [`windows_event_xml`], [Windows イベントログを XML に書き出したファイル],
  [`windows_event_viewer_csv`], [イベントビューアーで CSV に書き出したファイル],
  [`windows_prefetch`], [Windows の Prefetch ファイル],
  [`windows_registry_hive`], [Windows のレジストリのハイブ],
  [`squid_combined`], [Squid のアクセスログの combined 形式],
  [`squid_combined_request_bytes`], [Squid のアクセスログの combined 形式に、要求のバイト数を追加した形式],
  [`squid_logformat`], [Squid のアクセスログで、`--logformat` か `--logformat-file` でフィールドの並びを指定する形式],
  [`apache_access_combined`], [Apache HTTP Server のアクセスログの combined 形式],
  [`apache_error`], [Apache HTTP Server のエラーログ],
  [`linux_auditd`], [Linux の auditd の監査ログ],
  [`royalts_rds_connection`], [Royal TS の接続の文書],
  [`infotrace_mark_ii`], [InfoTrace Mark II のクライアントログ],
)

#console(
  "oraculum-server --investigation ./case1 windows_evtx:Hands-on-2/Security.evtx windows_evtx:Hands-on-2/System.evtx",
  output: "investigation ./case1: 0 recorded and 2 added sources under /data/advance\nSecurity.evtx published_full read=11640 succeeded=11640 failed=0\nSystem.evtx published_full read=1177 succeeded=1177 failed=0\nbuilding the graph for the default selection\nlistening on 127.0.0.1:8080",
)

コンソールには、収集元ごとに次の値が出力されます。

#table(
  columns: (auto, 1fr),
  [値], [意味],
  [`published_full`], [公開の状態です。収集元の全体を結果として公開しています。],
  [`published_partial`], [公開の状態です。取り込めなかった部分があり、読めたレコードだけを結果として公開しています。],
  [`withheld`], [公開の状態です。収集元の結果の公開を保留しています。],
  [`read`], [読み込んだレコードの件数です。],
  [`succeeded`], [取り込めたレコードの件数です。],
  [`failed`], [取り込めなかったレコードのうち、位置を確定できた件数です。],
)

同じファイル名の収集元が 2 件以上あるときは、ファイル名の後ろの括弧に、区別するためのディレクトリの名前が付きます。
公開の状態が `published_partial` または `withheld` のときは、「困ったとき」の章を参照します。

`listening on` の行が出力されると、画面から接続できます。
サーバは、画面からの接続を受け付けながら、グラフの組み立てを続けます。
組み立ての間、画面には#ui[調査の段階]が表示されます。#ui[処理]の#ui[状態]が#ui[完了]になると、検索とグラフの画面に切り替わります。

== 画面で収集元を選んで始める

収集元を付けずに、ログのファイルを置いたディレクトリを `--source-root` で指定してサーバを起動すると、収集元を画面で選べます。

#console("oraculum-server --investigation ./case1 --source-root /mnt/evidence/advance")

画面の#ui[収集元の選択]には、`--source-root` のディレクトリの中のフォルダとファイルが表示されます。
フォルダの名前を押すと、そのフォルダの中に移ります。
#ui[判定した形式]には、サーバがファイルの先頭を読んで判定した形式が表示されます。
形式を判定できないファイルには、理由が表示されます。

+ ファイルのチェックボックスを押すか、一覧から#ui[読み込む収集元]へドラッグして追加します。フォルダの行のボタンか、#ui[フォルダ内をすべて追加]を押すと、フォルダの下のファイルのうち、形式を判定できたファイルをまとめて選べます。ファイル名で一覧を絞り込んでも、選択した内容は保持されます。
+ #ui[読み込む収集元]で、ファイルごとの#ui[入力形式]を確かめます。形式が 1 つに決まったファイルは、その形式が選ばれています。候補が複数のファイルと、形式を判定できなかったファイルは、#ui[入力形式]を選ぶまで読み込みません。
+ Squid のカスタム形式では、#ui[ログ書式を指定]を押します。CLI の `--logformat` と同じ書式文字列・組み込み名・`squid.conf` の `logformat` 行を入力するか、#ui[書式ファイルを選択]で `--logformat-file` に使うファイルを読み込み、内容を確認して#ui[適用]を押します。#ui[選択中のSquid資料に同じ書式を適用]にチェックを付けると、現在 Squid 形式を選んでいる資料を同じカスタム形式と書式へ更新します。ほかの形式と形式が未選択の資料は、入力した指定を保持します。書式を指定するまで、その資料は読み込みません。
+ 必要なときは、#ui[記録した端末の表示名]と#ui[案件]を入力します。#ui[詳しい指定]では、記録した端末の識別子と IP を指定できます。
+ 読み込むファイルの件数を表示した#ui[… 件の読み込みを開始]を押します。#ui[読み込みの完了後に処理を始める]にチェックが付いていると、読み込みの後に処理が続けて始まり、処理が完了すると検索とグラフの画面に切り替わります。

画面から収集元を選べるのは、編集者の役割を持つ利用者です。

PC 上のファイルとフォルダは、#ui[読み込む収集元]へドロップするか、#ui[PCからファイルを選択]と#ui[PCからフォルダを選択]で追加できます。
アップロード先は、`--source-root` の下の `oraculum-uploads` です。サーバには、このディレクトリへ書き込む権限が必要です。
既存の資料を上書きせず、フォルダ内の資料と付属ファイルを同じ保存先に保持します。
アップロード中は進行した件数を表示します。中止と失敗の後も、追加済みの資料と入力内容は保持されます。
選択をやり直すときは、各資料の#ui[収集元を外す]か、#ui[すべて外す]を使います。選択を外しても、アップロードした資料はサーバに残ります。
狭い画面では一覧と取り込み対象を縦に配置し、資料ごとの入力欄をまとめて表示します。

== 画面を開く

別のコンソールで、リポジトリのディレクトリから画面を起動し、出力に表示された URL をブラウザで開きます。
画面は `127.0.0.1:8080` のサーバに接続します。

#console("pnpm -C frontend dev", output: "  ➜  Local:   http://localhost:5173/")

サーバを `--addr` で別の待ち受け先にしたときは、`ORACULUM_API_TARGET` に接続先を指定して画面を起動します。

#console("ORACULUM_API_TARGET=http://127.0.0.1:18080 pnpm -C frontend dev")

画面をビルドして、サーバから配信することもできます。
リポジトリのディレクトリで画面をビルドし、ビルドの出力先のディレクトリを `--frontend-dir` で指定してサーバを起動します。
サーバは API と同じ待ち受け先で画面を配信し、`http://127.0.0.1:8080/` で画面を開けます。
画面は、サーバと同じ commit のリポジトリでビルドします。

#console("pnpm -C frontend build\noraculum-server --investigation ./case1 --frontend-dir /path/to/oraculum/frontend/dist")

== 調査を開き直す

2 回目からは、収集元を付けずに、同じ調査のディレクトリを指定して起動します。
サーバは記録した収集元を再読み込みし、前回の調査と分析者の記録を開きます。

#console("oraculum-server --investigation ./case1")

新しいファイルを追加するときは、追加するファイルの収集元だけを付けて起動します。
記録した収集元をもう一度付けると、サーバは `the investigation already records ...` を出力して終了します。

収集元のファイルは、調査を作ったときと同じ場所に保存したままにします。
収集元のファイルを別のディレクトリへ移したときは、移した先を `--source-root` で指定して起動します。

#console("oraculum-server --investigation ./case1 --source-root /mnt/evidence/advance")

#note[調査のディレクトリを指定しない起動][`--investigation` を付けずに起動すると、ワークスペースと分析者の記録はメモリ上に保存され、サーバを止めると消えます。]

== 複数の分析者で使う

複数の分析者が同じサーバを使うときは、アカウントのファイルを作り、`--accounts` で指定して起動します。
アカウントは `accounts add` で追加します。パスワードは標準入力の 1 行目から読みます。パスワードは 8 文字以上にします。

#console("printf '%s\\n' 'analyst1-password' | oraculum-server accounts add --accounts ./accounts.db analyst1 --display-name '分析者 1'", output: "add analyst1")

最初の起動では、`--admin` で調査の管理者にするアカウントを指定します。
管理者は Members のビューで、ほかのアカウントに役割を与えます。

#console("oraculum-server --investigation ./case1 --accounts ./accounts.db --admin analyst1")

ほかの端末のブラウザから接続するときは、`--addr` にループバック以外の待ち受け先を指定し、`--allowed-host` にブラウザが開く `host:port` を指定します。
ほかの端末のブラウザから接続するときは、`--accounts` と `--investigation` も指定します。
サーバは、`--allowed-host` の値とループバックの名前の `host:port` に宛てた要求だけに応答します。

== Sigma のルールを適用する

Sigma のルールのファイルを保存したディレクトリを `--sigma-rules` で指定すると、取り込んだレコードにルールを適用した結果が、Detection のビューの#ui[Sigma ルールの候補]に表示されます。
ディレクトリが git の作業ツリーの中にあるときは、HEAD の commit がルールの集合のバージョンとして記録されます。
ディレクトリが git の作業ツリーの外にあるときは、`--sigma-rules-revision` で、ルールの集合の取得元の commit を指定できます。

#console("oraculum-server --investigation ./case1 --sigma-rules ~/sigma/rules/windows")

== サーバのフラグ

`oraculum-server` の起動のフラグは次のとおりです。

#table(
  columns: (auto, 1fr),
  [フラグ], [意味],
  [`--addr <host:port>`], [待ち受け先です。既定値は `127.0.0.1:8080` です。],
  [`--allowed-host <host:port>`], [ほかの端末のブラウザが開く `host:port` です。繰り返して指定できます。`--addr` がループバック以外のときに必要です。],
  [`--accounts <file>`], [アカウントのファイルです。],
  [`--admin <login>`], [調査の管理者にするアカウントのログイン名です。`--accounts` と一緒に指定します。],
  [`--investigation <dir>`], [調査のディレクトリです。ディレクトリに調査が無ければ作り、あれば開きます。],
  [`--source-root <dir>`], [収集元の相対パスを解決する基準のディレクトリです。],
  [`--frontend-dir <dir>`], [画面のビルドの出力先のディレクトリです。指定すると、サーバが画面を配信します。],
  [`--logon-session-limit <duration>`], [ログオンのセッションが続く最長の時間です。`24h`、`90m` の形で書きます。既定値は `24h` です。終わりを記録したレコードが無いセッションは、最後の操作のレコードの時刻から `--logon-session-limit` の時間が過ぎた時点で終わったものとして扱います。],
  [`--attack-rules <dir>`], [ATT&CK のルールのファイルを保存したディレクトリです。既定値は、`oraculum-server` のファイルのディレクトリから見た `../share/oraculum/attack-rules` です。],
  [`--sigma-rules <dir>`], [Sigma のルールのファイルを保存したディレクトリです。],
  [`--sigma-rules-revision <commit>`], [ルールの集合の取得元の commit です。`--sigma-rules` と一緒に指定します。],
  [収集元に付けるフラグ], [`--case`、`--logformat`、`--logformat-file`、`--terminal-id`、`--terminal-name`、`--terminal-ip`、`--time-offset` です。各フラグの引数の形は、`oraculum-server --help` が出力する `usage:` の行で確かめます。],
  [`--import-spec <path>`], [`oraculum-import` が出力した JSON の `importSpec` の値を保存したファイルです。記録と同じ収集元を、同じ入力形式で取り込みます。収集元と、収集元に付けるフラグは、`--import-spec` のファイルから読みます。],
)

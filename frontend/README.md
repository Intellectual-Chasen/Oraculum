# frontend

Node.js 24.15.0 以上の 24 系と pnpm 12.2.1 を使用します。
リポジトリの root で実行します。

```bash
pnpm -C frontend install --frozen-lockfile
pnpm -C frontend dev
```

端末に表示された URL をブラウザーで開くと、収集元の一覧とグラフの探索と元レコードの表示を
表示します。終了は `Ctrl+C` です。

収集元の一覧は `GET /api/v0/sources` の応答から、収集元の file 名、内容の識別 (sha256)、
レコード件数、取り込みの件数 (読み込み・成功・未対応・未判定・不整合)、公開の状態を
1 行ずつ出します。file 名のボタンを押すと、選んだ収集元の取得元・入力形式・件数の対象範囲と、
最初と最後のレコードの時刻を原資料の文字列と正規化値に分けて下の表に出します。

グラフの探索は `GET /api/v0/graph` の応答から、部分グラフの図と、ノードの一覧と、
エッジの一覧を出します。図はノードを種別ごとの色の丸で、エッジを両端を結ぶ線で描きます。
見出しは選んでいるノードと、そのノードに繋がるノードに出します。図のノードを押すと、
そのノードを選びます。

**読めたレコードは 1 件ずつノードとしてグラフに載ります。** レコードのノードは収集元の
file 名と位置を表示名に持ち、そのレコードが指す対象へ「レコードが指す対象」の関係で
繋がります。読めた欄はすべてレコードのノードの属性になり、語彙に写していない欄は原資料の
key の文字列で並びます。レコードの件数は対象の件数より多いため、種別を選ばない一覧は
レコードのノードが多くを占めます。対象だけを読むときはノードの種別を選びます。

起点から広げるホップ数を選べます。0 ホップは絞り込みに合うノードだけを、1 ホップは隣り合う
ノードとエッジまでを、2 ホップ以上はエッジをその本数まで辿って届くノードとエッジまでを出します。
プロセスの祖先の連鎖は、関係の種別に親子のプロセスを選び、起点のプロセスからホップ数を上げて
辿ります。起動のレコードを持たない祖先は、ノードの詳細が「起動のレコードが無い」を出します。
ノードの種別と関係の種別を選ぶと、条件を要求に載せて取り直します。図の上の件数の文は、
絞り込みに合うノードの総数と、図に出した件数と、打ち切りの有無を書きます。

欄の値に含まれる文字列で絞るときは、「検索する文字列」へ文字列を入力して「文字列を適用」を押します。
一致は大文字と小文字を区別します。一致したノードには、一致した欄の名前と、原資料の文字列と
正規化値のどちらで一致したかが、ノードの一覧の「検索が一致した欄」に出ます。読めた欄はすべて
レコードのノードの属性になっているため、対象の識別鍵にしか現れない値も、その値を持つ
レコードが一致します。文字列を持つ欄が読めた欄の中に 1 つも無いときと、文字列はあるが他の絞り込みで残らなかった
ときは、別の文を出します。判定に入るのは読めたレコードの読めた欄だけです。文字列を空にして適用すると文字列の条件を外します。

欄の値ごとに数えるときは、「数える欄」へ語彙の項目 (`http.user_agent`) または語彙に写して
いない欄の原資料の key を入力して「数える欄を適用」を押します。表は値ごとに、その値を観測
したレコードの件数と、最初と最後に観測した時刻と、根拠のレコードを出します。
**数える範囲は絞り込みに合うノードの全件であり、図と一覧に出したノードの中だけではありません。**
値を読めなかった欄は数に入りません。1 レコードが同じ欄へ 2 つの異なる値を持つこともあるため、
件数の和と、絞り込みに合うレコードの件数を比べられません。表の上の文が、数えた集合と、
この表に出していない値が残っているかを書きます。表の根拠のボタンを押すと元レコードの原文を
出します。数える欄に値を持つ欄が読めた欄の中に 1 つも無いときと、絞り込みで残らなかった
ときは、別の文を出します。欄を空にして適用すると数える条件を外します。

応答が用いた検索の文字列と数える欄とアドレスの範囲は、件数の文の下に出ます。0 件の応答でも
出るため、どの条件の結果かを読めます。アドレスの範囲の文字列が先頭のアドレスと prefix の
長さを `/` で繋いだ形でないときは、どの欄を直すかを画面が示します。

IP アドレスのノードをアドレスの範囲で絞るときは、「範囲の中に残すアドレス」または
「範囲の外に残すアドレス」へ `198.51.100.0/24` の形の文字列を入力して「アドレスの範囲を適用」を
押します。両方を入力すると、両方を満たすアドレスだけが残ります。範囲で絞るのは
IP アドレスのノードだけであり、プロセスを起点に接続先を辿るときも起点のプロセスは残ります。
両方を空にして適用すると範囲の条件を外します。

根拠のレコードを期間で絞るときは、下端と上端の時刻の文字列、各端の精度（秒またはミリ秒）、
比較の単位を入力して「期間を適用」を押します。片方の端だけでも指定できます。両端を空にして
適用すると期間の条件を外します。応答が用いた期間の条件は件数の文の下に出ます。

ノードの一覧は表示名、種別、識別鍵の形、識別鍵の値、応答がそのノードを含む理由を
1 行ずつ出します。表示名が別の欄から導いた値のときは、導いた値であることと導き方を
添えます。「詳細を開く」を押すと `GET /api/v0/nodes/{id}` の応答から、そのノードの
識別鍵、属性、ノードを記録したレコード、繋がるエッジの種別と向きごとの件数を出します。
「起点にする」を押すと、そのノードを起点にして近傍を取り直します。

エッジの一覧は関係の種別、関係の状態、起点と終点の表示名、根拠のレコードの総数、
根拠の時刻の範囲を 1 行ずつ出します。

ノードの詳細の根拠の行のボタンを押すと、選んだレコード位置を画面の見出しの下に出し、
元レコードの表示へ渡します。

元レコードの表示は `GET /api/v0/records` の応答から、収集元の file 名と内容の識別 (sha256) と
レコード件数、レコード位置、観測の種別、レコード全体の原文、項目の一覧を出します。項目の
一覧は応答の `fields` の要素数だけ行を並べ、1 行に項目の名前、原資料の文字列、比較に用いる
正規化値、導き方、時刻の精度を出します。応答が到達した経路を含むときは、起点のレコードと、
各段階の入力・用いた識別子・出力を段階の順に出します。

原資料の文字列が持つ制御文字と書式文字は `U+XXXX` の可視の符号で出します。原資料の文字列を
HTML として解釈せず、文字列のまま出します。

接続先が応答を返さない間、画面は取得失敗の表示と次に行える操作を出します。

取得失敗の表示は、失敗した操作と、応答が返した `code` が表す状態と、次に行える操作を
別の行に出します。`code` が `not_implemented` の失敗には、実装の途中であることと、実装が
入った後に同じ操作をもう一度実行できることを出します。

図を描けない環境では、図の位置に描けなかったことを出し、ノードの一覧とエッジの一覧と
元レコードの表示を使い続けられます。

## backend と一緒に動かす

別の端末で `oraculum-server` を起動してから、開発 server を起動します。
`go -C backend run` は `oraculum-server` の作業ディレクトリを `backend/` にするため、
原資料の path は `backend/` からの相対で指定します。原資料はリポジトリの外に置きます。
リポジトリと同じ親ディレクトリに置いた `oraculum-data/` は `../../oraculum-data/` で
指します。

```bash
go -C backend run ./cmd/oraculum-server \
  --attack-rules rules/attack \
  'squid_combined:../../oraculum-data/proxy/access.log' \
  'infotrace_mark_ii:../../oraculum-data/endpoint/host-a.log'
```

`oraculum-server` が受け取る原資料の指定は `<formatKey>:<相対 path>` の形です。
絶対 path を渡した起動は
`parsing source argument ...: expected <formatKey>:<relative path>` で止まります。

自機の名前も IP も記録しない原資料 (Linux の監査ログなど) は、原資料の指定の直前に
`--terminal-id <識別子>`、`--terminal-name <表示名>`、`--terminal-ip <IP>` を置くと、
その原資料を記録した端末を指定できます。3 つのうち分かるものだけを渡します。指定は直後の
原資料 1 件だけに付きます。端末を指定しない Linux の監査ログのプロセス・ファイル・
アカウントは、「`<file 名>` を記録した端末 (名前不明)」のノードにつながります。

```bash
go -C backend run ./cmd/oraculum-server \
  --attack-rules rules/attack \
  --terminal-name host-g --terminal-ip 192.0.2.14 \
  'linux_auditd:../../oraculum-data/linux/host-g.log'
```

Windows の Prefetch (`windows_prefetch`) は 1 つの `.pf` を 1 件の原資料として読み、file は
端末を名乗りません。同じ端末の `.pf` を 1 台の端末に置くには、各原資料の直前に同じ
`--terminal-id` を付けます。file 名は空白を含むことがあるため、引数を配列に組みます。
壊れていて読めない `.pf` は、端末に付けずに失敗として残ります。

```bash
cd backend
args=()
args+=(--attack-rules rules/attack)
for f in ../../oraculum-data/host-a/Prefetch/*.pf; do
  args+=(--terminal-id host-a --terminal-name host-a "windows_prefetch:$f")
done
go run ./cmd/oraculum-server "${args[@]}"
```

registry の hive (`windows_registry_hive`) は、hive の主 file (`SYSTEM`、`NTUSER.DAT` など) を
1 件ずつ指定します。同じ directory に `<主 file の名前>.LOG1` と `.LOG2` があれば一緒に読み、
主 file が書き込みの途中の状態であるときは log を適用した後の key と値を出します。
log の file は指定しません。key 1 つが 1 件のレコードになり、値は `Value.<値の名前>` の欄で
検索できます。

```bash
go -C backend run ./cmd/oraculum-server \
  --attack-rules rules/attack \
  'windows_registry_hive:../../oraculum-data/host-a/Registry/SYSTEM' \
  'windows_registry_hive:../../oraculum-data/host-a/Registry/SOFTWARE'
```

Windows イベントログに Sigma のルールを当てるときは、`--sigma-rules <directory>` を渡します。
directory の下の `.yml` をすべて読みます。SigmaHQ の repository を clone し、`rules/windows` を
渡します。git の作業ツリーの中の directory を渡すと、使ったルールの版として HEAD の commit を記録します。

```bash
git clone https://github.com/SigmaHQ/sigma.git ../../sigma
go -C backend run ./cmd/oraculum-server \
  --attack-rules rules/attack \
  --sigma-rules ../../sigma/rules/windows \
  'windows_evtx:../../oraculum-data/endpoint/host-a-security.evtx'
```

一致したルールは、画面の「Sigma ルールの候補」と「調べる順序の目安」で使います。
「調べる順序の目安」で手法に Sigma を選ぶと、数えるルールのレベルの下限を選べます。
既定はすべてのレベルです。

`oraculum-server` は起動引数の原資料を読み込んでから `127.0.0.1:8080` で待ち受け、
待ち受けを始めた後にグラフを組みます。画面は、グラフを組み終えるまで読み込みと処理の進行を
出し、組み終えると調査の画面に切り替わります。待ち受け先は `--addr <host:port>` で
変えられます。終了は `Ctrl+C` です。

`oraculum-server` は、要求の Host が自分の待ち受け先であり、状態を変える要求の Origin が
自分の origin である要求だけに答えます。同じ端末の `127.0.0.1`、`localhost`、`[::1]` と
待ち受けの port の組は、常に許可します。別の端末の browser から開くときは、`--addr` を
loopback 以外にし、browser が開くアドレスを `--allowed-host <host:port>` で渡します。
`--allowed-host` は繰り返し渡せます。

別の端末の分析者は、アカウントでログインします。loopback 以外の `--addr` は、`--allowed-host`、
アカウントの file の `--accounts <file>`、調査の directory の `--investigation <dir>` を必要とします。
アカウントは `oraculum-server accounts` で管理します。`add` と `passwd` は、パスワードを標準入力の
最初の行から読みます。`disable` はその利用者のセッションをすべて失効させ、server の起動中も
次の要求から作用します。

```bash
go -C backend run ./cmd/oraculum-server accounts add --accounts ../../oraculum-accounts.sqlite \
  alice --display-name 石橋 < alice-password.txt
go -C backend run ./cmd/oraculum-server --addr 0.0.0.0:8080 --allowed-host 192.0.2.10:8080 \
  --accounts ../../oraculum-accounts.sqlite --admin alice --investigation ../../oraculum-investigation \
  --attack-rules rules/attack \
  --source-root ../../oraculum-data
go -C backend run ./cmd/oraculum-server accounts disable --accounts ../../oraculum-accounts.sqlite alice
```

利用者は、調査の役割 (閲覧者・編集者・管理者) を持つときだけ調査を開けます。`--admin <login>` は
起動のたびにそのアカウントを調査の管理者にします。管理者は画面の「利用者と役割」で、ほかの
アカウントに役割を与えます。役割を持つ利用者が 1 人もいない調査は、`--admin` を渡さないと
起動しません。

`--accounts` を渡した起動は、ログインした利用者のログイン名を所見と端末割当の著者にします。
loopback で `--accounts` を渡さない起動は、ログインを求めず、著者は画面で入力した名前です。
通信は暗号化しません。

原資料を画面から指定するときは、原資料を置いた directory を `--source-root <dir>` で渡し、
原資料の指定を渡さずに起動します。画面で、その directory からの相対 path と入力形式を
収集元ごとに入力して「読み込みを始める」を押し、読み込みを終えたら「処理を始める」を
押します。画面は `--source-root` の外を指す path を読み込みません。

```bash
go -C backend run ./cmd/oraculum-server --attack-rules rules/attack --source-root ../../oraculum-data
```

`--investigation <dir>` を一緒に渡すと、画面から読み込んだ収集元と端末の指定を調査に
記録し、次の起動では `--investigation <dir>` だけで同じ収集元を読み込みます。

開発 server は `/api` で始まる要求を `oraculum-server` へ渡します。画面と API が同じ origin に
なるため、画面は CORS を扱いません。開発 server は、要求の Host と、開発 server 自身の origin から
届いた要求の Origin を、渡す先の値に書き換えます。

| 環境変数 | 読む場所 | 意味 | 既定値 |
|---|---|---|---|
| `VITE_ORACULUM_API_BASE_URL` | 画面 | backend の API の接続先 | `/api/v0` |
| `ORACULUM_API_TARGET` | `vite.config.ts` | 要求を渡す先の `oraculum-server` | `http://127.0.0.1:8080` |

`VITE_ORACULUM_API_BASE_URL` は `frontend/.env.local` に書くか、`pnpm -C frontend dev` の
実行時に環境変数として渡します。`ORACULUM_API_TARGET` は `pnpm -C frontend dev` の実行時に
渡します。

## server から画面を配信する

開発 server を使わずに画面を開くときは、画面を build し、build の directory を
`oraculum-server` の `--frontend-dir <dir>` に渡します。server は API と同じ origin で画面を
配信し、`http://127.0.0.1:8080/` で画面を開けます。

repo root で次を実行します。

```bash
pnpm -C frontend build
go -C backend build -o oraculum-server github.com/Intellectual-Chasen/Oraculum/backend/cmd/oraculum-server
backend/oraculum-server --frontend-dir frontend/dist --source-root ../oraculum-data
```

server と同じ commit で画面を build します。build の API の基点は `/api/v0` のままにします。
`VITE_ORACULUM_API_BASE_URL` に別の origin を設定した build は、server から配信しても
その origin の API を読みます。

```bash
make frontend-verify
```

型検査、Biome と dependency-cruiser、描画と依存境界のテスト、本番用ビルドを実行します。

## server から画面を配信する

開発 server を使わずに画面を開くときは、画面を build し、build の directory を
`oraculum-server` の `--frontend-dir <dir>` に渡します。server は API と同じ origin で画面を
配信し、`http://127.0.0.1:8080/` で画面を開けます。

repo root で次を実行します。

```bash
pnpm -C frontend build
go -C backend build -o oraculum-server github.com/Intellectual-Chasen/Oraculum/backend/cmd/oraculum-server
backend/oraculum-server --frontend-dir frontend/dist --source-root ../oraculum-data
```

server と同じ commit で画面を build します。build の API の基点は `/api/v0` のままにします。
`VITE_ORACULUM_API_BASE_URL` に別の origin を設定した build は、server から配信しても
その origin の API を読みます。

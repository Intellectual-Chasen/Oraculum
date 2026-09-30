#import "../lib/theme.typ": shot, console, ui

= 使ってみる

本章では、JPCERT/CC のログ分析トレーニングのログで、Oraculum の操作を 2 つの例で説明します。
1 つ目の例では、「ログ分析トレーニング バージョン2」の実践編のログを取り込み、1 つのアカウントを手がかりに、アカウントを作った記録と、アカウントの作成の前後のログオンを順に確かめます。
2 つ目の例では、「ログ分析トレーニング」のハンズオン 3 と 4 のログを取り込み、PowerShell から news-landsbbc.co への通信を、根拠のレコードまでたどります。

== ログを取り込む

1 つ目の例では、3 台の端末の Security と System のイベントログと、Proxy の Squid のアクセスログを取り込みます。
実践編のログのディレクトリに移り、サーバを起動します。
Squid のアクセスログは、Squid の設定の `logformat` の並びをファイルに書いて `--logformat-file` で渡します。
Squid のアクセスログの時刻は UTC+09:00 のタイムゾーンで記録されているため、`--time-offset +09:00` を収集元ごとに付けます。
`--attack-rules` には、Oraculum のリポジトリの `backend/rules/attack` のディレクトリを指定します。

#console("cd log-analysis-training_v2/Hands-on/advance\nprintf '%s' 'logformat squid %{%Y/%m/%d %H:%M:%S}tl.%03tu %6tr %>a %Ss/%03>Hs %<st %rm %ru %[un %Sh/%<a %mt' > logformat.txt\noraculum-server --investigation ./case1 \\\n  --attack-rules /path/to/oraculum/backend/rules/attack \\\n  windows_evtx:Hands-on-1/Security.evtx windows_evtx:Hands-on-1/System.evtx \\\n  windows_evtx:Hands-on-2/Security.evtx windows_evtx:Hands-on-2/System.evtx \\\n  windows_evtx:Hands-on-3/Security.evtx windows_evtx:Hands-on-3/System.evtx \\\n  --logformat-file logformat.txt \\\n  --time-offset +09:00 squid_logformat:Hands-on-4/access.log.1 \\\n  --time-offset +09:00 squid_logformat:Hands-on-4/access.log",
  output: "Security.evtx (Hands-on-1) published_full read=59898 succeeded=59898 failed=0\n...\naccess.log.1 published_full read=11301 succeeded=11301 failed=0\naccess.log published_partial read=57992 succeeded=57962 failed=30\nbuilding the graph for the default selection\nlistening on 127.0.0.1:8080")

`access.log` は `published_partial` になり、`failed=30` が出力されます。
30 行は途中で切れた行です。切れる前に読めたフィールドはレコードとして取り込まれています。
取り込めなかったレコードの位置と理由は、Artifacts のビューで確かめられます。確かめ方は「困ったとき」の章で説明します。
画面をブラウザで開く手順は「準備と起動」の章で説明します。

== 手がかりで検索する

Search のビューの#ui[条件を追加]の入力欄を押すと、条件の種類の一覧が表示されます。
#ui[文字列]の#ui[含む]を選ぶと、#ui[含む文字列]の入力欄が表示されます。
`eviluser` を入力して Enter を押します。

#shot("../images/qs-search-editor.png", width: 40%, caption: [#ui[含む]を選んで値を入力した入力欄])

条件は#ui[含む文字列 eviluser]として入力欄の中に表示されます。
Graph のビューには、一致ノードと、一致ノードをつなぐエッジが表示されます。
Search のビューの#ui[結果]には、#ui[一致ノード: 12]と、ノードの種類ごとの件数として#ui[アカウント 9]と#ui[IP アドレス 3]が表示されます。

#shot("../images/qs-graph.png",
  marks: ((0.59, 0.415, 0.075, 0.045, "1"), (0.66, 0.483, 0.10, 0.075, "2")),
  caption: [eviluser を含むレコードから作ったグラフ。1 は同一性の基準がアカウントの SID の eviluser、2 は 10.10.100.104 から client-win2-C.handsonlab.local へのエッジです。])

グラフには、eviluser、itmanager、domuser、testadmin001 のアカウント、client-win2-C.handsonlab.local などの端末、10.10.100.104 などの IP アドレスが表示されます。
eviluser のノードは 3 つあります。1 つは SID で特定したアカウントです。残りの 2 つはドメインとログイン名で特定したアカウントで、ドメインが client-win2-C のアカウントと、ドメインが localhost のアカウントです。ノードが分かれる理由は「Graph」の節で説明します。
10.10.100.104 から client-win2-C.handsonlab.local への橙色のエッジは、凡例の#ui[推定したエッジ]です。

== アカウントを作った記録を確かめる

グラフの eviluser のノードを押すと、Node Detail のビューにノードの詳細が表示されます。
#ui[同一性の基準]が#ui[アカウントの SID]のノードを選びます。
#ui[作成レコード]の表には、eviluser を作成した記録として、Security.evtx (Hands-on-2) の 2023-10-11 00:46:32.278815 のレコードが表示されます。
表を右へスクロールすると、Event ID の 4720 と、位置の 1373672-1376008 が表示されます。

#shot("../images/qs-detail-node.png", width: 45%,
  marks: ((0.02, 0.42, 0.96, 0.05, "1"), (0.02, 0.655, 0.96, 0.17, "2")),
  caption: [eviluser のノードの詳細。1 は同一性の基準、2 は作成レコードです。])

表の位置の値を押すと、Record のビューにレコードが表示されます。
Record のビューの上端には、#ui[Event ID: 4720]、#ui[収集元: Security.evtx (Hands-on-2)]、#ui[位置: 1373672-1376008]が表示されます。
位置の 1373672-1376008 は、Hands-on-2 の `Security.evtx` の中のバイトの範囲です。
#ui[原文]から、`SubjectUserName` の itmanager が `TargetUserName` の eviluser を作成したことを確かめられます。

#shot("../images/qs-record.png", width: 80%,
  marks: ((0.0, 0.0, 0.77, 0.045, "1"), (0.016, 0.39, 0.966, 0.605, "2")),
  caption: [eviluser を作成したレコード。Record のビューを最大化して表示しています。1 は収集元と位置、2 は原文です。])

== ログオンのエッジを確かめる

Graph のビューで、10.10.100.104 から client-win2-C.handsonlab.local へのエッジを押します。
Edge Detail のビューが前面に表示され、#ui[エッジの種類]に#ui[接続元が不明のリモートセッションの候補]、#ui[作り方]に#ui[推定]が表示されます。

#shot("../images/qs-detail-edge.png", width: 45%,
  marks: ((0.05, 0.582, 0.9, 0.03, "1"), (0.05, 0.915, 0.9, 0.03, "2")),
  caption: [10.10.100.104 から client-win2-C.handsonlab.local へのエッジの詳細。1 と 2 は各グループのログオンタイプです。])

#ui[根拠のグループ]は、根拠のレコードを、イベントの種類、接続先のポート、ログオンタイプの組ごとに分けています。
10.10.100.104 から client-win2-C.handsonlab.local へのエッジでは、根拠のレコードはどれも Event ID 4624 のログオンのイベントで、ログオンタイプで 2 つのグループに分かれます。

#table(
  columns: (auto, auto, 1fr),
  [ログオンタイプ], [根拠のレコード数], [内容],
  [3], [4], [itmanager と eviluser が NTLM で認証しています。3 はネットワークからのログオンです。],
  [10], [2], [eviluser が Negotiate で認証してログオンしています。10 はリモートデスクトップによるログオンです。],
)

Edge Detail のビューの下部の表には、根拠のレコードが 6 件表示されます。表の位置の値を押すと、ログオンのイベントを 1 件ずつ Record のビューで開けます。

== 前後のイベントを時刻の順に見る

Record のビューの#ui[前後のレコード]には、開いたレコードの前後 60 秒のレコードが表示されます。
eviluser を作成したレコードでは、#ui[同じ端末: 12]と#ui[全端末: 135]の 2 つの表が表示されます。
前後の幅は#ui[前後の幅]で変えられます。

#shot("../images/qs-record-context.png", width: 70%, caption: [eviluser を作成したレコードの前後のレコード])

== ブックマークとメモに記録する

Record のビュー、Node Detail のビュー、Edge Detail のビューの上部には、ブックマークのアイコンのボタンが表示されます。
ボタンを押すと、表示している対象がブックマークに記録されます。Record のビューで eviluser を作成したレコードを開いているときは、ボタンの名前は#ui[Event ID: 4720、収集元: Security.evtx (Hands-on-2)、位置: 1373672-1376008 をブックマークに追加]です。
記録したブックマークは Bookmarks のビューで確かめられます。
Node Detail のビューと Edge Detail のビューの#ui[メモ]では、選んでいるノードやエッジについてのメモを、根拠のレコードとともに記録できます。

== ハンズオン 3 と 4 の問いを調べる

2 つ目の例では、「ログ分析トレーニング」のハンズオン 3 と 4 のログで、端末 Win10_64JP_09 の PowerShell が news-landsbbc.co と通信したことを、根拠のレコードまでたどります。
取り込むのは、Win10_64JP_09 の Security、Sysmon、PowerShell のイベントログを CSV に書き出したファイルと、Proxy の Squid のアクセスログです。
CSV のファイルには記録した端末の名前の列が無いため、`--terminal-id`、`--terminal-name`、`--terminal-ip` で記録した端末を収集元ごとに指定します。

#console("cd log-analysis-training/Hands-on\nT='--time-offset +09:00 --terminal-id Win10_64JP_09 --terminal-name Win10_64JP_09 --terminal-ip 192.168.16.109'\noraculum-server --investigation ./case2 \\\n  --attack-rules /path/to/oraculum/backend/rules/attack \\\n  $T windows_event_viewer_csv:Handson3/Security.csv \\\n  $T windows_event_viewer_csv:Handson3/Sysmon.csv \\\n  $T windows_event_viewer_csv:Handson3/Powershell.csv \\\n  --terminal-id Proxy --terminal-name Proxy --terminal-ip 192.168.16.10 squid_combined:Handson4/access.log",
  output: "Security.csv published_full read=33621 succeeded=33621 failed=0\nSysmon.csv published_full read=1653 succeeded=1653 failed=0\nPowershell.csv published_full read=22 succeeded=22 failed=0\naccess.log published_full read=93001 succeeded=93001 failed=0\nbuilding the graph for the default selection\nlistening on 127.0.0.1:8080")

=== 条件を追加する

Search のビューで#ui[含む]を選び、`news-landsbbc.co` を入力して Enter を押します。
Search のビューの#ui[結果]には、#ui[一致ノード: 12]と、#ui[ホスト名 1]、#ui[ファイル 5]、#ui[IP アドレス 1]、#ui[プロセス 5]が表示されます。

=== Graph でエッジを選ぶ

Graph のビューには、ホスト名の news-landsbbc.co のノードと、news-landsbbc.co を含むコマンド行を記録した powershell.exe と cmd.exe のプロセスのノードが表示されます。
news-landsbbc.co のノードと powershell.exe のノードをつなぐ橙色のエッジにマウスを重ねると、エッジの種類と根拠のグループの数が#ui[収集元をまたぐプロセスの対応の候補 · 2]として表示されます。
エッジを押します。

#shot("../images/qs-news-graph.png",
  marks: ((0.627, 0.575, 0.16, 0.125),),
  caption: [news-landsbbc.co を含むレコードから作ったグラフ。印の位置は、マウスを重ねたエッジと、エッジの種類の表示です。])

=== Edge Detail で根拠を確かめる

Edge Detail のビューには、次の値が表示されます。

#table(
  columns: (auto, 1fr),
  [名前], [値],
  [#ui[エッジの種類]], [#ui[収集元をまたぐプロセスの対応の候補]],
  [#ui[作り方]], [#ui[推定]],
  [#ui[始点]], [news-landsbbc.co],
  [#ui[終点]], [`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe` のプロセスです。端末は Win10_64JP_09、PID は 5764、PID が使われ始めた時刻は 2019-11-07T15:16:56 です。],
)

#ui[根拠のグループ]には、2 つのグループが表示されます。

#table(
  columns: (auto, auto, 1fr),
  [グループ], [根拠のレコード数], [内容],
  [#ui[接続先 port] が 80、#ui[HTTP の状態] が 200], [1], [Proxy のアクセスログの要求です。],
  [#ui[EventID: 5156]、#ui[接続先 port] が 8080], [1], [Security のイベントログの接続の許可です。],
)

#ui[エッジを作った推定]には、#ui[段階 2: 同じ秒の時刻]と、推定の元にしたレコードとして#ui[元のレコード: 収集元: access.log、行: 6309]が表示されます。
下部の表には、根拠のレコードが 2 件表示されます。

#shot("../images/qs-news-edge.png", width: 45%,
  marks: ((0.02, 0.2, 0.96, 0.115, "1"), (0.05, 0.47, 0.9, 0.165, "2"), (0.05, 0.685, 0.9, 0.25, "3")),
  caption: [news-landsbbc.co から powershell.exe へのエッジの詳細。1 は始点と終点、2 は Proxy の要求のグループ、3 は Event ID 5156 のグループです。])

=== Record で原文を読む

下部の表の access.log の行の位置の値 6309 を押すと、Record のビューにアクセスログの 6309 行目が表示されます。
#ui[原文]は次の 1 行です。

```
192.168.16.109 - - [07/Nov/2019:15:16:57 +0900] "GET http://news-landsbbc.co/upload/21.jpg HTTP/1.1" 200 183667 "-" "-" TCP_MEM_HIT:NONE
```

192.168.16.109 は Win10_64JP_09 の IP アドレスです。
Proxy は 2019 年 11 月 7 日 15:16:57 に、192.168.16.109 からの `http://news-landsbbc.co/upload/21.jpg` の要求を記録しています。

#shot("../images/qs-news-record.png", width: 80%,
  marks: ((0.016, 0.385, 0.966, 0.11),),
  caption: [access.log の 6309 行目。Record のビューを最大化して表示しています。])

次に、Edge Detail のビューに戻り、Security.csv の行の位置の値 11689205-11689911 を押します。
Record のビューの上端には、#ui[Event ID: 5156]、#ui[収集元: Security.csv]、#ui[行: 290754-290771]、#ui[位置: 11689205-11689911]が表示されます。
原文から、同じ 15:16:57 に、プロセス ID 5764 の `powershell.exe` が、送信元アドレス 192.168.16.109 から宛先アドレス 192.168.16.10 の宛先ポート 8080 へ接続したことを確かめられます。
192.168.16.10 は Proxy の IP アドレスです。

#shot("../images/qs-news-record-5156.png", width: 80%,
  marks: ((0.016, 0.367, 0.966, 0.064, "1"), (0.016, 0.494, 0.966, 0.086, "2")),
  caption: [Security.csv の Event ID 5156 のレコード。1 はプロセス ID とアプリケーション名、2 は送信元と宛先です。Record のビューを最大化して表示しています。])

2 件のレコードから、Win10_64JP_09 の PID 5764 の PowerShell が Proxy に接続した秒と、Proxy が news-landsbbc.co への要求を記録した秒が一致することを確かめられます。
Oraculum は、2 件の時刻が同じ秒であることから、要求をしたプロセスの候補として PowerShell を推定しています。

#import "lib/theme.typ": theme

#show: theme.with(title: "Oraculum 利用者ガイド")

#page(header: none, {
  align(center + horizon)[
    #text(size: 28pt, weight: "bold")[Oraculum 利用者ガイド]
  ]
  place(bottom + center, dy: -10mm, text(size: 14pt)[インテリ茶筅])
})

#page(header: none, outline(title: [目次], depth: 2))

// 章の番号は include の順で決まる。
#include "chapters/overview.typ"
#include "chapters/setup.typ"
#include "chapters/quick-start.typ"

= 画面の構成と共同作業
#include "chapters/views/header.typ"
#include "chapters/views/workspaces.typ"
#include "chapters/views/members.typ"

= ビューごとの操作
#include "chapters/views/search.typ"
#include "chapters/views/graph.typ"
#include "chapters/views/node-detail.typ"
#include "chapters/views/edge-detail.typ"
#include "chapters/views/path.typ"
#include "chapters/views/record.typ"
#include "chapters/views/timeline.typ"
#include "chapters/views/artifacts.typ"
#include "chapters/views/hosts.typ"
#include "chapters/views/ips.typ"
#include "chapters/views/histogram.typ"
#include "chapters/views/detection.typ"
#include "chapters/views/priority.typ"
#include "chapters/views/bookmarks.typ"
#include "chapters/views/time-host.typ"

// 入力形式ごとの説明は、この版のガイドに含めない。adapters/ の file は残す。
// = 入力形式
// #include "adapters/windows-evtx.typ"
// #include "adapters/windows-event-xml.typ"
// #include "adapters/squid.typ"
// #include "adapters/apache.typ"
// #include "adapters/auditd.typ"
// #include "adapters/prefetch.typ"
// #include "adapters/registry.typ"
// #include "adapters/royalts.typ"
// #include "adapters/markii.typ"

#include "chapters/troubleshooting.typ"

= 資料の出典
#include "credits.typ"

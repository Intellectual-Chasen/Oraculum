// Command oraculum-assist は、分析者の端末で動く AI 支援の中継である。
//
// 分析者は `oraculum-assist --server <Oraculum server の URL>` を起動し、表示された URL を browser で
// 開く。中継は画面と API を server へ転送し、分析者本人がログインした `claude` の CLI と会話する。
package main

import "os"

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

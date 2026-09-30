// Package squid は Squid のアクセスログのレコードを文字列へ分け、時刻と要求行を解釈する。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリと backend/core である。
// External Tool: 無し。呼び出し側が開いた収集元を io.Reader で受け取る。
//
// 本 package が読める入力形式は Formats が宣言する。欄の並びは Layout が持ち、
// どの並びで読むかは Reader を作る側が渡す。並びを定めるのは logformat の文字列であり、
// ParseLogFormat が欄の並びへ直す。組み込みの combined の文字列は CombinedSpec、
// 要求の byte 数を足した文字列は CombinedRequestBytesSpec が持つ。
//
// logformat は Squid と同じ文法で読む (ParseLogFormat)。対応する format code は codes.go の
// 表であり、Squid の TokenTable を写す。組み込みの logformat は名前で指定できる (LayoutForSpec)。
// %tl と %tg の日時の書式は strftime の変換で読む (strftime.go)。
//
// Limitations: 表に無い code を含む logformat は、収集元を読む前に error になる。
// **未知の code を読み飛ばさない。** 読み飛ばすと後続の欄の位置がすべてずれ、診断を
// 出さずに別の意味の値を取り込む。literal を挟まずに隣り合う 2 つの code も読む前に
// error にする。値の境目が出力に残らないためである。
// 区切りの literal を値の中に持つ欄 (encoding を付けない %ssl::>cert_subject など) は、
// 行が余るため文字列の分割の失敗になる。
//
// 仕様の出典は https://www.squid-cache.org/Doc/config/logformat/ と、Squid の
// src/format/Token.cc と src/format/Format.cc である。
// 改行は LF と CR LF。レコードから Squid のバージョンを判定できない。文字コードを自動判定せず、byte 列を保持する。
// item に literal な CR / LF / TAB / NUL があればレコードごと文字列の分割の失敗にする。
// 引用符内の \r / \n / \t は復号して値に保持する。
// 復号後の authority は制御 byte (0x00–0x1F と 0x7F) を拒み、normalize の失敗にする。
// 時刻の code は要求完了時点の Proxy の時計であり、meaning は event とする。UTC からの
// ずれの扱いは parseSpecifiedTime が定める。
// Item.RawValue は core の rawText に対応し、外側の引用符、時刻の角括弧、escape を含む
// 原資料の文字列を保持する。Item.Value は外側の引用符を外し、引用符の中の escape を復号する。
// RecordFields は ItemOrderOf(layout) が定める欄を core.RecordField にする。値の不在を表す
// 文字列 - が入った欄を absent にする。端末への割当、収集元の識別、%ru から導く
// requestTargetHost と requestTargetPort は組み立てない。
// 状態コード 0 を out_of_definition にする裁定は確定しているが、Phase 1/2 では未実装である。
// ParseRequestLine は要求行の - を欠測へ読み替えず原資料の文字列のまま返す。
// ParseRequestLine が返す AuthorityHost は userinfo と port と IPv6 の角括弧を外した文字列で
// あり、%ru 全体の原資料の文字列は RawTarget が保つ。
//
// 共通の意味の語彙への対応は SemanticOfItem と RequestTargetHostSemantic と
// SemanticRequestMethod と SemanticRequestTargetPort が公開する。RecordFields は
// SemanticOfItem を当てた欄を返す。ident と requestLine と squidStatus と mimeType と
// upstreamIp と upstreamStatusCode とヘッダーの欄は空の値になり、この入力形式に固有の
// 意味を持つ。%> は client との間、%< は上位の server との間を指し、2 つを別の欄にする。
// upstreamIp と upstreamStatusCode が語彙の項目を持たないのは、語彙の接続先と状態符号が
// client から見た値を指し、proxy と上位の server の間の値と別の意味であるためである。
// ident は組み込みの combined がリテラルの - を置く位置、
// requestLine は要求行全体の原資料の文字列、squidStatus は Squid の要求処理の結果と上位への
// 転送の経路である。requestLine の 3 token へ意味を与えるのは、%ru から
// requestMethod と requestTargetHost と requestTargetPort を組み立てる呼び出し側で
// ある。RequestTargetHostSemantic は host の文字列が IP アドレスかホスト名かで語彙の項目を
// 選び、authority を書いていない要求先には空の値を返す。
package squid

// Package sigma は利用者が配置した Sigma ルールの file を読み、Windows イベントログの
// レコードに当てる形へ直す。
//
// 出力元は Sigma の仕様に従って書かれた YAML の検出ルールである。ルールの集合は利用者が
// directory に置き、本 package は置かれた file だけを読む。ネットワークからルールを取得しない。
// ルールの集合の commit は、起動が渡した commit の文字列か、directory を追跡する git の作業ツリーの
// HEAD から決める (revision.go)。
//
// Hexagonal Layer: adapters。依存先は標準ライブラリと go.yaml.in/yaml/v3 である。
// backend/pipeline と backend/api と他の adapter を import しない。
//
// External Tool: 無し。git の HEAD と index は file を読んで決め、git の command を起動しない。
//
// Limitations:
//   - 評価できる logsource は product が windows で、service か category が logsource.go の
//     表にあるものに限る。表に無い logsource のルールは評価しなかったルールに数える。
//   - 評価できる detection は、項目名と値の組の map と、その map の list である。
//     項目名を持たない値 (キーワードの検索) は評価しない。
//   - 評価できる修飾子は contains、startswith、endswith、all、windash、re (i、m、s)、cidr、
//     exists である。ほかの修飾子を含むルールは評価しない。
//   - 評価できる condition は and、or、not、括弧、`1 of`、`all of` である。集計 (`|`) と
//     ほかの数の `of` は評価しない。
//   - re は Go の正規表現 (RE2) で評価する。RE2 が受け付けない式 (先読みなど) のルールは
//     評価しない。
//   - 1 つの file に複数の YAML 文書を持つルール (ルールの集まり、相関のルール) は評価しない。
//   - logsource が指すイベントのうち、記録する項目の集合を表に持つのは Security の 4688 だけで
//     ある。ほかのイベントは、ルールが参照する項目をイベントが記録するとみなす。
//   - 項目を名前で持たないレコード (Record.Named が偽) には、その項目を参照するルールを当てない。
//   - git の index はバージョン 2 から 4 を読む。読めない index の作業ツリーの HEAD をルールの集合の commit にしない。
//   - git の HEAD は、作業ツリーの未 commit の変更を表さない。file の内容の識別は
//     ContentSha256 が表す。
package sigma

// Package markii は markii 形式のクライアントログの 1 レコードを文字列へ分け、観測へ直す。
//
// Hexagonal Layer: adapters。依存先は backend/core と標準ライブラリだけである。
// 兄弟 adapter、自身の subpackage、backend/output、os/exec を import しない。
//
// External Tool: 無し。外部プロセスを起動しない。入力は Recorder が出力した file の byte 列を
// io.Reader で受け取る。本 package が読める入力形式は Formats が宣言する。
//
// Limitations: 対応を確認した文字符号化は UTF-8 (BOM 無し)、改行は CR LF と LF である。
// 他のバージョン・設定・文字符号化の出力は未確認である。レコードが形式のバージョンを表す key を持たない
// ため、本 package はバージョンを判定できない。値が "-" の欄を欠測へ読み替えない。key の並びを
// 固定と扱わない。文字列の分割は空文字列と "-" と key の不在を別の形で返すところまでを行い、
// 値を数値や path へ変換しない。変換するのは意味付けであり、sn の 10 進整数への変換と
// ヘッダーの時刻の core.Timestamp への変換の 2 つに限る。
//
// レコードの形は、固定長のヘッダー 29 文字と、半角空白 1 個と、key=value の並びである。
// ヘッダーは key を持たず、位置で決まる。先頭 23 文字が日時、25 文字目からの 5 文字が
// UTC からのずれの文字列である。
//
// 引用符で囲んだ value の中の引用符は、引用符を 2 個並べて表す。引用符 1 個を value の
// 終端として読む処理は、その形の value を途中で切る。本 package は引用符 2 個を 1 個と
// して読み進める。
//
// 公開するのは文字列の分割の Record と Field と Reader、およびレコードの種別ごとの
// 意味付けである。プロセス開始記録の意味付けは ProcessStart と ParseProcessStart と
// IsProcessStart と ProcessStartParserID を公開する。通信記録の意味付けは
// Communication と ParseCommunication と IsCommunication と CommunicationParserID を
// 公開する。種別に依らない意味付けは RecordObservation と
// ParseRecordObservation と RecordObservationParserID を公開し、任意の evt と subEvt の
// レコードを受け付ける。
//
// file 名が process_start で始まる file はプロセス開始記録の意味付け、communication で始まる
// file は通信記録の意味付け、record_observation で始まる file は種別に依らない意味付けを持つ。
// 接頭辞を持たない file は 3 つが共有し、文字列の分割と共通の意味付けを持つ。
package markii

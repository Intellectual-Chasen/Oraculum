package core

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// 件数と通番は `int64` で受け、負の値を Validate で拒否する。

// validatable は自身の項目の整合を確かめる組である。
type validatable interface {
	Validate() error
}

// validateElements は集合の要素を 1 つずつ確かめ、検査に失敗した要素の位置を error に添える。
//
// 位置を添えるのは、集合の中のどの要素が検査に失敗したかを呼ぶ側が特定できるようにするため
// である。item には集合の項目名を渡す。
func validateElements[T validatable](item string, values []T) error {
	for index, value := range values {
		if err := value.Validate(); err != nil {
			return itemError(item+" at "+formatIndex(index), err)
		}
	}
	return nil
}

// requireNoEmptyElement は文字列の集合が空文字列の要素を持たないことを確かめる。
//
// 文字列の集合の要素は値を持つ。空文字列の要素は、値の無い要素を集合に
// 入れた形である。
func requireNoEmptyElement(item string, values []string) error {
	for index, value := range values {
		if value == "" {
			return itemError(item+" has an empty element at "+formatIndex(index),
				ErrMissingRequiredItem)
		}
	}
	return nil
}

// ErrInvalid は本 package の不変条件に反する値を表す。本 package が返す error はすべて
// ErrInvalid を包む。
var ErrInvalid = errors.New("core: value violates the contract")

// ErrUnknownEnumValue は列挙の定義の外にある値を表す。
var ErrUnknownEnumValue = fmt.Errorf("%w: unknown enumeration value", ErrInvalid)

// ErrMissingRequiredItem は必須の項目に値が無いことを表す。
var ErrMissingRequiredItem = fmt.Errorf("%w: required item is absent", ErrInvalid)

// ErrUnexpectedItem は出てはいけない項目に値があることを表す。
var ErrUnexpectedItem = fmt.Errorf("%w: item must be absent", ErrInvalid)

// ErrNegativeCount は件数と byte 数に負の値があることを表す。
var ErrNegativeCount = fmt.Errorf("%w: count must not be negative", ErrInvalid)

// ErrDuplicateElement は一意でなければならない要素が集合の中で重なっていることを表す。
var ErrDuplicateElement = fmt.Errorf("%w: duplicate element", ErrInvalid)

// ErrTooManyElements は集合の要素数が上限を超えていることを表す。
var ErrTooManyElements = fmt.Errorf("%w: too many elements", ErrInvalid)

// ErrControlCharacter は無害化した文字列に制御文字または改行が入っていることを表す。
var ErrControlCharacter = fmt.Errorf("%w: sanitized text contains a control character", ErrInvalid)

// ErrInconsistentValue は 2 つ以上の項目の値が互いに矛盾していることを表す。
var ErrInconsistentValue = fmt.Errorf("%w: items contradict each other", ErrInvalid)

// ErrCountOverflow は件数の和が int64 の範囲を超えることを表す。
var ErrCountOverflow = fmt.Errorf("%w: the sum of counts overflows", ErrInvalid)

// sha256HexLength は contentSha256 の文字数である。
const sha256HexLength = 64

// itemError は項目の名前を前に置いた error を返す。書式が `%s` で始まるため、
// 項目の名前が大文字で始まっても error message の書き方の検査に掛からない。
func itemError(item string, cause error) error {
	return fmt.Errorf("%s: %w", item, cause)
}

// firstProblem は項目ごとの検査の結果を並びで受け取り、最初の違反を返す。
// 引数は呼び出しの時点ですべて評価されるため、pointer の参照外しを伴う検査は
// nil の判定を済ませてから渡す。
func firstProblem(problems ...error) error {
	for _, problem := range problems {
		if problem != nil {
			return problem
		}
	}
	return nil
}

// formatIndex は集合の中の位置を error message に載せる形にする。
func formatIndex(index int) string {
	return "index " + strconv.Itoa(index)
}

// unknownEnumError は列挙の定義の外にある値の error を返す。
func unknownEnumError(item, value string) error {
	return fmt.Errorf("%s %q: %w", item, value, ErrUnknownEnumValue)
}

// enumValue は定義の中にある値かを自分で判定できる列挙である。
type enumValue interface {
	~string
	IsKnown() bool
}

// requireKnownEnum は列挙の値が定義の中にあることを確かめる。
func requireKnownEnum[E enumValue](item string, value E) error {
	if !value.IsKnown() {
		return unknownEnumError(item, string(value))
	}
	return nil
}

// requireKnownEnumWhenPresent は省略可の列挙が、出ているときに定義の中にあることを
// 確かめる。空の値は項目が出ていない状態である。
func requireKnownEnumWhenPresent[E enumValue](item string, value E) error {
	if value == "" {
		return nil
	}
	return requireKnownEnum(item, value)
}

// requirePresent は必須の文字列の項目に値があることを確かめる。
func requirePresent(item, value string) error {
	if value == "" {
		return itemError(item, ErrMissingRequiredItem)
	}
	return nil
}

// requireAbsent は出てはいけない文字列の項目が空であることを確かめる。
func requireAbsent(item, value string) error {
	if value != "" {
		return itemError(item, ErrUnexpectedItem)
	}
	return nil
}

// requireNonNegative は件数と byte 数が負でないことを確かめる。
func requireNonNegative(item string, value int64) error {
	if value < 0 {
		return itemError(item, ErrNegativeCount)
	}
	return nil
}

// requireSanitized は診断ログと端末出力に出す文字列に制御文字と改行が無いことを確かめる。
func requireSanitized(item, value string) error {
	if strings.ContainsFunc(value, unicode.IsControl) {
		return itemError(item, ErrControlCharacter)
	}
	return nil
}

// requireLowerHex64 は sha256 の 16 進表現が小文字 64 文字であることを確かめる。
func requireLowerHex64(item, value string) error {
	if len(value) != sha256HexLength {
		return itemError(item, ErrInvalid)
	}
	for _, r := range value {
		isDigit := r >= '0' && r <= '9'
		isLowerHexLetter := r >= 'a' && r <= 'f'
		if !isDigit && !isLowerHexLetter {
			return itemError(item, ErrInvalid)
		}
	}
	return nil
}

// IsAbsolutePath は path の文字列が絶対 path であるかを返す。
//
// core は I/O を持たないため、判定は OS に依らず文字列だけで行う。`/` か `\` で始まる形
// (UNC の `\\host\share` を含む) と、ドライブ文字を持つ形 (`C:\logs`、`C:/logs`) を絶対 path と
// 数える。
func IsAbsolutePath(value string) bool {
	if strings.HasPrefix(value, "/") || strings.HasPrefix(value, `\`) {
		return true
	}
	const driveLetterPrefixLength = 3
	if len(value) < driveLetterPrefixLength {
		return false
	}
	if value[1] != ':' || (value[2] != '/' && value[2] != '\\') {
		return false
	}
	letter := value[0]
	return (letter >= 'A' && letter <= 'Z') || (letter >= 'a' && letter <= 'z')
}

// requireRelativePath は利用者が配置した位置を指す path が、相対 path であることを確かめる。
// SourceIdentity.originPath は、取り込みの基準の directory からの相対 path を持つ。
//
// 既知の制限: 判定は絶対 path でないことまでである,
// path が基準の directory の下にあるかは file system を読まないと決まらず、core は I/O を
// 持たない。originPath を受け取るのは運用者が渡す起動引数だけであり、API の要求は path を
// 受け取らないため、信頼境界は起動引数である,
// API か画面から path を受け取るようにしたときに見直す。
func requireRelativePath(item, value string) error {
	if IsAbsolutePath(value) {
		return itemError(item+" is an absolute path", ErrInvalid)
	}
	return nil
}

// requireDecodedNumber は JSON から復元した必須の 10 進整数が入っていたことを確かめる。
// 欠けた項目を 0 へ復元すると、「値が無い」と「値が 0」が同じ形になる。
func requireDecodedNumber(item string, value *int64) error {
	if value == nil {
		return itemError(item, ErrMissingRequiredItem)
	}
	return nil
}

// requireDecodedFlag は JSON から復元した必須の真偽が入っていたことを確かめる。
// 欠けた項目を偽へ復元すると、「値が無い」と「値が偽」が同じ形になる。
func requireDecodedFlag(item string, value *bool) error {
	if value == nil {
		return itemError(item, ErrMissingRequiredItem)
	}
	return nil
}

// requireBothOrNeither は 2 つの文字列の項目が揃って出るか、揃って出ないことを確かめる。
func requireBothOrNeither(firstItem, firstValue, secondItem, secondValue string) error {
	if (firstValue == "") != (secondValue == "") {
		return fmt.Errorf("%s and %s: %w", firstItem, secondItem, ErrInconsistentValue)
	}
	return nil
}

// emptyIfNil は nil の集合を要素数 0 の集合にする。要素数 0 の場合も集合である必須の集合は、
// 要素が 1 件も無い応答でも集合として直列化する。
func emptyIfNil[E any](values []E) []E {
	if values == nil {
		return []E{}
	}
	return values
}

// sumCounts は非負の件数を足した和を返す。int64 の範囲を超える入力は error を返す。
// 桁あふれで和が負の値または 0 へ回り込むと、和の一致の検査が通ってしまう。
func sumCounts(item string, counts ...int64) (int64, error) {
	total := int64(0)
	for _, count := range counts {
		if count < 0 {
			return 0, itemError(item, ErrNegativeCount)
		}
		if total > math.MaxInt64-count {
			return 0, itemError(item, ErrCountOverflow)
		}
		total += count
	}
	return total, nil
}

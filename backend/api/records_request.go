package api

import (
	"errors"
	"net/http"
	"net/url"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// `GET /api/v0/records` の要求の項目の名前。
const (
	sequenceNumberParam            = "sequenceNumber"
	lineNumberParam                = "lineNumber"
	byteOffsetParam                = "byteOffset"
	originSourceIdParam            = "originSourceId"
	originSourceContentSha256Param = "originSourceContentSha256"
	originSequenceNumberParam      = "originSequenceNumber"
	originLineNumberParam          = "originLineNumber"
	originByteOffsetParam          = "originByteOffset"
)

// 位置を持つ 10 進整数の下限。行番号は 1 起点であり、通番と byte 位置は非負である
// (core.RecordLocator.Validate)。
const (
	firstLineNumber      = 1
	lowestSequenceNumber = 0
	lowestByteOffset     = 0
	shortestByteLength   = 1
)

// originParams は起点のレコードを指す項目である。並びは収集元、内容の識別、通番、行番号、
// byte 位置の順である。
var originParams = []string{
	originSourceIdParam, originSourceContentSha256Param,
	originSequenceNumberParam, originLineNumberParam, originByteOffsetParam,
}

// recordsRequest は`/api/v0/records` の要求の項目である。
type recordsRequest struct {
	record requestedRecord
	// origin は起点のレコードである。起点の項目を与えない要求では nil である。
	origin *requestedRecord
	// selection は経路を組む関連付けの条件である。origin を持つ要求だけが読む。
	selection pipeline.MatchConditionSelection
}

// requestedRecord は要求が収集元と位置で指した 1 レコードである。
type requestedRecord struct {
	source   requestedSource
	position requestedPosition
}

// requestedPosition は要求が与えた収集元の中の位置である。
// sequenceNumber と lineNumber と byteOffset のうち 1 つ以上に値が入る。
type requestedPosition struct {
	sequenceNumber *int64
	lineNumber     *int64
	byteOffset     *int64
}

// parseRecordsRequest は`/api/v0/records` の要求を読む。
//
// **`code` の判定は本関数に書いた上からの順で行う。** 要求の値だけで決まる判定をすべて
// ここで行い、取り込み結果を要する判定を呼び出し元に残す。
func parseRecordsRequest(r *http.Request) (recordsRequest, *core.ApiError) {
	query := r.URL.Query()
	if err := checkRecordsParameterNames(query); err != nil {
		return recordsRequest{}, invalidRecordsRequest(err, nil)
	}
	missing := missingRecordsParameters(query)
	originGiven := originItemsGiven(query)
	if originGiven {
		missing = append(missing, missingOriginParameters(query)...)
	}
	if len(missing) > 0 {
		return recordsRequest{}, invalidRecordsRequest(
			errors.New("required query parameters are missing"), missing)
	}
	record, err := readRequestedRecord(query, sourceIdParam, sourceContentSha256Param,
		sequenceNumberParam, lineNumberParam, byteOffsetParam)
	if err != nil {
		return recordsRequest{}, invalidRecordsRequest(err, nil)
	}
	request := recordsRequest{record: record}
	if !originGiven {
		return request, nil
	}
	origin, err := readRequestedRecord(query, originSourceIdParam, originSourceContentSha256Param,
		originSequenceNumberParam, originLineNumberParam, originByteOffsetParam)
	if err != nil {
		return recordsRequest{}, invalidRecordsRequest(err, nil)
	}
	selection, err := readMatchConditions(query)
	if err != nil {
		return recordsRequest{}, invalidRecordsRequest(err, []string{matchConditionParam})
	}
	request.origin, request.selection = &origin, selection
	return request, nil
}

// readRequestedRecord は、収集元と内容の識別と位置を持つ項目の名前を指して 1 レコードを読む。
// 名前が本体のレコードと起点のレコードで異なるため、名前を引数で受け取る。
func readRequestedRecord(
	query url.Values, sourceIdName, contentName, sequenceName, lineName, byteOffsetName string,
) (requestedRecord, error) {
	source := requestedSource{sourceId: query.Get(sourceIdName), contentSha256: query.Get(contentName)}
	// 原資料に実在する値と、項目を与えていない状態を分けるため、空の値を退ける。
	if source.sourceId == "" || source.contentSha256 == "" {
		return requestedRecord{}, errors.New(sourceIdName + " and " + contentName + " must not be empty")
	}
	position, err := readPositionOf(query, sequenceName, lineName, byteOffsetName)
	if err != nil {
		return requestedRecord{}, err
	}
	return requestedRecord{source: source, position: position}, nil
}

// checkRecordsParameterNames は要求の項目の名前と多重度を確かめる。
//
// 未知の項目は invalid_request の条件に入っていない。綴り誤りを通知せずに既定値で
// 処理する応答を避けるため拒否する。同じ判定は、query の項目の値に
// RecordField の直列化形を持たせた要求にも該当する。
//
// **`limit` と `cursor` は名前の一覧に入らない。** 2 つの操作の応答は 1 件のレコードの
// 組であり、上限を適用する対象を持たない。適用する対象の無い項目を受理して値を捨てる形は、
// 綴り誤りを通知せずに処理する応答と同じになる。
//
// **matchCondition だけは繰り返しを許す。** 1 つの条件につき 1 回書く。
func checkRecordsParameterNames(query url.Values) error {
	known := map[string]struct{}{
		sourceIdParam: {}, sourceContentSha256Param: {},
		sequenceNumberParam: {}, lineNumberParam: {}, byteOffsetParam: {},
		originSourceIdParam: {}, originSourceContentSha256Param: {},
		originSequenceNumberParam: {}, originLineNumberParam: {},
		originByteOffsetParam: {}, matchConditionParam: {},
	}
	for name, values := range query {
		if _, ok := known[name]; !ok {
			return errors.New("unsupported query parameter")
		}
		if len(values) != 1 && name != matchConditionParam {
			return errors.New("query parameter must occur once")
		}
	}
	return nil
}

// originItemsGiven は、起点の項目と関連付けの条件のどれか 1 つでも与えた要求かを返す。
//
// **関連付けの条件は起点の組の項目である。** 起点の無い要求に条件を与えても使う先が無く、
// 値を通知せずに捨てる応答は綴り誤りを通知せずに処理する応答と同じになる。
func originItemsGiven(query url.Values) bool {
	for _, name := range append([]string{matchConditionParam}, originParams...) {
		if _, given := query[name]; given {
			return true
		}
	}
	return false
}

// missingRecordsParameters は、開くレコードを指す項目のうち欠けている必須の項目を、
// 本関数が並べた項目の順で返す。
//
// 位置の sequenceNumber と lineNumber と byteOffset は 1 つ以上が必須である。どれも与えて
// いない要求は 3 つの名前を持つ。
// 必須の項目を 1 つ以上欠いた要求への応答は、値が受け取る範囲の外にある条件へ同時に
// 該当する場合も missingParameters を出す。
func missingRecordsParameters(query url.Values) []string {
	return missingRecordParameters(query, sourceIdParam, sourceContentSha256Param,
		sequenceNumberParam, lineNumberParam, byteOffsetParam)
}

// missingOriginParameters は、起点のレコードを指す項目と関連付けの条件のうち欠けている項目を返す。
//
// **起点の組は、全部あるか全部無いかのどちらかである。** 組の項目を 1 つでも与えた要求は、
// 収集元の識別子と内容の識別と位置 1 つ以上と、関連付けの条件 1 つ以上を持つ。
func missingOriginParameters(query url.Values) []string {
	missing := missingRecordParameters(query, originSourceIdParam, originSourceContentSha256Param,
		originSequenceNumberParam, originLineNumberParam, originByteOffsetParam)
	if _, given := query[matchConditionParam]; !given {
		missing = append(missing, matchConditionParam)
	}
	return missing
}

// missingRecordParameters は、1 レコードを指す項目の名前を受け取り、欠けている項目を返す。
func missingRecordParameters(
	query url.Values, sourceIdName, contentName string, positionNames ...string,
) []string {
	var missing []string
	for _, name := range []string{sourceIdName, contentName} {
		if _, given := query[name]; !given {
			missing = append(missing, name)
		}
	}
	for _, name := range positionNames {
		if _, given := query[name]; given {
			return missing
		}
	}
	return append(missing, positionNames...)
}

// readPositionOf は、通番と行番号と byte 位置を持つ項目の名前を指して位置を読む。
// 名前が操作ごとに異なるため、名前を引数で受け取る。
func readPositionOf(
	query url.Values, sequenceName, lineName, byteOffsetName string,
) (requestedPosition, error) {
	position := requestedPosition{}
	read := func(name string, lowest int64, into **int64) error {
		if _, given := query[name]; !given {
			return nil
		}
		value, err := readDecimalInteger(query, name, lowest)
		if err != nil {
			return err
		}
		*into = &value
		return nil
	}
	for _, item := range []struct {
		name   string
		lowest int64
		into   **int64
	}{
		{sequenceName, lowestSequenceNumber, &position.sequenceNumber},
		{lineName, firstLineNumber, &position.lineNumber},
		{byteOffsetName, lowestByteOffset, &position.byteOffset},
	} {
		if err := read(item.name, item.lowest, item.into); err != nil {
			return requestedPosition{}, err
		}
	}
	return position, nil
}

// readDecimalInteger は 10 進整数の項目を、下限とともに読む。
func readDecimalInteger(query url.Values, name string, lowest int64) (int64, error) {
	value, err := strconv.ParseInt(query.Get(name), 10, 64)
	if err != nil || value < lowest {
		return 0, errors.New(name + " must be a decimal integer of " +
			strconv.FormatInt(lowest, 10) + " or more")
	}
	return value, nil
}

func invalidRecordsRequest(err error, missing []string) *core.ApiError {
	return &core.ApiError{
		Code:              core.ApiErrorCodeInvalidRequest,
		Message:           err.Error(),
		MissingParameters: missing,
	}
}

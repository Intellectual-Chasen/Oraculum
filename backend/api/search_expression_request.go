package api

import (
	"errors"
	"net/url"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

// searchExpressionParam は、欄と演算子で書いた検索式を持つ要求の項目である。`/api/v0/graph` と
// 時系列の操作が読む。
const searchExpressionParam = "searchExpression"

// searchExpressionRequest は検索式を持つ要求の項目である。部分グラフの要求と時系列の要求が
// 同じ項目を読むため、両方が本型を埋め込む。
type searchExpressionRequest struct {
	// searchExpressionText は要求が与えた式の文字列である。読んだ式は searchExpression が持つ。
	searchExpressionText string
	// searchExpression は読んだ式である。項目を書かない要求では nil である。
	searchExpression *pipeline.SearchExpression
}

// readSearchExpression は検索式の項目を読む。
//
// **空の文字列と空白だけの文字列も式として読み、誤りの範囲を付けて退ける。** 画面が入力欄の
// 近くに誤りを出せるよう、どの構文の誤りも同じ形で返す。
func (r *searchExpressionRequest) readSearchExpression(query url.Values) *core.ApiError {
	if _, given := query[searchExpressionParam]; !given {
		return nil
	}
	text := query.Get(searchExpressionParam)
	expression, err := pipeline.ParseSearchExpression(text)
	if err != nil {
		return searchQueryError(err)
	}
	r.searchExpressionText, r.searchExpression = text, expression
	return nil
}

// searchQueryError は検索の条件の誤りを invalid_request の失敗にする。検索式の構文の誤りには、
// 誤りの範囲を添える。
func searchQueryError(err error) *core.ApiError {
	apiError := invalidRequestError(err, nil)
	var syntax *pipeline.SearchExpressionSyntaxError
	if errors.As(err, &syntax) {
		detail := syntax.Detail
		apiError.SearchExpressionError = &detail
	}
	return apiError
}

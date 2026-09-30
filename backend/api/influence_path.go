package api

import (
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const influencePathPattern = "GET /api/v0/influence-path"

// 影響の経路の要求の項目の名前。
const (
	influenceFromParam = "from"
	influenceToParam   = "to"
	excludeBasisParam  = "excludeBasis"
)

// influencePathHandler は、起点と終点のノードの間の影響の経路に乗る影響のエッジを返す。
type influencePathHandler struct {
	graph pipeline.Graph
}

// influencePathResponse は影響の経路の応答である。**本型が項目の定義元である。**
type influencePathResponse struct {
	// From と To は要求の起点と終点のノードの識別子である。
	From string `json:"from"`
	To   string `json:"to"`
	// ExcludedBases は要求が除いた根拠の種類である。要求に書いた順を保つ。
	ExcludedBases []core.InfluenceBasis `json:"excludedBases"`
	// Origin と Destination は起点と終点である。ノードがタイムスタンプを持つレコードを持たないときは出ない。
	Origin      *core.InfluenceEndpoint `json:"origin,omitempty"`
	Destination *core.InfluenceEndpoint `json:"destination,omitempty"`
	// Vertices は Edges の端と、起点と終点の要素である。Frontier のノードは Frontier の項目が持つ。
	Vertices []core.InfluenceVertex `json:"vertices"`
	// Edges は起点から終点へのどれかの経路に乗る影響のエッジである。本数が上限を超えるときは、
	// 起点から終点への短い経路に乗るエッジから順に上限までを持ち、載せなかった本数は OmittedEdgeCount である。
	Edges            []core.InfluenceEdge `json:"edges"`
	OmittedEdgeCount int64                `json:"omittedEdgeCount"`
	// Stops は Edges が空である理由、または求めきれなかった理由である。
	Stops []core.InfluencePathStop `json:"stops"`
	// RouteExcludedBases は、Stops が route_through_excluded_basis を持つときの、除いた根拠の種類を
	// 戻して求めた経路のエッジが持つ、除いた根拠の種類である。ほかのときは空である。
	RouteExcludedBases []core.InfluenceBasis `json:"routeExcludedBases"`
	// OriginNodeKind と DestinationNodeKind は、Stops が origin_not_influence_end または
	// destination_not_influence_end を持つときの、その端のノードの種別である。ほかのときは出ない。
	OriginNodeKind      core.NodeKind `json:"originNodeKind,omitempty"`
	DestinationNodeKind core.NodeKind `json:"destinationNodeKind,omitempty"`
	// Frontier は、起点から届いたが先へ影響が進まないノードの先頭からの一部であり、総数は
	// FrontierCount である。
	Frontier      []core.InfluenceFrontier `json:"frontier"`
	FrontierCount int64                    `json:"frontierCount"`
	// UntimedRecordCount は、タイムスタンプを持たず影響のエッジにしなかった根拠のレコードの数である。
	UntimedRecordCount int64 `json:"untimedRecordCount"`
}

func (h influencePathHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query, apiError := parseInfluencePathRequest(r.URL.Query())
	if apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	started := time.Now()
	path, found := h.graph.InfluencePath(query)
	// 探索の状態の数と所要を、上限を見直す記録に残す。原資料の文字列を載せない。
	slog.Info("influence path computed", "states", path.StateCount, "edges", len(path.Edges),
		"elapsed", time.Since(started))
	if !found {
		writeError(w, http.StatusNotFound, core.ApiError{
			Code: core.ApiErrorCodeRecordNotFound, Message: "no node matches the requested from or to",
		})
		return
	}
	writeJSON(w, http.StatusOK, influencePathResponse{
		From: query.From, To: query.To, ExcludedBases: emptyIfNil(query.Excluded),
		Origin: path.Origin, Destination: path.Destination,
		Vertices: emptyIfNil(path.Vertices), Edges: emptyIfNil(path.Edges), OmittedEdgeCount: int64(path.OmittedEdgeCount),
		Stops: emptyIfNil(path.Stops), RouteExcludedBases: emptyIfNil(path.RouteExcludedBases),
		OriginNodeKind: path.OriginKind, DestinationNodeKind: path.DestinationKind,
		Frontier: emptyIfNil(path.Frontier), FrontierCount: int64(path.FrontierCount),
		UntimedRecordCount: int64(path.UntimedRecordCount),
	})
}

// parseInfluencePathRequest は影響の経路の要求を読む。関連付けの条件は matchSelectingHandler が読む。
//
// from と to は 1 回ずつ必須である。excludeBasis は根拠の種類の値であり、1 つにつき 1 回書く。
func parseInfluencePathRequest(query url.Values) (pipeline.InfluencePathQuery, *core.ApiError) {
	for name, values := range query {
		switch name {
		case influenceFromParam, influenceToParam:
			if len(values) != 1 {
				return pipeline.InfluencePathQuery{}, invalidRequestError(errors.New(name+" must occur once"), nil)
			}
		case excludeBasisParam, matchConditionParam:
		default:
			return pipeline.InfluencePathQuery{}, invalidRequestError(errors.New("unsupported query parameter"), nil)
		}
	}
	var missing []string
	for _, name := range []string{influenceFromParam, influenceToParam} {
		if query.Get(name) == "" {
			missing = append(missing, name)
		}
	}
	if len(missing) > 0 {
		return pipeline.InfluencePathQuery{}, invalidRequestError(errors.New("required query parameters are missing"), missing)
	}
	request := pipeline.InfluencePathQuery{From: query.Get(influenceFromParam), To: query.Get(influenceToParam)}
	for _, value := range query[excludeBasisParam] {
		basis := core.InfluenceBasis(value)
		if !basis.IsKnown() {
			return pipeline.InfluencePathQuery{}, invalidRequestError(errors.New(excludeBasisParam+" carries an unknown basis"), nil)
		}
		for _, held := range request.Excluded {
			if held == basis {
				return pipeline.InfluencePathQuery{}, invalidRequestError(errors.New(excludeBasisParam+" repeats a basis"), nil)
			}
		}
		request.Excluded = append(request.Excluded, basis)
	}
	return request, nil
}

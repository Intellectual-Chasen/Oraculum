package api_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"testing"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
)

const influencePathPath = "/api/v0/influence-path"

// influencePathResponse は応答の項目名を test 側で固定する。
type influencePathResponse struct {
	From          string                   `json:"from"`
	To            string                   `json:"to"`
	ExcludedBases []core.InfluenceBasis    `json:"excludedBases"`
	Origin        *core.InfluenceEndpoint  `json:"origin"`
	Destination   *core.InfluenceEndpoint  `json:"destination"`
	Vertices      []core.InfluenceVertex   `json:"vertices"`
	Edges         []core.InfluenceEdge     `json:"edges"`
	Omitted       *int64                   `json:"omittedEdgeCount"`
	Stops         []core.InfluencePathStop `json:"stops"`
	RouteBases    []core.InfluenceBasis    `json:"routeExcludedBases"`
	Frontier      []core.InfluenceFrontier `json:"frontier"`
	FrontierCount int64                    `json:"frontierCount"`
	Untimed       *int64                   `json:"untimedRecordCount"`
}

func requestInfluencePath(t *testing.T, handler http.Handler, query string) *httptest.ResponseRecorder {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet,
		withAllMatchConditions(influencePathPath+"?"+query), nil))
	return response
}

// 向きを決められない記録と関連付けの候補を除いても、祖先から孫への経路は子の起動の影響のエッジを
// 通る。応答の項目は型の定義元のとおりに出る。
func TestInfluencePathEndpointFollowsTheProcessStarts(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	ancestor := graphNodeWithIdentity(t, whole, "T1", "{P0}")
	child := graphNodeWithIdentity(t, whole, "T1", "{P7}")
	response := requestInfluencePath(t, handler, "from="+url.QueryEscape(ancestor.Id)+"&to="+url.QueryEscape(child.Id)+
		"&excludeBasis=undetermined_direction&excludeBasis=candidate")
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var decoded influencePathResponse
	decodeJSON(t, response.Body, &decoded)
	if decoded.From != ancestor.Id || decoded.To != child.Id || decoded.Origin == nil || decoded.Destination == nil ||
		decoded.ExcludedBases == nil || decoded.Stops == nil || decoded.Frontier == nil || decoded.Untimed == nil ||
		decoded.Omitted == nil {
		t.Fatalf("response = %+v", decoded)
	}
	if len(decoded.Edges) == 0 {
		t.Fatalf("no influence edge from the ancestor to the child: stops=%v", decoded.Stops)
	}
	parent := graphNodeWithIdentity(t, whole, "T1", "{P1}")
	starts := map[[2]string]bool{}
	for _, edge := range decoded.Edges {
		if edge.GraphEdgeKind == core.EdgeKindProcessParentChild {
			starts[[2]string{edge.SourceKey, edge.TargetKey}] = true
		}
		if err := edge.Validate(); err != nil {
			t.Errorf("edge %s: %v", edge.Id, err)
		}
	}
	if !starts[[2]string{ancestor.Id, parent.Id}] || !starts[[2]string{parent.Id, child.Id}] {
		t.Errorf("process starts on the path = %v, want {P0} → {P1} → {P7}", starts)
	}
	for _, vertex := range decoded.Vertices {
		if err := vertex.Validate(); err != nil {
			t.Errorf("vertex %s: %v", vertex.Key, err)
		}
	}

	excluded := requestInfluencePath(t, handler, "from="+url.QueryEscape(ancestor.Id)+"&to="+
		url.QueryEscape(child.Id)+"&excludeBasis=specified_operation")
	decoded = influencePathResponse{}
	decodeJSON(t, excluded.Body, &decoded)
	if len(decoded.Edges) != 0 || !slices.Equal(decoded.Stops, []core.InfluencePathStop{
		core.InfluencePathStopNoInfluenceRoute, core.InfluencePathStopRouteThroughExcludedBasis,
	}) || !slices.Equal(decoded.RouteBases, []core.InfluenceBasis{core.InfluenceBasisSpecifiedOperation}) {
		t.Errorf("excluded: edges=%d stops=%v bases=%v, want no route and the excluded basis on the route",
			len(decoded.Edges), decoded.Stops, decoded.RouteBases)
	}
	for _, item := range decoded.Frontier {
		if err := item.Validate(); err != nil {
			t.Errorf("frontier %s: %v", item.Key, err)
		}
	}
}

// 要求の項目の誤りは invalid_request、グラフに無いノードは record_not_found である。
func TestInfluencePathEndpointRejectsInvalidRequests(t *testing.T) {
	handler := graphHandler(t)
	whole := decodeGraph(t, handler, wholeGraphQuery)
	node := url.QueryEscape(graphNodeWithIdentity(t, whole, "T1", "{P0}").Id)
	for _, query := range []string{
		"to=" + node,
		"from=" + node,
		"from=" + node + "&from=" + node + "&to=" + node,
		"from=" + node + "&to=" + node + "&excludeBasis=unknown",
		"from=" + node + "&to=" + node + "&excludeBasis=candidate&excludeBasis=candidate",
		"from=" + node + "&to=" + node + "&depth=1",
	} {
		response := requestInfluencePath(t, handler, query)
		var apiError core.ApiError
		decodeJSON(t, response.Body, &apiError)
		if response.Code != http.StatusBadRequest || apiError.Code != core.ApiErrorCodeInvalidRequest {
			t.Errorf("%q: status=%d code=%q, want invalid_request", query, response.Code, apiError.Code)
		}
	}
	response := requestInfluencePath(t, handler, "from="+node+"&to=n:terminal:absent")
	var apiError core.ApiError
	decodeJSON(t, response.Body, &apiError)
	if response.Code != http.StatusNotFound || apiError.Code != core.ApiErrorCodeRecordNotFound {
		t.Errorf("unknown node: status=%d code=%q, want record_not_found", response.Code, apiError.Code)
	}
}

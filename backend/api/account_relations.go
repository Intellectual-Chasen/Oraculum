package api

import (
	"errors"
	"net/http"
	"net/url"
	"slices"
	"strconv"

	"github.com/Intellectual-Chasen/Oraculum/backend/core"
	"github.com/Intellectual-Chasen/Oraculum/backend/pipeline"
)

const accountRelationsPattern = "GET /api/v0/account-relations"
const counterpartIdParam = "counterpartId"
const originRoleParam = "originRole"
const otherRoleParam = "otherRole"
const relationOffsetParam = "offset"

type accountRelationsHandler struct {
	graph pipeline.Graph
	sigma pipeline.SigmaEvaluation
}

func (h accountRelationsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	if err := checkAccountRelationsParameters(query); err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	if query.Get(accountNodeIdParam) == "" {
		writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("accountNodeId is required"), []string{accountNodeIdParam}))
		return
	}
	if apiError := checkKnownNodesForParameter(h.graph, []string{query.Get(accountNodeIdParam)}, accountNodeIdParam); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	origin, _ := h.graph.NodeDetail(query.Get(accountNodeIdParam))
	if origin.Node.Kind != core.NodeKindAccount {
		writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("accountNodeId must name an account"), nil))
		return
	}
	records, err := readRecordConditionsRequest(query)
	if err != nil {
		writeError(w, http.StatusBadRequest, *invalidRequestError(err, nil))
		return
	}
	if apiError := checkKnownRecordConditions(h.graph, records.conditions); apiError != nil {
		writeError(w, httpStatusFor(apiError.Code), *apiError)
		return
	}
	roles := make([]core.EdgeKind, 0, len(query[edgeKindParam]))
	for _, name := range query[edgeKindParam] {
		kind := core.EdgeKind(name)
		if !accountRelationRole(kind) || slices.Contains(roles, kind) {
			writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("edgeKind must name each account role once"), nil))
			return
		}
		roles = append(roles, kind)
	}
	var selected *pipeline.AccountRelationKey
	offset := 0
	if _, given := query[counterpartIdParam]; given {
		if query.Get(counterpartIdParam) == "" || query.Get(originRoleParam) == "" || query.Get(otherRoleParam) == "" {
			writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("counterpartId requires both roles"), nil))
			return
		}
		if apiError := checkKnownNodesForParameter(h.graph, []string{query.Get(counterpartIdParam)}, counterpartIdParam); apiError != nil {
			writeError(w, httpStatusFor(apiError.Code), *apiError)
			return
		}
		detail, _ := h.graph.NodeDetail(query.Get(counterpartIdParam))
		if detail.Node.Kind != core.NodeKindAccount || query.Get(counterpartIdParam) == query.Get(accountNodeIdParam) ||
			!accountRelationRole(core.EdgeKind(query.Get(originRoleParam))) || !accountRelationRole(core.EdgeKind(query.Get(otherRoleParam))) {
			writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("counterpartId and roles must name another account and its roles"), nil))
			return
		}
		selected = &pipeline.AccountRelationKey{CounterpartId: query.Get(counterpartIdParam), OriginRole: core.EdgeKind(query.Get(originRoleParam)), OtherRole: core.EdgeKind(query.Get(otherRoleParam))}
		if query.Has(relationOffsetParam) {
			var parseError error
			offset, parseError = strconv.Atoi(query.Get(relationOffsetParam))
			if parseError != nil || offset < 0 || uint64(offset) > uint64(core.MaxSafeInteger) {
				writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("offset must be a non-negative safe integer"), nil))
				return
			}
		}
	} else if _, first := query[originRoleParam]; first || query.Has(otherRoleParam) {
		writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("roles require counterpartId"), nil))
		return
	} else if query.Has(relationOffsetParam) {
		writeError(w, http.StatusBadRequest, *invalidRequestError(errors.New("offset requires counterpartId"), nil))
		return
	}
	writeJSON(w, http.StatusOK, h.graph.AccountRelations(query.Get(accountNodeIdParam), records.recordFilter(), roles, selected, offset, h.sigma))
}

func accountRelationRole(kind core.EdgeKind) bool {
	return kind == core.EdgeKindRecordSubjectAccount || kind == core.EdgeKindRecordTargetAccount || kind == core.EdgeKindRecordNamesObject
}

func checkAccountRelationsParameters(query url.Values) error {
	known := map[string]struct{}{
		accountNodeIdParam: {}, counterpartIdParam: {}, originRoleParam: {}, otherRoleParam: {}, relationOffsetParam: {}, edgeKindParam: {},
		eventCategoryParam: {}, eventActionParam: {}, eventActionFromParam: {}, eventActionToParam: {},
		timeFromParam: {}, timeFromPrecisionParam: {}, timeToParam: {}, timeToPrecisionParam: {}, filterUnitParam: {},
		caseParam: {}, terminalParam: {}, sourceParam: {}, matchConditionParam: {},
	}
	if err := unsupportedParameterError(query, known); err != nil {
		return err
	}
	for name, values := range query {
		if len(values) != 1 && name != edgeKindParam && !repeatedRequestItem(name) {
			return errors.New("query parameter must occur once")
		}
	}
	if missing := missingTimelineParameters(query); len(missing) > 0 {
		return errors.New("time bounds require precision and filterUnit")
	}
	for _, name := range []string{eventCategoryParam, eventActionParam, sourceParam} {
		if slices.Contains(query[name], "") {
			return errors.New(name + " must not be empty")
		}
	}
	return nil
}

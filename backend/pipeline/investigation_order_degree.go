package pipeline

// InvestigationOrderDegree は、対象の組の相手になる異なる対象の数の多い順である。
const InvestigationOrderDegree = "degree"

// degreeValues は、対象を a または b に持つ組の数を値にする。組は 2 つの対象ごとに 1 つなので、
// 組の数は相手の異なり数と同じである。組を持たない対象の値は 0 である。
func degreeValues(_ Graph, input orderInput) []orderValue {
	values := make([]orderValue, len(input.objects))
	for i := range values {
		values[i].present = true
	}
	for _, pair := range input.pairs {
		values[pair.a].value++
		values[pair.b].value++
	}
	return values
}

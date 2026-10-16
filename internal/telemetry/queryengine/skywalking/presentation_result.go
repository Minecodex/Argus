package skywalking

import "github.com/graphql-go/graphql/language/ast"

// PresentationResults freezes each selected root independently. Response keys
// retain root aliases; nested aliases are projected to their schema field names.
// The original tool result remains unchanged for analysis and file delivery.
func PresentationResults(document, operation string, payload map[string]any) ([]any, error) {
	d, err := parseResultDocument(document, operation)
	if err != nil {
		return nil, err
	}
	views := []any{}
	positions := map[string]int{}
	for _, root := range d.roots {
		key := root.Name.Value
		if root.Alias != nil {
			key = root.Alias.Value
		}
		value, exists := payload[key]
		if !exists {
			continue
		} // @skip/@include can legitimately omit a root.
		canonical, _ := d.presentationValue(value, root.SelectionSet).(map[string]any)
		kind := apmShapes[root.Name.Value]
		if kind == "" {
			kind = "traces"
		}
		view := map[string]any{"key": key, "field": root.Name.Value, "kind": kind, "rows": []any{}}
		for field, value := range canonical {
			view[field] = value
		}
		switch root.Name.Value {
		case "queryTrace":
			view["kind"] = "trace_detail"
		case "queryTraceGraph":
			view["kind"] = "trace_graph"
		case "queryBasicTraces", "queryBasicTracesByName", "queryTraces":
			if rows, ok := canonical["traces"].([]any); ok {
				view["rows"] = rows
			}
			delete(view, "traces")
		}
		if index, ok := positions[key]; ok {
			views[index] = mergeProjection(views[index], view)
		} else {
			positions[key] = len(views)
			views = append(views, view)
		}
	}
	RefreshPresentationPreviews(views)
	return views, nil
}

func RefreshPresentationPreviews(views []any) {
	for _, raw := range views {
		if view, ok := raw.(map[string]any); ok && view["kind"] == "apm_red" {
			AttachREDPreviews(view)
		}
	}
}

func (d resultDocument) presentationValue(value any, selection *ast.SelectionSet) any {
	switch item := value.(type) {
	case []any:
		rows := make([]any, len(item))
		for i, row := range item {
			rows[i] = d.presentationValue(row, selection)
		}
		return rows
	case map[string]any:
		out := map[string]any{}
		for _, field := range d.fields(selection) {
			key := field.Name.Value
			if field.Alias != nil {
				key = field.Alias.Value
			}
			if child, exists := item[key]; exists {
				out[field.Name.Value] = mergeProjection(out[field.Name.Value], d.presentationValue(child, field.SelectionSet))
			}
		}
		return out
	default:
		return value
	}
}

func mergeProjection(left, right any) any {
	switch r := right.(type) {
	case map[string]any:
		if l, ok := left.(map[string]any); ok {
			for key, value := range r {
				l[key] = mergeProjection(l[key], value)
			}
			return l
		}
	case []any:
		if l, ok := left.([]any); ok && len(l) == len(r) {
			for i, value := range r {
				l[i] = mergeProjection(l[i], value)
			}
			return l
		}
	}
	return right
}

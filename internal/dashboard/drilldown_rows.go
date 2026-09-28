package dashboard

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

func selectionValues(input map[string]string) map[string]Selection {
	values := map[string]Selection{}
	for name, value := range input {
		values[name] = Selection{Values: []string{value}}
	}
	return values
}

func matchDrilldownRow(data any, drill Drilldown, selected map[string]string) (bool, error) {
	if len(selected) != len(drill.Inputs) || len(selected) > 16 {
		return false, ErrInvalid
	}
	for name, value := range selected {
		if _, ok := drill.Inputs[name]; !ok || len(value) > 4096 {
			return false, ErrInvalid
		}
	}
	if len(selected) == 0 {
		return true, nil
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return false, err
	}
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	var normalized any
	if err := decoder.Decode(&normalized); err != nil {
		return false, err
	}
	var visit func(any, int) bool
	visit = func(value any, depth int) bool {
		if depth > 32 {
			return false
		}
		switch item := value.(type) {
		case map[string]any:
			matches := true
			for name, pointer := range drill.Inputs {
				actual, ok := rowPointer(item, pointer)
				if !ok || actual != selected[name] {
					matches = false
					break
				}
			}
			if matches {
				return true
			}
			for _, child := range item {
				if visit(child, depth+1) {
					return true
				}
			}
		case []any:
			for _, child := range item {
				if visit(child, depth+1) {
					return true
				}
			}
		}
		return false
	}
	return visit(normalized, 0), nil
}

func rowPointer(value any, pointer string) (string, bool) {
	if !validRowPointer(pointer) {
		return "", false
	}
	for _, part := range strings.Split(pointer[1:], "/") {
		key := strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		switch item := value.(type) {
		case map[string]any:
			var ok bool
			value, ok = item[key]
			if !ok {
				return "", false
			}
		case []any:
			index, err := strconv.Atoi(key)
			if err != nil || index < 0 || index >= len(item) {
				return "", false
			}
			value = item[index]
		default:
			return "", false
		}
	}
	switch scalar := value.(type) {
	case string:
		return scalar, true
	case json.Number:
		return scalar.String(), true
	case bool:
		return strconv.FormatBool(scalar), true
	default:
		return "", false
	}
}

func applyDrillWindow(scope queryScope, drill Drilldown, selected map[string]string) (queryScope, error) {
	if drill.TimeWindow == nil {
		return scope, nil
	}
	at, err := time.Parse(time.RFC3339Nano, selected[drill.TimeWindow.Input])
	if err != nil {
		return scope, fmt.Errorf("%w: invalid row timestamp", ErrInvalid)
	}
	seconds := float64(drill.TimeWindow.Seconds)
	if drill.TimeWindow.DurationInput != "" {
		seconds, err = strconv.ParseFloat(selected[drill.TimeWindow.DurationInput], 64)
		if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds > 7*86400 {
			return scope, ErrInvalid
		}
	}
	end := at.Add(time.Duration(seconds * float64(time.Second)))
	if at.After(scope.From) {
		scope.From = at
	}
	if end.Before(scope.To) {
		scope.To = end
	}
	if !scope.To.After(scope.From) {
		return scope, ErrInvalid
	}
	return scope, nil
}

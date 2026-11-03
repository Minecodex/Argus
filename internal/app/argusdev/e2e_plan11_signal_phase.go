package argusdev

import (
	"errors"
	"strings"
)

// This explicit phase is the only caller of the extra actual SDK/log fixtures.
// Its model cohort and data admission are declared separately before execution.
func plan11SignalPhase(phase string) bool { return phase == "plan11-signals" }

func plan11ParseSignalTasks(raw, suite, phase string) ([]string, error) {
	if raw == "" {
		return nil, nil
	}
	if suite != "plan8" || !plan11SignalPhase(phase) {
		return nil, errors.New("signal task subset requires plan8/plan11-signals")
	}
	requested := []string{}
	for _, name := range strings.Split(raw, ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, errors.New("empty signal task name")
		}
		requested = append(requested, "plan11-signal-"+strings.TrimPrefix(name, "plan11-signal-"))
	}
	return plan11SignalModelNames(requested)
}

func plan11SignalModelNames(requested []string) ([]string, error) {
	canonical := plan8SelectedModelNames("plan11-signals")
	if len(requested) == 0 {
		return canonical, nil
	}
	selected := map[string]bool{}
	for _, name := range requested {
		if selected[name] {
			return nil, errors.New("duplicate signal task name")
		}
		selected[name] = true
	}
	result := []string{}
	for _, name := range canonical {
		if selected[name] {
			result = append(result, name)
		}
	}
	if len(result) != len(requested) {
		return nil, errors.New("unknown signal task name")
	}
	return result, nil
}

func plan11SelectedSignalTasks(d plan11SignalDataset, requested []string) ([]plan11SignalTask, error) {
	names, err := plan11SignalModelNames(requested)
	if err != nil {
		return nil, err
	}
	selected := map[string]bool{}
	for _, name := range names {
		selected[name] = true
	}
	result := []plan11SignalTask{}
	for _, task := range plan11SignalTasks(d) {
		if selected[task.Name] {
			result = append(result, task)
		}
	}
	if len(result) != len(names) {
		return nil, errors.New("signal task declaration differs from executor")
	}
	return result, nil
}

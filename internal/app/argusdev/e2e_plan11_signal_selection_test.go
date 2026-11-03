package argusdev

import (
	"reflect"
	"testing"
)

func TestSignalTaskSubsetIsExplicitCanonicalAndBounded(t *testing.T) {
	selected, err := plan11ParseSignalTasks("semantics-en,general-zh", "plan8", "plan11-signals")
	expected := []string{"plan11-signal-general-zh", "plan11-signal-semantics-en"}
	if err != nil || !reflect.DeepEqual(selected, expected) {
		t.Fatal(selected, err)
	}
	tasks, err := plan11SelectedSignalTasks(plan11SignalDataset{}, selected)
	if err != nil || len(tasks) != 2 || tasks[0].Name != expected[0] || tasks[1].Name != expected[1] {
		t.Fatal("declared subset differed from actual task dispatch", tasks, err)
	}
	all, err := plan11SelectedSignalTasks(plan11SignalDataset{}, nil)
	if err != nil || len(all) != 9 {
		t.Fatal("default cohort silently changed", err)
	}
	for _, raw := range []string{"unknown", "general-zh,general-zh", "general-zh,", "all"} {
		if _, err := plan11ParseSignalTasks(raw, "plan8", "plan11-signals"); err == nil {
			t.Fatal("invalid subset admitted", raw)
		}
	}
	for _, pair := range [][2]string{{"plan7", "plan11-signals"}, {"plan8", "plan11-metrics"}} {
		if _, err := plan11ParseSignalTasks("general-zh", pair[0], pair[1]); err == nil {
			t.Fatal("subset leaked into another suite/phase")
		}
	}
}

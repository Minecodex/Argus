package argusdev

import (
	"reflect"
	"strings"
	"testing"
)

func TestPlan8SDKGroupsPreserveEveryProducerAndFrozenEnvironment(t *testing.T) {
	base, e := plan8BaseChecks("sdk", t.TempDir())
	if e != nil || len(base) != 1 {
		t.Fatal(e)
	}
	groups, e := plan8BoundedSDKChecks(base)
	if e != nil || len(groups) != 6 {
		t.Fatal("SDK work is not bounded", e)
	}
	expected := map[string]bool{}
	for _, name := range base[0].Tests {
		expected[name] = true
	}
	seen := map[string]bool{}
	for _, group := range groups {
		if len(group.Tests) > 4 || !reflect.DeepEqual(group.Env, base[0].Env) || !reflect.DeepEqual(group.RequiredEnv, base[0].RequiredEnv) {
			t.Fatal("bounded grouping altered the frozen environment or scope")
		}
		args, e := group.executionArgs()
		if e != nil {
			t.Fatal(e)
		}
		pattern := ""
		for i, arg := range args {
			if arg == "-run" {
				pattern = args[i+1]
			}
		}
		for _, name := range group.Tests {
			if seen[name] || !expected[name] || !strings.Contains(pattern, name) {
				t.Fatal("producer omitted/duplicated or omitted from actual command", name)
			}
			seen[name] = true
		}
	}
	if !reflect.DeepEqual(seen, expected) || groups[0].ID != "sdk" || len(groups[0].Tests) != 4 {
		t.Fatal("native protocol/browser artifact producer changed")
	}
	groups[0].Env["ARGUS_MONITORING_NATIVE_PIPELINE"] = "changed-local-copy"
	if reflect.DeepEqual(groups[0].Env, base[0].Env) {
		t.Fatal("one group mutated the source environment")
	}
	base[0].Tests = append(base[0].Tests, "TestUnclassifiedNewSDKScenario")
	if _, e = plan8BoundedSDKChecks(base); e == nil {
		t.Fatal("new unbounded SDK scenario silently omitted")
	}
}

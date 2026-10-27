package argusdev

import (
	"regexp"
	"testing"
)

func TestPlan8ExecutionUsesFinalRequiredTestSet(t *testing.T) {
	check := plan8Check{Program: "go", Args: []string{"test", "-json", "-run", "^(TestInitial)$", "./fixture"}, Tests: []string{"TestInitial"}}
	check.Tests = append(check.Tests, "TestAddedAfterConstruction")
	args, err := check.executionArgs()
	if err != nil {
		t.Fatal(err)
	}
	pattern := regexp.MustCompile(args[3])
	if !pattern.MatchString("TestInitial") || !pattern.MatchString("TestAddedAfterConstruction") || pattern.MatchString("TestUnrelated") {
		t.Fatal("execution diverged from named evidence requirements")
	}
	if check.Args[3] != "^(TestInitial)$" {
		t.Fatal("execution mutated the stored command")
	}
	checks, err := plan8Checks("full", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range checks {
		args, err = item.executionArgs()
		if err != nil {
			t.Fatal(item.ID, err)
		}
		if item.Program != "go" || !(len(args) > 0 && args[0] == "test" || len(args) > 2 && args[0] == "-C" && args[2] == "test") {
			continue
		}
		var selector *regexp.Regexp
		for i, arg := range args {
			if arg == "-run" {
				selector = regexp.MustCompile(args[i+1])
				break
			}
		}
		if selector == nil {
			t.Fatal("named test selector missing", item.ID)
		}
		for _, name := range item.Tests {
			if !selector.MatchString(name) {
				t.Fatal("required root is not executable", item.ID, name)
			}
		}
	}
}

package logcollection

import "testing"

func TestLegacyRulesAndJournaldUnitAliases(t *testing.T) {
	for _, tc := range []struct {
		path    string
		overlap bool
	}{{"/var/log/app.log", true}, {"/var/log/orders/**/*.log", false}, {"/var/log/argus-otelcol.log", false}} {
		actual, err := ConflictsWithLegacy(t.Context(), Config{Include: []string{tc.path}}, "filelog", "linux_amd64", []string{"file-log"})
		if err != nil || actual != tc.overlap {
			t.Fatalf("%s: %v %v", tc.path, actual, err)
		}
	}
	c, err := Normalize("journald", "linux_amd64", []byte(`{"scope":"business","units":["nginx","nginx.service","postgresql@main"]}`))
	if err != nil || len(c.Units) != 2 || c.Units[0] != "nginx.service" || c.Units[1] != "postgresql@main.service" {
		t.Fatalf("unit aliases: %+v %v", c, err)
	}
	if conflict, err := ConflictsWithLegacy(t.Context(), c, "journald", "linux_amd64", []string{"linux-journald"}); err != nil || !conflict {
		t.Fatal("legacy journal overlap not detected")
	}
}

package logcollection

import (
	"github.com/google/uuid"
	"testing"
)

func TestFileClaimsIntersectPatternsAndSubtractExclusions(t *testing.T) {
	for _, tc := range []struct {
		name, a, b         string
		excludeA, excludeB []string
		want               bool
	}{
		{"nested glob", "/var/log/**/*.log", "/var/log/orders/app.log", nil, nil, true},
		{"zero directory glob", "/var/log/**/*.log", "/var/log/app.log", nil, nil, true},
		{"non-recursive", "/var/log/*.log", "/var/log/orders/app.log", nil, nil, false},
		{"different names", "/var/log/orders/*.log", "/var/log/payments/*.log", nil, nil, false},
		{"single rune", "/logs/app?.log", "/logs/app1.*", nil, nil, true},
		{"excluded branch", "/var/log/**/*.log", "/var/log/orders/*.log", []string{"/var/log/orders/**"}, nil, false},
		{"partial exclusion", "/var/log/**/*.log", "/var/log/orders/*.log", []string{"/var/log/orders/a.log"}, nil, true},
		{"both exclude", "/logs/*", "/logs/app.log", nil, []string{"/logs/app.log"}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := FileOverlap(t.Context(), Config{Include: []string{tc.a}, Exclude: tc.excludeA}, Config{Include: []string{tc.b}, Exclude: tc.excludeB})
			if err != nil || got != tc.want {
				t.Fatalf("intersection %v %v", got, err)
			}
		})
	}
}
func TestWorkloadClaimsUseUIDAncestryAndContainer(t *testing.T) {
	ns := uuid.New()
	a := Workload{NamespaceID: ns, NamespaceUID: "ns", UID: "deployment", Containers: []string{"app"}}
	b := Workload{NamespaceID: ns, NamespaceUID: "ns", UID: "pod", Ancestors: []string{"rs", "deployment"}, Containers: []string{"app"}}
	if !WorkloadOverlap(a, b) {
		t.Fatal("pod/deployment overlap lost")
	}
	b.Containers = []string{"sidecar"}
	if WorkloadOverlap(a, b) {
		t.Fatal("distinct container blocked")
	}
	b.Containers = a.Containers
	b.NamespaceUID = "replacement"
	if WorkloadOverlap(a, b) {
		t.Fatal("same namespace name inherited old UID")
	}
}
func TestPathNormalizationIsPlatformAware(t *testing.T) {
	value, err := NormalizePattern(`C:\Logs\App\*.LOG`, true)
	if err != nil || value != "C:/Logs/App/*.LOG" {
		t.Fatal(value, err)
	}
	if overlap, e := FileOverlapForPlatform(t.Context(), Config{Include: []string{"C:/Logs/*.LOG"}}, Config{Include: []string{"c:/logs/test.log"}}, true); e != nil || !overlap {
		t.Fatal("Windows case-insensitive claim missed", e)
	}
	for _, value := range []string{"relative.log", "/logs/../secret", "/logs/[ab].log"} {
		if _, err := NormalizePattern(value, false); err == nil {
			t.Fatal("unsupported or escaping pattern accepted", value)
		}
	}
}

package configbundle

import (
	"github.com/kakj-go/Argus/internal/logcollection"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestHostFileLogGuardExcludesKubernetesPathsBeforeUntrustedParsing(t *testing.T) {
	excluded := protectedLogExclusions([]string{"/var/log/private/**"}, "linux_amd64")
	for _, path := range logcollection.KubernetesContainerExclusions() {
		found := false
		for _, item := range excluded {
			found = found || item == path
		}
		if !found {
			t.Fatal("generic host rule omitted managed Kubernetes path", path)
		}
	}
	operator := protectedLogOperator()
	expr := operator["expr"].(string)
	if !strings.Contains(expr, strconv.Quote(logcollection.HostFileLogPathPattern)) {
		t.Fatal("resolved-path guard lacks shared host/Kubernetes boundary")
	}
	r, e := regexp.Compile(logcollection.HostFileLogPathPattern)
	if e != nil {
		t.Fatal(e)
	}
	for _, path := range []string{"/var/log/pods/ns_pod_uid/main/0.log", "/var/log/containers/pod_ns_main-abc.log", "/etc/argus-otelcol/identity"} {
		if !r.MatchString(path) {
			t.Fatal("resolved reserved origin admitted", path)
		}
	}
	if r.MatchString("/var/log/orders/normal.log") {
		t.Fatal("ordinary host source blocked")
	}
	jsonOperators := jsonFileLogOperators()
	if jsonOperators[0]["expr"] != expr {
		t.Fatal("JSON parsing precedes the canonical-path ownership guard")
	}
}

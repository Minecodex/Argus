package argusdev

import (
	"strings"
	"testing"
)

func TestMatrixMaterialHashesCannotClaimAnotherPlatformOrView(t *testing.T) {
	m := plan8VerificationMaterials{Distribution: "tested-dist", CollectorVersion: "tested-upstream", Collectors: map[string]string{"linux_amd64": strings.Repeat("a", 64)}, Views: map[string]plan8ViewMaterial{"mysql.overview": {Version: 2, SHA256: strings.Repeat("b", 64)}}}
	if err := m.validate("linux_amd64", "tested-dist", "tested-upstream", strings.Repeat("a", 64), "mysql.overview", 2, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	for _, row := range []struct {
		platform, dist, upstream, collector, key string
		version                                  int64
		view                                     string
	}{{"linux_arm64", "tested-dist", "tested-upstream", strings.Repeat("a", 64), "mysql.overview", 2, strings.Repeat("b", 64)}, {"linux_amd64", "other", "tested-upstream", strings.Repeat("a", 64), "mysql.overview", 2, strings.Repeat("b", 64)}, {"linux_amd64", "tested-dist", "tested-upstream", strings.Repeat("c", 64), "mysql.overview", 2, strings.Repeat("b", 64)}, {"linux_amd64", "tested-dist", "tested-upstream", strings.Repeat("a", 64), "mysql.overview", 1, strings.Repeat("b", 64)}, {"linux_amd64", "tested-dist", "tested-upstream", strings.Repeat("a", 64), "unknown", 2, strings.Repeat("b", 64)}} {
		if err := m.validate(row.platform, row.dist, row.upstream, row.collector, row.key, row.version, row.view); err == nil {
			t.Fatal("foreign matrix material accepted", row.platform, row.key)
		}
	}
}

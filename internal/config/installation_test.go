package config

import (
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/installinstruction"
)

func TestInstallationTrustIsSharedByServerWorkerAndIngest(t *testing.T) {
	for _, mode := range []string{"strict", "insecure-first-fetch", "invalid", ""} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("ARGUS_BOOTSTRAP_TLS_MODE", mode)
			t.Setenv("ARGUS_TRUST_BUNDLE_PATH", "/argus-test/ca.pem")
			t.Setenv("ARGUS_TRUST_BUNDLE_EPOCH", "7")
			t.Setenv("ARGUS_HOST_INSTALLER_SHA256", strings.Repeat("a", 64))
			server, ingest := LoadServer(), LoadTelemetry("ingest")
			if mode == "" {
				mode = "strict"
			}
			want := installinstruction.TrustConfig{TrustBundlePath: "/argus-test/ca.pem", TrustBundleEpoch: 7, BootstrapTLSMode: installinstruction.DownloadTLSMode(mode)}
			if server.TrustConfig != want || ingest.TrustConfig != want {
				t.Fatal("installation entry points did not load the same trust policy and installer digest")
			}
			if (server.BootstrapTLSMode.Validate() != nil) != (mode == "invalid") {
				t.Fatal("invalid mode was silently replaced")
			}
		})
	}
}

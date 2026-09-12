package config

import (
	"strconv"

	"github.com/kakj-go/Argus/internal/installinstruction"
)

func loadInstallationTrust() installinstruction.TrustConfig {
	epoch, _ := strconv.ParseInt(valueOrDefault("ARGUS_TRUST_BUNDLE_EPOCH", "1"), 10, 64)
	return installinstruction.TrustConfig{
		TrustBundlePath:  valueOrDefault("ARGUS_TRUST_BUNDLE_PATH", "/var/run/secrets/argus/trust/ca.crt"),
		TrustBundleEpoch: epoch,
		BootstrapTLSMode: installinstruction.DownloadTLSMode(valueOrDefault("ARGUS_BOOTSTRAP_TLS_MODE", "strict")),
	}
}

package installinstruction

import "errors"

// TrustConfig travels as one value from process configuration to every
// installation entry point. The first-fetch policy never changes runtime TLS.
type TrustConfig struct {
	TrustBundlePath  string
	TrustBundleEpoch int64
	BootstrapTLSMode DownloadTLSMode
}

func (mode DownloadTLSMode) Validate() error {
	if mode != DownloadTLSStrict && mode != DownloadTLSInsecureFirstFetch {
		return errors.New("bootstrap download TLS mode must be explicitly strict or insecure-first-fetch")
	}
	return nil
}

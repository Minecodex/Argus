package installation

// CallbackProbePlan freezes only the target-visible callback route. Executor
// internal upstreams remain private process configuration, never wire payloads.
type CallbackProbePlan struct {
	ControlPath         string `json:"control_path"`
	EnrollmentEndpoint  string `json:"enrollment_endpoint"`
	GatewayEndpoint     string `json:"gateway_endpoint"`
	EnrollDialAddress   string `json:"enroll_dial_address,omitempty"`
	GatewayDialAddress  string `json:"gateway_dial_address,omitempty"`
	TrustBundlePEM      []byte `json:"trust_bundle_pem"`
	TrustBundleEpoch    int64  `json:"trust_bundle_epoch"`
	RelayPortGeneration int64  `json:"relay_port_generation,omitempty"`
}

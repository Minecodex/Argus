package telemetry

import "context"

// A Kubernetes installation (or an Edge Gateway receiving its own telemetry)
// can have two valid leaves while its receiver and upstream channel reconnect.
// Admit only a separately registered, still-valid leaf of the exact same
// logical identity; never turn a same-ID payload claim into authorization.
func (server *IngestServer) resolveSelfIdentity(ctx context.Context, gateway TrustedIdentity, serial string) (TrustedIdentity, error) {
	if serial == gateway.CertificateSerial {
		return gateway, nil
	}
	if server.Control == nil {
		return TrustedIdentity{}, errIngestRejected
	}
	previous, uri, err := server.Control.ResolveCollectorIdentity(ctx, serial)
	if err != nil {
		if retryableAdmission(err) {
			return TrustedIdentity{}, err
		}
		return TrustedIdentity{}, errIngestRejected
	}
	if previous.CertificateSerial != serial || previous.CollectorID != gateway.CollectorID ||
		previous.EnterpriseID != gateway.EnterpriseID || previous.ResourceID != gateway.ResourceID ||
		previous.ResourceType != gateway.ResourceType || previous.Role != gateway.Role ||
		previous.AuthorizationVersion != gateway.AuthorizationVersion || uri != CollectorCertificateURI(gateway.CollectorID) {
		return TrustedIdentity{}, errIngestRejected
	}
	return gateway, nil
}

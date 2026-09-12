package artifacthttp

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"net"
)

var (
	ErrSize      = errors.New("artifact size mismatch")
	ErrDigest    = errors.New("artifact SHA-256 mismatch")
	ErrSignature = errors.New("artifact signature is invalid")
)

type StatusError struct{ Status int }

func (e StatusError) Error() string { return "artifact HTTP request failed" }

func FailureCode(err error) string {
	var dns *net.DNSError
	var network *net.OpError
	var cert *tls.CertificateVerificationError
	var authority x509.UnknownAuthorityError
	var hostname x509.HostnameError
	var status StatusError
	switch {
	case errors.As(err, &cert), errors.As(err, &authority), errors.As(err, &hostname):
		return "TLS_INVALID"
	case errors.As(err, &dns):
		return "DNS_FAILED"
	case errors.As(err, &network):
		return "UNREACHABLE"
	case errors.Is(err, ErrSize):
		return "SIZE_MISMATCH"
	case errors.Is(err, ErrDigest):
		return "DIGEST_MISMATCH"
	case errors.Is(err, ErrSignature):
		return "SIGNATURE_INVALID"
	case errors.As(err, &status):
		if status.Status == 404 {
			return "NOT_FOUND"
		}
		return "HTTP_FAILED"
	}
	return ""
}

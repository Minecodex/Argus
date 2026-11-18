package alerting

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
)

// Trust additions belong to one leased notification credential and connection.
// Never modify the process root pool, hostname verification or endpoint policy.
func notificationTLS(base *tls.Config, material string) (*tls.Config, error) {
	invalid := errors.New("invalid notification certificate authority")
	if len(material) == 0 || len(material) > 64<<10 {
		return nil, invalid
	}
	configuration := &tls.Config{MinVersion: tls.VersionTLS12}
	if base != nil {
		configuration = base.Clone()
		if configuration.MinVersion < tls.VersionTLS12 {
			configuration.MinVersion = tls.VersionTLS12
		}
	}
	if configuration.RootCAs != nil {
		configuration.RootCAs = configuration.RootCAs.Clone()
	} else {
		roots, err := x509.SystemCertPool()
		if err != nil || roots == nil {
			roots = x509.NewCertPool()
		}
		configuration.RootCAs = roots
	}
	rest := []byte(material)
	count := 0
	for len(bytes.TrimSpace(rest)) != 0 {
		rest = bytes.TrimSpace(rest)
		if !bytes.HasPrefix(rest, []byte("-----BEGIN CERTIFICATE-----")) {
			return nil, invalid
		}
		block, next := pem.Decode(rest)
		if block == nil || block.Type != "CERTIFICATE" || len(block.Headers) != 0 {
			return nil, invalid
		}
		certificate, err := x509.ParseCertificate(block.Bytes)
		if err != nil || !certificate.BasicConstraintsValid || !certificate.IsCA || certificate.KeyUsage&x509.KeyUsageCertSign == 0 {
			return nil, invalid
		}
		count++
		if count > 16 {
			return nil, invalid
		}
		configuration.RootCAs.AddCert(certificate)
		rest = next
	}
	if count == 0 {
		return nil, invalid
	}
	return configuration, nil
}

package workspaceio

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"os"
)

func TLS(certPath, keyPath, caPath, serverName string, server bool, clientName string) (*tls.Config, error) {
	if server && clientName == "" {
		return nil, errors.New("workspace client identity is required")
	}
	certificate, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	ca, err := os.ReadFile(caPath)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid workspace trust bundle")
	}
	config := &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{certificate}, RootCAs: roots, ServerName: serverName}
	if server {
		config.ClientAuth = tls.RequireAndVerifyClientCert
		config.ClientCAs = roots
		config.VerifyConnection = func(state tls.ConnectionState) error {
			if len(state.VerifiedChains) == 0 || len(state.PeerCertificates) == 0 || state.PeerCertificates[0].Subject.CommonName != clientName {
				return errors.New("workspace client identity rejected")
			}
			return nil
		}
	}
	return config, nil
}

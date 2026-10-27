package argusidentity

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"time"
)

func (extension *identityExtension) deliveryLoop(ctx context.Context) {
	defer close(extension.deliveryDone)
	if extension.config.DeliveryEndpoint == "" {
		return
	}
	state, err := loadDeliveryState(extension.config.DeliveryStateFile, time.Now().UTC())
	if err != nil {
		extension.logger.Error("collector delivery ledger unavailable; data loss visibility is degraded")
		return
	}
	// The loopback reader is mandatory control health. User-facing self-metric
	// collection can be disabled independently and never disables this report.
	ticker := time.NewTicker(extension.config.DeliveryInterval)
	defer ticker.Stop()
	for {
		bounded, cancel := context.WithTimeout(ctx, 12*time.Second)
		err = extension.observeDelivery(bounded, &state)
		cancel()
		if err != nil && ctx.Err() == nil {
			extension.logger.Warn("collector delivery status unavailable; persisted observations will be retried")
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (extension *identityExtension) observeDelivery(ctx context.Context, state *deliveryState) error {
	client := &http.Client{Timeout: 3 * time.Second, Transport: &http.Transport{Proxy: nil, DisableKeepAlives: true}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("native metric redirects are forbidden") }}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:8888/metrics", nil)
	if err != nil {
		return err
	}
	response, err := client.Do(request)
	if err == nil {
		func() {
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				err = errors.New("native delivery metrics unavailable")
				return
			}
			value, readErr := io.ReadAll(io.LimitReader(response.Body, (2<<20)+1))
			if readErr != nil || len(value) > 2<<20 {
				err = errors.New("native metric response exceeds observation budget")
				return
			}
			var snapshot deliverySnapshot
			snapshot, err = readDeliverySnapshot(bytes.NewReader(value))
			if err != nil {
				return
			}
			next := *state
			err = next.observe(snapshot, time.Now().UTC())
			if err != nil {
				return
			}
			err = saveDeliveryState(extension.config.DeliveryStateFile, next)
			if err == nil {
				*state = next
			}
		}()
	}
	// A previously saved loss report is still sent if the current reader is
	// unavailable. A fresh successful sample never clears historical gaps.
	if state.Report.Sequence > 0 {
		if reportErr := extension.reportDelivery(ctx, state.Report); reportErr != nil {
			return reportErr
		}
	}
	return err
}

func (extension *identityExtension) reportDelivery(ctx context.Context, report deliveryReport) error {
	certificate, err := tls.LoadX509KeyPair(extension.config.CertificateFile, extension.config.PrivateKeyFile)
	if err != nil {
		return err
	}
	ca, err := os.ReadFile(extension.config.CABundleFile)
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return errors.New("collector delivery trust bundle is invalid")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS13, RootCAs: pool, Certificates: []tls.Certificate{certificate}}, Proxy: nil}
	defer transport.CloseIdleConnections()
	if extension.config.DialAddress != "" {
		transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, extension.config.DialAddress)
		}
	}
	client := &http.Client{Timeout: 8 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
		return errors.New("collector delivery redirects are forbidden")
	}}
	body, err := json.Marshal(report)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, extension.config.DeliveryEndpoint, bytes.NewReader(body))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
	if response.StatusCode != http.StatusNoContent {
		return deliveryReportError(response.StatusCode)
	}
	return nil
}

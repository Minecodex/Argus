package connector

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"
)

const (
	identityFile = "identity.json"
	keyFile      = "connector-key.pem"
	certFile     = "connector-cert.pem"
	caFile       = "connector-ca.pem"
	resultsFile  = "command-results.json"
	relayFile    = "bastion-relay.json"
)

type relayPortState struct {
	HTTPSPort        uint32 `json:"https_port"`
	GatewayPort      uint32 `json:"gateway_port"`
	Generation       uint64 `json:"generation"`
	AdvertiseAddress string `json:"advertise_address"`
}

type identityState struct {
	ConnectorID          string    `json:"connector_id"`
	Role                 string    `json:"role"`
	InstanceID           string    `json:"instance_id"`
	Name                 string    `json:"name"`
	EnrollmentEndpoint   string    `json:"enrollment_endpoint"`
	GatewayEndpoint      string    `json:"gateway_endpoint"`
	CertificateExpiresAt time.Time `json:"certificate_expires_at"`
	Capabilities         []string  `json:"capabilities"`
	TrustBundleEpoch     int64     `json:"trust_bundle_epoch"`
	TrustBundleSHA256    string    `json:"trust_bundle_sha256"`
	TrustCAFingerprints  []string  `json:"trust_ca_fingerprints"`
}

type commandRecord struct {
	CommandID      string    `json:"command_id"`
	IdempotencyKey string    `json:"idempotency_key"`
	Status         string    `json:"status"`
	ResultTypeURL  string    `json:"result_type_url,omitempty"`
	Result         []byte    `json:"result,omitempty"`
	ResultHash     string    `json:"result_hash,omitempty"`
	ErrorCode      string    `json:"error_code,omitempty"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type localStore struct {
	directory string
	mirror    func(localStore) error
}

func (store localStore) ensure() error {
	if store.directory == "" {
		return errors.New("connector data directory is required")
	}
	if err := os.MkdirAll(store.directory, 0o700); err != nil {
		return err
	}
	return hardenDirectory(store.directory)
}

func (store localStore) loadIdentity() (identityState, error) {
	var value identityState
	encoded, err := os.ReadFile(filepath.Join(store.directory, identityFile))
	if err != nil {
		return value, err
	}
	err = json.Unmarshal(encoded, &value)
	if err != nil || value.ConnectorID == "" || value.InstanceID == "" || value.EnrollmentEndpoint == "" || value.GatewayEndpoint == "" || len(value.Capabilities) == 0 ||
		value.TrustBundleEpoch < 1 || len(value.TrustBundleSHA256) != 64 || len(value.TrustCAFingerprints) == 0 {
		return identityState{}, errors.New("connector identity metadata is invalid")
	}
	return value, nil
}

func (store localStore) saveTrustBundle(value identityState, caBundle []byte) error {
	if err := store.ensure(); err != nil {
		return err
	}
	if len(caBundle) == 0 {
		return errors.New("connector Trust Bundle is empty")
	}
	// Persist trust first. A crash between these atomic writes only causes the
	// next handshake to report the old epoch and request the same Bundle again.
	if err := atomicPrivateWrite(filepath.Join(store.directory, caFile), caBundle); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := atomicPrivateWrite(filepath.Join(store.directory, identityFile), encoded); err != nil {
		return err
	}
	return store.syncMirror()
}

func (store localStore) saveIdentity(value identityState, privateKey, certificate, caBundle []byte) error {
	return store.saveIdentityMaterial(value, privateKey, nil, certificate, caBundle)
}

func (store localStore) saveEnrollmentIdentity(value identityState, privateKey, csr, certificate, caBundle []byte) error {
	if len(csr) == 0 {
		return errors.New("connector enrollment CSR is empty")
	}
	return store.saveIdentityMaterial(value, privateKey, csr, certificate, caBundle)
}

func (store localStore) saveIdentityMaterial(value identityState, privateKey, csr, certificate, caBundle []byte) error {
	if err := store.ensure(); err != nil {
		return err
	}
	previous := identityState{}
	previousEncoded, _ := os.ReadFile(filepath.Join(store.directory, identityFile))
	_ = json.Unmarshal(previousEncoded, &previous)
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	materials := []struct {
		path    string
		content []byte
	}{{keyFile, privateKey}}
	if len(csr) > 0 {
		materials = append(materials, struct {
			path    string
			content []byte
		}{"connector.csr.pem", csr})
	}
	materials = append(materials, struct {
		path    string
		content []byte
	}{certFile, certificate}, struct {
		path    string
		content []byte
	}{caFile, caBundle})
	for _, material := range materials {
		path, content := material.path, material.content
		if len(content) == 0 {
			return errors.New("connector identity material is incomplete")
		}
		if err := atomicPrivateWrite(filepath.Join(store.directory, path), content); err != nil {
			return err
		}
	}
	if previous.ConnectorID != "" && previous.ConnectorID != value.ConnectorID {
		for _, name := range []string{resultsFile, relayFile} {
			if err := os.Remove(filepath.Join(store.directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
	}
	// Metadata is the commit marker and is written only after the complete
	// key, CSR, certificate, and Trust Bundle set has converged.
	if err := atomicPrivateWrite(filepath.Join(store.directory, identityFile), encoded); err != nil {
		return err
	}
	return store.syncMirror()
}

func (store localStore) syncMirror() error {
	if store.mirror == nil {
		return nil
	}
	return store.mirror(store)
}

func (store localStore) identityMaterial() (certificate, privateKey, caBundle []byte, err error) {
	certificate, err = os.ReadFile(filepath.Join(store.directory, certFile))
	if err != nil {
		return nil, nil, nil, err
	}
	privateKey, err = os.ReadFile(filepath.Join(store.directory, keyFile))
	if err != nil {
		return nil, nil, nil, err
	}
	caBundle, err = os.ReadFile(filepath.Join(store.directory, caFile))
	return certificate, privateKey, caBundle, err
}

func (store localStore) loadResults() (map[string]commandRecord, error) {
	encoded, err := os.ReadFile(filepath.Join(store.directory, resultsFile))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]commandRecord{}, nil
	}
	if err != nil {
		return nil, err
	}
	values := map[string]commandRecord{}
	if err := json.Unmarshal(encoded, &values); err != nil {
		return nil, err
	}
	return values, nil
}

func (store localStore) saveResult(value commandRecord) error {
	values, err := store.loadResults()
	if err != nil {
		return err
	}
	value.UpdatedAt = time.Now().UTC()
	if len(value.Result) > 0 {
		digest := sha256.Sum256(value.Result)
		value.ResultHash = hex.EncodeToString(digest[:])
	}
	values[value.CommandID] = value
	cutoff := time.Now().UTC().Add(-24 * time.Hour)
	for key, item := range values {
		if item.UpdatedAt.Before(cutoff) {
			delete(values, key)
		}
	}
	encoded, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	return atomicPrivateWrite(filepath.Join(store.directory, resultsFile), encoded)
}

func (store localStore) loadRelayPortState() (relayPortState, bool, error) {
	var value relayPortState
	encoded, err := os.ReadFile(filepath.Join(store.directory, relayFile))
	if errors.Is(err, os.ErrNotExist) {
		return value, false, nil
	}
	if err != nil {
		return value, false, err
	}
	if err = json.Unmarshal(encoded, &value); err != nil || value.HTTPSPort < 1 || value.HTTPSPort > 65535 ||
		value.GatewayPort < 1 || value.GatewayPort > 65535 || value.HTTPSPort == value.GatewayPort || value.Generation < 1 ||
		!validRelayAdvertiseAddress(value.AdvertiseAddress) {
		return relayPortState{}, false, errors.New("Bastion relay state is invalid")
	}
	return value, true, nil
}

func (store localStore) saveRelayPortState(value relayPortState) error {
	if err := store.ensure(); err != nil {
		return err
	}
	if value.HTTPSPort < 1 || value.HTTPSPort > 65535 || value.GatewayPort < 1 || value.GatewayPort > 65535 ||
		value.HTTPSPort == value.GatewayPort || value.Generation < 1 || !validRelayAdvertiseAddress(value.AdvertiseAddress) {
		return errors.New("Bastion relay state is invalid")
	}
	encoded, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err = atomicPrivateWrite(filepath.Join(store.directory, relayFile), encoded); err != nil {
		return err
	}
	return store.syncMirror()
}

func atomicPrivateWrite(path string, content []byte) error {
	temporary := path + ".tmp"
	_ = os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	remove := true
	defer func() {
		_ = file.Close()
		if remove {
			_ = os.Remove(temporary)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	remove = false
	return hardenFile(path)
}

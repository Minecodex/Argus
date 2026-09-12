package connector

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

type LocalIdentity struct {
	ConnectorID uuid.UUID
	PrivateKey  *ecdsa.PrivateKey
	CSRPEM      string
	Staged      bool
}

func GenerateLocalIdentity(directory string, connectorID uuid.UUID) (LocalIdentity, error) {
	if connectorID == uuid.Nil {
		return LocalIdentity{}, errors.New("connector ID is required")
	}
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return LocalIdentity{}, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return LocalIdentity{}, err
	}
	if foreign, err := hasForeignLocalIdentity(directory, connectorID); err != nil {
		return LocalIdentity{}, err
	} else if foreign {
		identity, err := generateLocalIdentityAt(filepath.Join(directory, ".enrollment", connectorID.String()), connectorID)
		identity.Staged = err == nil
		return identity, err
	}
	return generateLocalIdentityAt(directory, connectorID)
}

func generateLocalIdentityAt(directory string, connectorID uuid.UUID) (LocalIdentity, error) {
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return LocalIdentity{}, err
	}
	if err := os.Chmod(directory, 0o700); err != nil {
		return LocalIdentity{}, err
	}
	keyPath := filepath.Join(directory, "connector-key.pem")
	csrPath := filepath.Join(directory, "connector.csr.pem")
	keyPEM, keyErr := os.ReadFile(keyPath)
	csrPEM, csrErr := os.ReadFile(csrPath)
	if keyErr == nil && csrErr == nil {
		return parseLocalIdentity(connectorID, keyPEM, csrPEM)
	}
	// Kubernetes bootstraps the durable private key from a Secret into a
	// writable emptyDir, but the CSR itself is intentionally not persisted.
	// Re-create that public request from the existing key during PKI repair.
	if keyErr == nil && errors.Is(csrErr, os.ErrNotExist) {
		key, err := parsePrivateKey(keyPEM)
		if err != nil {
			return LocalIdentity{}, err
		}
		csrPEM, err = createCSR(connectorID, key)
		if err != nil {
			return LocalIdentity{}, err
		}
		if err = writePrivateFile(csrPath, csrPEM); err != nil {
			return LocalIdentity{}, err
		}
		return LocalIdentity{ConnectorID: connectorID, PrivateKey: key, CSRPEM: string(csrPEM)}, nil
	}
	if (!errors.Is(keyErr, os.ErrNotExist) && keyErr != nil) || (!errors.Is(csrErr, os.ErrNotExist) && csrErr != nil) ||
		(errors.Is(keyErr, os.ErrNotExist) != errors.Is(csrErr, os.ErrNotExist)) {
		return LocalIdentity{}, errors.New("Connector enrollment identity is incomplete")
	}
	key, csrPEM, err := GenerateCSR(connectorID)
	if err != nil {
		return LocalIdentity{}, err
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return LocalIdentity{}, err
	}
	keyPEM = pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})
	if err := writePrivateFile(keyPath, keyPEM); err != nil {
		return LocalIdentity{}, err
	}
	if err := writePrivateFile(csrPath, csrPEM); err != nil {
		return LocalIdentity{}, err
	}
	return LocalIdentity{ConnectorID: connectorID, PrivateKey: key, CSRPEM: string(csrPEM)}, nil
}

func hasForeignLocalIdentity(directory string, connectorID uuid.UUID) (bool, error) {
	metadata, err := os.ReadFile(filepath.Join(directory, "identity.json"))
	if err == nil {
		var value struct {
			ConnectorID string `json:"connector_id"`
		}
		if json.Unmarshal(metadata, &value) != nil || value.ConnectorID == "" {
			return false, errors.New("Connector identity metadata is invalid")
		}
		current, parseErr := uuid.Parse(value.ConnectorID)
		if parseErr != nil {
			return false, errors.New("Connector identity metadata is invalid")
		}
		return current != connectorID, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	keyPEM, keyErr := os.ReadFile(filepath.Join(directory, "connector-key.pem"))
	csrPEM, csrErr := os.ReadFile(filepath.Join(directory, "connector.csr.pem"))
	if keyErr == nil && csrErr == nil {
		if _, parseErr := parseLocalIdentity(connectorID, keyPEM, csrPEM); parseErr == nil {
			return false, nil
		}
		oldID, parseErr := connectorIDFromCSR(csrPEM)
		if parseErr == nil && oldID != connectorID {
			if _, oldErr := parseLocalIdentity(oldID, keyPEM, csrPEM); oldErr == nil {
				return true, nil
			}
		}
	}
	return false, nil
}

func connectorIDFromCSR(csrPEM []byte) (uuid.UUID, error) {
	block, rest := pem.Decode(csrPEM)
	if block == nil || block.Type != "CERTIFICATE REQUEST" || len(rest) != 0 {
		return uuid.Nil, errors.New("Connector enrollment CSR is invalid")
	}
	csr, err := x509.ParseCertificateRequest(block.Bytes)
	if err != nil || csr.CheckSignature() != nil || len(csr.URIs) != 1 {
		return uuid.Nil, errors.New("Connector enrollment CSR is invalid")
	}
	const prefix = "spiffe://argus.io/connector/"
	if value := csr.URIs[0].String(); strings.HasPrefix(value, prefix) {
		return uuid.Parse(strings.TrimPrefix(value, prefix))
	}
	return uuid.Nil, errors.New("Connector enrollment CSR is invalid")
}

func RemoveStagedLocalIdentity(directory string, connectorID uuid.UUID) error {
	if connectorID == uuid.Nil {
		return errors.New("connector ID is required")
	}
	return os.RemoveAll(filepath.Join(directory, ".enrollment"))
}

func parseLocalIdentity(connectorID uuid.UUID, keyPEM, csrPEM []byte) (LocalIdentity, error) {
	keyBlock, _ := pem.Decode(keyPEM)
	csrBlock, _ := pem.Decode(csrPEM)
	if keyBlock == nil || csrBlock == nil {
		return LocalIdentity{}, errors.New("Connector enrollment identity PEM is invalid")
	}
	keyValue, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return LocalIdentity{}, err
	}
	key, ok := keyValue.(*ecdsa.PrivateKey)
	if !ok {
		return LocalIdentity{}, errors.New("Connector enrollment key must be ECDSA")
	}
	csr, err := x509.ParseCertificateRequest(csrBlock.Bytes)
	if err != nil || csr.CheckSignature() != nil || len(csr.URIs) != 1 || csr.URIs[0].String() != CertificateURI(connectorID) {
		return LocalIdentity{}, errors.New("Connector enrollment CSR is invalid")
	}
	csrKey, err := x509.MarshalPKIXPublicKey(csr.PublicKey)
	if err != nil {
		return LocalIdentity{}, err
	}
	keyPublic, err := x509.MarshalPKIXPublicKey(key.Public())
	if err != nil || string(csrKey) != string(keyPublic) {
		return LocalIdentity{}, errors.New("Connector enrollment CSR does not match its private key")
	}
	return LocalIdentity{ConnectorID: connectorID, PrivateKey: key, CSRPEM: string(csrPEM)}, nil
}

func GenerateCSR(connectorID uuid.UUID) (*ecdsa.PrivateKey, []byte, error) {
	if connectorID == uuid.Nil {
		return nil, nil, errors.New("connector ID is required")
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}
	csrPEM, err := createCSR(connectorID, key)
	if err != nil {
		return nil, nil, err
	}
	return key, csrPEM, nil
}

func createCSR(connectorID uuid.UUID, key *ecdsa.PrivateKey) ([]byte, error) {
	uri, _ := url.Parse(CertificateURI(connectorID))
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, &x509.CertificateRequest{URIs: []*url.URL{uri}}, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), nil
}

func parsePrivateKey(keyPEM []byte) (*ecdsa.PrivateKey, error) {
	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return nil, errors.New("Connector enrollment private key PEM is invalid")
	}
	keyValue, err := x509.ParsePKCS8PrivateKey(keyBlock.Bytes)
	if err != nil {
		return nil, err
	}
	key, ok := keyValue.(*ecdsa.PrivateKey)
	if !ok {
		return nil, errors.New("Connector enrollment key must be ECDSA")
	}
	return key, nil
}

func MarshalPrivateKey(key *ecdsa.PrivateKey) ([]byte, error) {
	value, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: value}), nil
}

func writePrivateFile(path string, value []byte) error {
	temporary := path + ".tmp"
	_ = os.Remove(temporary)
	file, err := os.OpenFile(temporary, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
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
	if _, err := file.Write(value); err != nil {
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
	return nil
}

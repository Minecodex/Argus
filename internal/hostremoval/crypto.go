package hostremoval

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
)

const tokenKeyVersion int32 = 1

type tokenEnvelope struct {
	Token        string `json:"token"`
	ReceiptToken string `json:"receipt_token,omitempty"`
}

func newToken() (string, []byte, error) {
	value := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", nil, err
	}
	plain := base64.RawURLEncoding.EncodeToString(value)
	digest := sha256.Sum256([]byte(plain))
	return plain, digest[:], nil
}

func tokenDigest(value string) []byte {
	digest := sha256.Sum256([]byte(value))
	return digest[:]
}

func sealToken(key []byte, enterpriseID, operationID uuid.UUID, value tokenEnvelope) ([]byte, []byte, error) {
	if len(key) != 32 || value.Token == "" {
		return nil, nil, errors.New("host removal token key or material is invalid")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, err
	}
	plain, err := json.Marshal(value)
	if err != nil {
		return nil, nil, err
	}
	defer clear(plain)
	return nonce, aead.Seal(nil, nonce, plain, tokenAAD(enterpriseID, operationID)), nil
}

func openToken(key, nonce, ciphertext []byte, enterpriseID, operationID uuid.UUID) (tokenEnvelope, error) {
	if len(key) != 32 {
		return tokenEnvelope{}, ErrTokenInvalid
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return tokenEnvelope{}, ErrTokenInvalid
	}
	aead, err := cipher.NewGCM(block)
	if err != nil || len(nonce) != aead.NonceSize() {
		return tokenEnvelope{}, ErrTokenInvalid
	}
	plain, err := aead.Open(nil, nonce, ciphertext, tokenAAD(enterpriseID, operationID))
	if err != nil {
		return tokenEnvelope{}, ErrTokenInvalid
	}
	defer clear(plain)
	var value tokenEnvelope
	if json.Unmarshal(plain, &value) != nil || value.Token == "" {
		return tokenEnvelope{}, ErrTokenInvalid
	}
	return value, nil
}

func tokenAAD(enterpriseID, operationID uuid.UUID) []byte {
	return []byte(fmt.Sprintf("argus.host_removal_token/v1\x00%s\x00%s", enterpriseID, operationID))
}

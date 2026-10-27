package kuberneteshelm

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
)

func operationAAD(r Request) []byte {
	return []byte(SchemaVersion + "\x00" + r.OperationID + "\x00" + r.Namespace.UID + "\x00" + r.APIServer)
}
func sealPlan(r Request, m Material, f frozen) ([]byte, error) {
	if len(m.Key) != 32 {
		return nil, ErrInvalid
	}
	raw, e := json.Marshal(f)
	if e != nil {
		return nil, ErrInvalid
	}
	defer clear(raw)
	if len(raw) > MaxManifestBytes {
		return nil, ErrBudget
	}
	block, e := aes.NewCipher(m.Key)
	if e != nil {
		return nil, ErrInvalid
	}
	a, e := cipher.NewGCM(block)
	if e != nil {
		return nil, ErrInvalid
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return a.Seal(nonce, nonce, raw, operationAAD(r)), nil
}
func openPlan(r Request, m Material) (frozen, error) {
	var f frozen
	if len(m.Key) != 32 || len(r.SealedPlan) > MaxManifestBytes+64 {
		return f, ErrInvalid
	}
	block, e := aes.NewCipher(m.Key)
	if e != nil {
		return f, ErrInvalid
	}
	a, e := cipher.NewGCM(block)
	if e != nil || len(r.SealedPlan) < a.NonceSize()+a.Overhead() {
		return f, ErrInvalid
	}
	raw, e := a.Open(nil, r.SealedPlan[:a.NonceSize()], r.SealedPlan[a.NonceSize():], operationAAD(r))
	if e != nil {
		return f, ErrConflict
	}
	defer clear(raw)
	if json.Unmarshal(raw, &f) != nil || f.SchemaVersion != SchemaVersion {
		return f, ErrInvalid
	}
	expected, actual := r, f.Request
	expected.SealedPlan = nil
	actual.SealedPlan = nil
	x, _ := json.Marshal(expected)
	y, _ := json.Marshal(actual)
	if !hmac.Equal(x, y) {
		return f, ErrConflict
	}
	return f, nil
}
func keyedDigest(key, raw []byte) string {
	m := hmac.New(sha256.New, key)
	m.Write(raw)
	return hex.EncodeToString(m.Sum(nil))
}

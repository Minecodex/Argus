package kuberneteshelm

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"net/url"
	"strings"
)

func sourceAAD(r Request) []byte {
	raw, _ := json.Marshal(r.Source)
	return bytes.Join([][]byte{[]byte("helm-resolved-chart/v2"), operationAAD(r), raw}, []byte{0})
}

func sealDownloadedChart(r Request, key, raw []byte) ([]byte, error) {
	if len(key) != 32 || len(raw) == 0 || len(raw) > MaxArchiveBytes || r.Source == nil {
		return nil, ErrInvalid
	}
	block, e := aes.NewCipher(key)
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
	return a.Seal(nonce, nonce, raw, sourceAAD(r)), nil
}

// ResolvePreviewMaterial authenticates the executor's encrypted archive and
// receipt, so only resolved chart bytes (without source credentials) are kept
// for the approved execution. It never makes a network request.
func ResolvePreviewMaterial(r Request, m Material, result Result) (Material, error) {
	if r.Source == nil {
		if len(result.EncryptedChart) != 0 {
			return Material{}, ErrInvalid
		}
		return m, nil
	}
	if len(m.Key) != 32 || len(result.EncryptedChart) > MaxArchiveBytes+64 || result.Source == nil || result.Source.Kind != r.Source.Kind || result.Source.Origin != r.Source.URL || result.Source.Version != r.Source.Version || !sourceDigestPattern.MatchString(result.Source.Digest) || !sourceDigestPattern.MatchString(result.Source.ArchiveDigest) || r.Source.Digest != "" && r.Source.Digest != result.Source.Digest {
		return Material{}, ErrSourceIntegrity
	}
	transport := "https"
	if r.Source.Kind == "oci" && r.Source.PlainHTTP {
		transport = "http"
	} else if r.Source.Kind == "repository" {
		u, e := url.Parse(r.Source.URL)
		if e != nil {
			return Material{}, ErrSourceIntegrity
		}
		transport = u.Scheme
	}
	if result.Source.Transport != transport || result.Source.AuthURL != r.Source.AuthURL {
		return Material{}, ErrSourceIntegrity
	}
	block, e := aes.NewCipher(m.Key)
	if e != nil {
		return Material{}, ErrInvalid
	}
	a, e := cipher.NewGCM(block)
	if e != nil || len(result.EncryptedChart) < a.NonceSize()+a.Overhead() {
		return Material{}, ErrInvalid
	}
	raw, e := a.Open(nil, result.EncryptedChart[:a.NonceSize()], result.EncryptedChart[a.NonceSize():], sourceAAD(r))
	if e != nil {
		return Material{}, ErrSourceIntegrity
	}
	if archiveDigest(raw) != result.Source.ArchiveDigest {
		clear(raw)
		return Material{}, ErrSourceIntegrity
	}
	ch, e := loadChart(raw)
	name := r.Source.Chart
	if name == "" {
		name = r.Source.URL[strings.LastIndex(r.Source.URL, "/")+1:]
	}
	if e != nil || ch.Metadata.Name != result.Source.Chart || ch.Metadata.Version != result.Source.Version || ch.Metadata.Name != name {
		clear(raw)
		return Material{}, ErrSourceIntegrity
	}
	check := r
	check.SealedPlan = result.SealedPlan
	f, e := openPlan(check, m)
	resolved := Material{Chart: raw, Values: m.Values, Key: m.Key}
	if e != nil || f.Source == nil || *f.Source != *result.Source || f.MaterialDigest != materialDigest(resolved) {
		clear(raw)
		return Material{}, ErrSourceIntegrity
	}
	return resolved, nil
}

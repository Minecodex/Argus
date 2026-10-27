package kuberneteshelm

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"io"

	corev1 "k8s.io/api/core/v1"
)

// Customer-managed release Secrets are untrusted compressed input too. Bound
// decoded history before the Helm storage driver allocates its release graph.
func validateStorageResponse(raw []byte) error {
	var head struct {
		Kind string `json:"kind"`
	}
	if json.Unmarshal(raw, &head) != nil {
		return ErrInvalid
	}
	var secrets []corev1.Secret
	switch head.Kind {
	case "Secret":
		var secret corev1.Secret
		if json.Unmarshal(raw, &secret) != nil {
			return ErrInvalid
		}
		secrets = []corev1.Secret{secret}
	case "SecretList":
		var list corev1.SecretList
		if json.Unmarshal(raw, &list) != nil {
			return ErrInvalid
		}
		secrets = list.Items
	default:
		return nil
	}
	total := 0
	for _, secret := range secrets {
		if secret.Type != "helm.sh/release.v1" && secret.Labels["owner"] != "helm" {
			continue
		}
		data, e := base64.StdEncoding.DecodeString(string(secret.Data["release"]))
		if e != nil {
			return ErrInvalid
		}
		if bytes.HasPrefix(data, []byte{0x1f, 0x8b, 0x08}) {
			reader, e := gzip.NewReader(bytes.NewReader(data))
			if e != nil {
				return ErrInvalid
			}
			count, e := io.Copy(io.Discard, io.LimitReader(reader, (16<<20)+1))
			reader.Close()
			clear(data)
			if e != nil {
				return ErrInvalid
			}
			total += int(count)
		} else {
			total += len(data)
			clear(data)
		}
		if total > 16<<20 {
			return ErrBudget
		}
	}
	return nil
}

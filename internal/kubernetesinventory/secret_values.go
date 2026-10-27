package kubernetesinventory

import (
	"encoding/base64"
	"encoding/json"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	"k8s.io/apimachinery/pkg/util/validation"
	"sort"
)

func SecretValueKeys(raw []byte) ([]string, error) {
	var values map[string]string
	if len(raw) == 0 || len(raw) > 64<<10 || json.Unmarshal(raw, &values) != nil || len(values) == 0 || len(values) > 128 {
		return nil, ErrMutationInvalid
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if len(validation.IsConfigMapKey(key)) != 0 {
			return nil, ErrMutationInvalid
		}
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys, nil
}

// WithSecretValues is called only after an authenticated private-input lease.
// Placeholders stay in plans; base64 material is constructed in execution memory.
func WithSecretValues(input Mutation, raw []byte) (Mutation, error) {
	keys, err := SecretValueKeys(raw)
	if err != nil || input.PrivateInputID == "" || input.Query.Type != "secret" || input.Operation != "create" && input.Operation != "patch" {
		return input, ErrMutationInvalid
	}
	var values map[string]string
	_ = json.Unmarshal(raw, &values)
	var object map[string]any
	if json.Unmarshal(input.Object, &object) != nil {
		return input, ErrMutationInvalid
	}
	data, ok := object["data"].(map[string]any)
	if !ok {
		return input, ErrMutationInvalid
	}
	for _, key := range keys {
		if data[key] != datapolicy.Marker {
			return input, ErrMutationInvalid
		}
		data[key] = base64.StdEncoding.EncodeToString([]byte(values[key]))
		delete(values, key)
	}
	input.Object, err = json.Marshal(object)
	return input, err
}

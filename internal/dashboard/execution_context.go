package dashboard

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry"
)

const contextVersion = "argus.dashboard_execution/v1"
const maxContextBytes = 512 << 10

type executionContext struct {
	Version     string         `json:"version"`
	Enterprise  uuid.UUID      `json:"enterprise"`
	Subject     uuid.UUID      `json:"subject"`
	SubjectType string         `json:"subject_type"`
	ExpiresAt   time.Time      `json:"expires_at"`
	Scope       Execution      `json:"scope"`
	Leaf        *detailContext `json:"leaf,omitempty"`
	Depth       int            `json:"depth"`
}
type detailContext struct {
	PanelID       string                      `json:"panel_id"`
	TargetID      string                      `json:"target_id"`
	Inputs        map[string]string           `json:"inputs"`
	From          time.Time                   `json:"from"`
	To            time.Time                   `json:"to"`
	Resources     []ResourceScope             `json:"resources"`
	Sources       []ResolvedSource            `json:"sources"`
	DetailSources map[string][]ResolvedSource `json:"detail_sources"`
}

func (runtime Runtime) attachExecutionContext(actor Actor, result *Execution) error {
	scope := executionIdentity(*result)
	scope.ContextToken, scope.ContextExpiresAt = "", nil
	scope.VariableCandidates, scope.LocalCandidates = nil, nil
	context := executionContext{Version: contextVersion, Enterprise: actor.EnterpriseID, Subject: actor.SubjectID, SubjectType: actor.SubjectType, ExpiresAt: time.Now().UTC().Add(15 * time.Minute), Scope: scope}
	token, err := runtime.signContext(context)
	if err != nil {
		return err
	}
	result.ContextToken, result.ContextExpiresAt = token, &context.ExpiresAt
	return nil
}

func (runtime Runtime) signContext(value executionContext) (string, error) {
	if len(runtime.ContextKey) < 32 {
		return "", ErrUnavailable
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	if len(encoded) > maxContextBytes {
		return "", telemetry.ErrQueryBudget
	}
	payload := base64.RawURLEncoding.EncodeToString(encoded)
	mac := hmac.New(sha256.New, runtime.ContextKey)
	mac.Write([]byte(contextVersion + "\x00" + payload))
	return payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func (runtime Runtime) readContext(actor Actor, token string) (executionContext, error) {
	var value executionContext
	if len(runtime.ContextKey) < 32 {
		return value, ErrUnavailable
	}
	if len(token) > maxContextBytes*2 {
		return value, ErrInvalid
	}
	payload, signature, ok := strings.Cut(token, ".")
	if !ok {
		return value, ErrDenied
	}
	provided, err := base64.RawURLEncoding.DecodeString(signature)
	if err != nil {
		return value, ErrDenied
	}
	mac := hmac.New(sha256.New, runtime.ContextKey)
	mac.Write([]byte(contextVersion + "\x00" + payload))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return value, ErrDenied
	}
	encoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil || len(encoded) > maxContextBytes || json.Unmarshal(encoded, &value) != nil {
		return value, ErrInvalid
	}
	if value.Version != contextVersion || value.Enterprise != actor.EnterpriseID || value.Subject != actor.SubjectID || value.SubjectType != actor.SubjectType {
		return value, ErrDenied
	}
	if !value.ExpiresAt.After(time.Now()) || value.Depth >= 16 {
		return value, ErrContextExpired
	}
	return value, nil
}

package argusdev

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// A loopback fixture drives the existing OTLP emitter during real browser
// acceptance. It exposes no business API and can emit only the two test states.
func startPlan6BrowserSignals(ctx context.Context, emit func(string) error, fault ...func(context.Context, string, string) error) (string, string, func(), error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", nil, err
	}
	token := uuid.NewString()
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		var body struct {
			Value     string `json:"value"`
			Fault     string `json:"fault"`
			ActionRef string `json:"action_ref"`
		}
		decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1024))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&body) != nil || body.Value != "" && body.Fault != "" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		var operationError error
		if body.Fault != "" && len(fault) == 1 {
			operationError = fault[0](ctx, body.Fault, body.ActionRef)
		} else if body.Value == "95" || body.Value == "60" {
			operationError = emit(body.Value)
		} else {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if ctx.Err() != nil || operationError != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"emitted":true}`))
	})}
	go func() { _ = server.Serve(listener) }()
	return "http://" + listener.Addr().String(), token, func() { _ = server.Close() }, nil
}

func (a *App) plan6BrowserFault(ctx context.Context, env *E2EEnvironment, kind, ref string) error {
	ns, err := env.Kube.Client.CoreV1().Namespaces().Get(ctx, env.SystemNS, metav1.GetOptions{})
	if err != nil || ns.Labels["argus.io/release-id"] != env.ReleaseID {
		return fmt.Errorf("browser faults require owned namespace: %v", err)
	}
	enterprise, account, actor := env.State.Values["enterprise_id"], env.State.Values["p6_evaluator_id"], env.State.Values["admin_user_id"]
	if uuid.Validate(enterprise) != nil || uuid.Validate(account) != nil || uuid.Validate(actor) != nil {
		return fmt.Errorf("browser fixture identity missing")
	}
	var statement string
	switch kind {
	case "evaluator-disable", "evaluator-enable":
		status := "disabled"
		if kind == "evaluator-enable" {
			status = "active"
		}
		statement = fmt.Sprintf("UPDATE service_accounts SET status='%s',authorization_version=authorization_version+1 WHERE id='%s' AND enterprise_id='%s';", status, account, enterprise)
	case "preview-expire":
		if !strings.HasPrefix(ref, "act_") || strings.ContainsAny(ref, "'\\; ") {
			return fmt.Errorf("invalid owned action reference")
		}
		statement = fmt.Sprintf("UPDATE pending_actions SET expires_at=now()-interval '1 minute' WHERE action_ref='%s' AND enterprise_id='%s' AND creator_subject_id='%s' AND action_type LIKE 'alert.%%';", ref, enterprise, actor)
	default:
		return fmt.Errorf("unsupported browser fixture fault")
	}
	_, err = a.postgresQuery(ctx, env, statement)
	return err
}

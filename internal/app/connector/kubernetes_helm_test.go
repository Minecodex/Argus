package connector

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestHelmConnectorPermitsAreSingleWriteBoundAndExpire(t *testing.T) {
	for _, mode := range []string{"allow", "revoke", "stale", "epoch", "command", "operation", "disconnected"} {
		t.Run(mode, func(t *testing.T) {
			out := make(chan outboundFrame, 1)
			p := &helmPermits{epoch: 7, out: out, pending: map[string]chan *connectorv1.KubernetesHelmWriteGrant{}}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			done := make(chan error, 1)
			go func() {
				done <- p.authorize(ctx, &connectorv1.ConnectorCommand{CommandId: "cmd-owned"}, kuberneteshelm.Request{OperationID: "op-owned"}, kuberneteshelm.Write{Version: "v1", Resource: "configmaps", Namespace: "owned", Name: "settings", Verb: "PATCH"})
			}()
			r := (<-out).value.GetKubernetesHelmWriteRequest()
			if len(r.Nonce) != 32 || r.Name != "settings" || r.Resource != "configmaps" {
				t.Fatal("unbound request")
			}
			grant := &connectorv1.KubernetesHelmWriteGrant{CommandId: r.CommandId, ConnectionEpoch: 7, OperationId: r.OperationId, Nonce: r.Nonce, Allowed: true, ExpiresAt: timestamppb.New(time.Now().Add(time.Second))}
			switch mode {
			case "revoke":
				grant.Allowed = false
			case "stale":
				grant.ExpiresAt = timestamppb.New(time.Now().Add(-time.Second))
			case "epoch":
				grant.ConnectionEpoch++
			case "command":
				grant.CommandId = "other"
			case "operation":
				grant.OperationId = "other"
			case "disconnected":
				cancel()
			}
			if mode != "disconnected" {
				p.accept(grant)
			}
			err := <-done
			if (mode == "allow") != (err == nil) {
				t.Fatal(mode, err)
			}
			p.accept(grant)
			p.mu.Lock()
			pending := len(p.pending)
			p.mu.Unlock()
			if pending != 0 {
				t.Fatal("permit can be replayed")
			}
		})
	}
}
func TestHelmConnectorRejectsOldSchemaAndPrivateQueryMaterial(t *testing.T) {
	for _, version := range []string{"", "argus.kubernetes_helm/v0"} {
		payload, _ := anypb.New(&connectorv1.KubernetesHelmCommand{SchemaVersion: version, ClusterId: uuid.NewString()})
		result, e := executeKubernetesHelm(t.Context(), &connectorv1.ConnectorCommand{TypedPayload: payload}, nil)
		if e != nil || result.ErrorCode == "" || len(payload.Value) != 0 && payload.Value[0] != 0 {
			t.Fatal("old protocol or retained private command", e, result.ErrorCode)
		}
	}
}

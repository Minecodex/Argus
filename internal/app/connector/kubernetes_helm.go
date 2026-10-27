package connector

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"sync"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
)

type helmPermitKey struct{}
type helmPermits struct {
	epoch   uint64
	out     chan<- outboundFrame
	mu      sync.Mutex
	pending map[string]chan *connectorv1.KubernetesHelmWriteGrant
}

func (p *helmPermits) accept(grant *connectorv1.KubernetesHelmWriteGrant) {
	p.mu.Lock()
	ch := p.pending[hex.EncodeToString(grant.Nonce)]
	p.mu.Unlock()
	if ch != nil {
		select {
		case ch <- grant:
		default:
		}
	}
}
func (p *helmPermits) authorize(ctx context.Context, command *connectorv1.ConnectorCommand, r kuberneteshelm.Request, w kuberneteshelm.Write) error {
	nonce := make([]byte, 32)
	if _, e := rand.Read(nonce); e != nil {
		return kuberneteshelm.ErrDenied
	}
	key := hex.EncodeToString(nonce)
	ch := make(chan *connectorv1.KubernetesHelmWriteGrant, 1)
	p.mu.Lock()
	p.pending[key] = ch
	p.mu.Unlock()
	defer func() { p.mu.Lock(); delete(p.pending, key); p.mu.Unlock() }()
	call, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	request := &connectorv1.KubernetesHelmWriteRequest{CommandId: command.CommandId, ConnectionEpoch: p.epoch, OperationId: r.OperationID, Nonce: nonce, Group: w.Group, Version: w.Version, Resource: w.Resource, Namespace: w.Namespace, Name: w.Name, Verb: w.Verb}
	select {
	case p.out <- outboundFrame{value: &connectorv1.ConnectRequest{Frame: &connectorv1.ConnectRequest_KubernetesHelmWriteRequest{KubernetesHelmWriteRequest: request}}}:
	case <-call.Done():
		return kuberneteshelm.ErrDenied
	}
	select {
	case grant := <-ch:
		if grant.ConnectionEpoch != p.epoch || grant.CommandId != command.CommandId || grant.OperationId != r.OperationID || !grant.Allowed || grant.ExpiresAt == nil || !grant.ExpiresAt.IsValid() || !time.Now().Before(grant.ExpiresAt.AsTime()) || time.Until(grant.ExpiresAt.AsTime()) > 3*time.Second {
			return kuberneteshelm.ErrDenied
		}
		return nil
	case <-call.Done():
		return kuberneteshelm.ErrDenied
	}
}

func executeKubernetesHelm(ctx context.Context, command *connectorv1.ConnectorCommand, credential []byte) (*connectorv1.KubernetesHelmResult, error) {
	defer clear(command.GetTypedPayload().GetValue())
	out := &connectorv1.KubernetesHelmResult{SchemaVersion: kuberneteshelm.SchemaVersion}
	fail := func(e error) (*connectorv1.KubernetesHelmResult, error) {
		out.ErrorCode = kuberneteshelm.ErrorCode(e)
		return out, nil
	}
	var wire connectorv1.KubernetesHelmCommand
	if command.GetTypedPayload().UnmarshalTo(&wire) != nil {
		return fail(kuberneteshelm.ErrInvalid)
	}
	defer clear(wire.PrivateMaterialJson)
	if wire.SchemaVersion != kuberneteshelm.SchemaVersion || len(wire.RequestJson) > 768<<10 || len(wire.PrivateMaterialJson) > 768<<10 {
		return fail(kuberneteshelm.ErrInvalid)
	}
	var r kuberneteshelm.Request
	if kuberneteshelm.DecodeRequest(wire.RequestJson, &r) != nil || r.OperationID != wire.OperationId || r.SchemaVersion != kuberneteshelm.SchemaVersion {
		return fail(kuberneteshelm.ErrInvalid)
	}
	if _, e := uuid.Parse(wire.ClusterId); e != nil {
		return fail(kuberneteshelm.ErrInvalid)
	}
	config, e := connectorKubeconfig(credential, r.APIServer)
	if e != nil {
		return fail(kuberneteshelm.ErrUnavailable)
	}
	engine := kuberneteshelm.Engine{Config: config}
	if permits, ok := ctx.Value(helmPermitKey{}).(*helmPermits); ok {
		engine.AuthorizeWrite = func(ctx context.Context, r kuberneteshelm.Request, w kuberneteshelm.Write) error {
			return permits.authorize(ctx, command, r, w)
		}
	}
	var value any
	if wire.Mode == "query" {
		if len(wire.PrivateMaterialJson) > 0 || (r.Operation != "list" && r.Operation != "history") {
			return fail(kuberneteshelm.ErrInvalid)
		}
		value, e = engine.Releases(ctx, r)
	} else {
		var material kuberneteshelm.Material
		if json.Unmarshal(wire.PrivateMaterialJson, &material) != nil || len(material.Key) != 32 {
			return fail(kuberneteshelm.ErrInvalid)
		}
		defer material.Clear()
		switch wire.Mode {
		case "preview":
			value, e = engine.Preview(ctx, r, material)
		case "apply":
			value, e = engine.Execute(ctx, r, material)
		case "observe":
			value, e = engine.Observe(ctx, r, material)
		default:
			e = kuberneteshelm.ErrInvalid
		}
	}
	if e != nil {
		return fail(e)
	}
	out.ResultJson, e = json.Marshal(value)
	if e != nil || len(out.ResultJson) > 768<<10 {
		return fail(kuberneteshelm.ErrBudget)
	}
	return out, nil
}

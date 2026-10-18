package directexecutor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/remoteexecution"
	"github.com/kakj-go/Argus/internal/remoteexecution/kubernetesruntime"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/secret"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"k8s.io/client-go/tools/clientcmd"
)

type remoteDirectRuntime struct {
	executor  *Executor
	owner     string
	mu        sync.Mutex
	runtimes  map[string]*kubernetesruntime.Runtime
	stopSlots chan struct{}
}

func (e *Executor) runRemoteExecutionSupervisor(ctx context.Context) {
	for ctx.Err() == nil {
		r := &remoteDirectRuntime{executor: e, owner: e.InstanceID + "/" + uuid.NewString(), runtimes: map[string]*kubernetesruntime.Runtime{}, stopSlots: make(chan struct{}, 2)}
		err := r.run(ctx)
		if err != nil && ctx.Err() == nil {
			slog.Warn("Direct remote runtime stopped; contexts require recovery", "error", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}
func (r *remoteDirectRuntime) run(parent context.Context) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	var jobs sync.WaitGroup
	defer func() {
		cancel()
		r.mu.Lock()
		for _, runtime := range r.runtimes {
			runtime.Close()
		}
		r.mu.Unlock()
		jobs.Wait()
		cleanup, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		_ = r.executor.Store.Queries.StopRemoteDirectWorker(cleanup, r.owner)
	}()
	if err := r.executor.Store.Queries.RegisterRemoteDirectWorker(ctx, r.owner); err != nil {
		return err
	}
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	heartbeat := time.NewTicker(2 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-heartbeat.C:
			if err := r.executor.Store.Queries.RegisterRemoteDirectWorker(ctx, r.owner); err != nil {
				return err
			}
		case <-tick.C:
			if err := r.executor.Store.Queries.RecoverLostRemoteDirectCommands(ctx); err != nil {
				return err
			}
			if err := r.executor.Store.Queries.MarkLostRemoteDirectSessions(ctx); err != nil {
				return err
			}
			for _, control := range []bool{true, false} {
				reserved := false
				if control {
					select {
					case r.stopSlots <- struct{}{}:
						reserved = true
					default:
					}
				} else {
					reserved = r.executor.reserveOne()
				}
				if !reserved {
					continue
				}
				release := func() {
					if control {
						<-r.stopSlots
					} else {
						r.executor.release(1)
					}
				}
				rows, err := r.executor.Store.Queries.ClaimRemoteDirectCommands(ctx, db.ClaimRemoteDirectCommandsParams{OwnerInstance: r.owner, StopsOnly: control, RowLimit: 1})
				if err != nil {
					release()
					return err
				}
				if len(rows) == 0 {
					release()
					continue
				}
				command := rows[0]
				jobs.Add(1)
				go func() { defer jobs.Done(); defer release(); r.execute(ctx, command) }()
			}
		}
	}
}
func remoteMessage(kind string, raw []byte) (proto.Message, *connectorv1.RemoteExecutionBatch, error) {
	var message proto.Message
	var binding *connectorv1.RemoteExecutionBatch
	switch kind {
	case "remote_execution_batch":
		message = &connectorv1.RemoteExecutionBatch{}
	case "remote_execution_control":
		message = &connectorv1.RemoteExecutionControl{}
	case "remote_execution_file":
		message = &connectorv1.RemoteExecutionFile{}
	default:
		return nil, nil, remoteexecution.Error("REMOTE_INPUT_INVALID")
	}
	if protojson.Unmarshal(raw, message) != nil {
		return nil, nil, remoteexecution.Error("REMOTE_BINDING_INVALID")
	}
	switch p := message.(type) {
	case *connectorv1.RemoteExecutionBatch:
		binding = p
	case *connectorv1.RemoteExecutionControl:
		binding = p.Binding
	}
	return message, binding, nil
}
func (r *remoteDirectRuntime) execute(parent context.Context, command db.RemoteExecutionDirectCommand) {
	ctx, cancel := context.WithDeadline(parent, command.ExpiresAt.Time)
	defer cancel()
	canonical, hashErr := resource.CanonicalJSON(command.Payload)
	sum := sha256.Sum256(canonical)
	if hashErr != nil || string(sum[:]) != string(command.PayloadHash) {
		r.finish(command, "failed", nil, "REMOTE_BINDING_INVALID", true)
		return
	}
	payload, binding, err := remoteMessage(command.CommandType, command.Payload)
	if err != nil {
		r.finish(command, "failed", nil, "REMOTE_BINDING_INVALID", true)
		return
	}
	owns := false
	recovery := false
	if control, ok := payload.(*connectorv1.RemoteExecutionControl); ok {
		recovery = control.Operation == "recover_files" || control.Operation == "reclaim"
	}
	err = r.executor.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		n, e := q.BindRemoteDirectOwner(ctx, db.BindRemoteDirectOwnerParams{SessionID: command.SessionID, Generation: command.Generation, DirectOwner: r.owner})
		if e == nil && n == 0 && recovery {
			n, e = q.BindRemoteDirectRecoveryOwner(ctx, db.BindRemoteDirectRecoveryOwnerParams{SessionID: command.SessionID, Generation: command.Generation, DirectOwner: r.owner})
		}
		if e != nil {
			return e
		}
		if n != 1 {
			return nil
		}
		owns = true
		return remoteexecution.ValidateDirectCommand(ctx, q, command, payload)
	})
	if err != nil {
		r.finish(command, "failed", nil, remoteCode(err, "REMOTE_NOT_APPROVED"), true)
		return
	}
	if !owns {
		_ = r.executor.Store.Queries.RequeueUnownedRemoteDirectCommand(ctx, db.RequeueUnownedRemoteDirectCommandParams{ID: command.ID, OwnerInstance: r.owner})
		return
	}
	key := command.SessionID.String() + "/" + command.Generation.String()
	r.mu.Lock()
	runtime := r.runtimes[key]
	r.mu.Unlock()
	if recovery && runtime != nil && runtime.Closed() {
		r.mu.Lock()
		delete(r.runtimes, key)
		r.mu.Unlock()
		runtime = nil
	}
	if runtime == nil {
		control, ok := payload.(*connectorv1.RemoteExecutionControl)
		if !ok || binding == nil || (control.Operation != "open" && !recovery) {
			r.finish(command, "failed", nil, "REMOTE_CONTEXT_LOST", false)
			return
		}
		runtime, err = r.open(ctx, command, binding, recovery)
		if err != nil {
			r.finish(command, "failed", nil, remoteCode(err, "REMOTE_ROUTE_UNAVAILABLE"), false)
			return
		}
		r.mu.Lock()
		r.runtimes[key] = runtime
		r.mu.Unlock()
	}
	result, err := runtime.Call(ctx, command.CommandType, payload)
	if err != nil {
		r.finish(command, "result_unknown", nil, remoteCode(err, "REMOTE_RESULT_UNKNOWN"), false)
		return
	}
	raw, err := protojson.Marshal(result)
	if err != nil {
		r.finish(command, "result_unknown", nil, "REMOTE_COMPLETION_INVALID", false)
		return
	}
	r.finish(command, "succeeded", raw, "", false)
	if control, ok := payload.(*connectorv1.RemoteExecutionControl); ok && control.Operation == "reclaim" {
		runtime.Close()
		r.mu.Lock()
		delete(r.runtimes, key)
		r.mu.Unlock()
	}
}
func (r *remoteDirectRuntime) open(ctx context.Context, command db.RemoteExecutionDirectCommand, binding *connectorv1.RemoteExecutionBatch, filesOnly bool) (*kubernetesruntime.Runtime, error) {
	identity, err := r.executor.Store.Queries.GetRemoteIdentityByID(ctx, db.GetRemoteIdentityByIDParams{ID: command.IdentityID, EnterpriseID: command.EnterpriseID})
	if err != nil || !identity.CredentialID.Valid {
		return nil, remoteexecution.Error("REMOTE_IDENTITY_UNAVAILABLE")
	}
	cluster, err := r.executor.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{ID: identity.ResourceID, EnterpriseID: identity.EnterpriseID})
	if err != nil || cluster.ConnectionMode != "direct" || cluster.ApiServer != binding.KubernetesApiServer {
		return nil, remoteexecution.Error("REMOTE_IDENTITY_CHANGED")
	}
	target, addresses, err := r.executor.Validator.ResolveHTTPS(ctx, cluster.ApiServer)
	if err != nil || len(addresses) == 0 {
		return nil, remoteexecution.Error("REMOTE_ROUTE_UNAVAILABLE")
	}
	lease, err := r.executor.Secrets.IssueLease(secret.WithActorType(ctx, "direct_executor"), r.owner, identity.EnterpriseID, secret.LeaseRequest{CredentialID: identity.CredentialID.UUID, OperationRef: command.CommandID, TargetResourceType: identity.ResourceType, TargetResourceID: identity.ResourceID, RecipientType: "direct_executor", RecipientID: r.owner, Protocol: "kubernetes", TTL: time.Minute})
	if err != nil {
		return nil, err
	}
	defer clear(lease.Value)
	defer func() {
		cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		_ = r.executor.Secrets.ConsumeLease(cleanup, identity.EnterpriseID, lease.Lease.ID)
	}()
	config, err := clientcmd.RESTConfigFromKubeConfig(lease.Value)
	if err != nil || config.Insecure || config.ExecProvider != nil || config.AuthProvider != nil || config.Proxy != nil || config.WrapTransport != nil || config.CAFile != "" || config.CertFile != "" || config.KeyFile != "" || config.BearerTokenFile != "" || config.Impersonate.UserName != "" {
		return nil, remoteexecution.Error("REMOTE_IDENTITY_UNAVAILABLE")
	}
	config.Host = target.String()
	if config.ServerName == "" {
		config.ServerName = target.Hostname()
	}
	port := 443
	if target.Port() != "" {
		port, err = strconv.Atoi(target.Port())
		if err != nil {
			return nil, err
		}
	}
	config.Dial = func(ctx context.Context, _, _ string) (net.Conn, error) {
		if err := r.executor.Validator.Revalidate(ctx, target.Hostname(), addresses); err != nil {
			return nil, err
		}
		return (&net.Dialer{Timeout: 10 * time.Second}).DialContext(ctx, "tcp", net.JoinHostPort(addresses[0].String(), fmt.Sprint(port)))
	}
	namespace := cluster.DefaultNamespace
	if binding.Namespace != "" {
		namespace = binding.Namespace
	}
	if namespace == "" {
		namespace = "default"
	}
	return kubernetesruntime.OpenWithConfig(ctx, binding, config, kubernetesruntime.Options{Namespace: namespace, Image: r.executor.RemoteRuntimeImage, FilesOnly: filesOnly})
}
func (r *remoteDirectRuntime) finish(command db.RemoteExecutionDirectCommand, status string, result json.RawMessage, code string, rejected bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if len(result) == 0 {
		result = []byte(`{}`)
	}
	if _, err := r.executor.Store.Queries.CompleteRemoteDirectCommand(ctx, db.CompleteRemoteDirectCommandParams{ID: command.ID, EnterpriseID: command.EnterpriseID, OwnerInstance: r.owner, Status: status, Result: result, ErrorCode: code, DispatchRejected: rejected}); err != nil {
		slog.Warn("Direct remote receipt could not be persisted", "command_id", command.ID, "error", err)
	}
}
func remoteCode(err error, fallback string) string {
	var coded interface{ Code() string }
	if errors.As(err, &coded) {
		return coded.Code()
	}
	if text := err.Error(); strings.HasPrefix(text, "REMOTE_") && len(text) < 128 && !strings.ContainsAny(text, " \n\r\t:/\\") {
		return text
	}
	return fallback
}

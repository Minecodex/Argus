package kubernetesworkbench

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesstream"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type StreamSession struct {
	ID           uuid.UUID `json:"id"`
	ClusterID    uuid.UUID `json:"cluster_id"`
	NamespaceID  uuid.UUID `json:"namespace_id"`
	Namespace    string    `json:"namespace"`
	NamespaceUID string    `json:"namespace_uid"`
	Pod          string    `json:"pod"`
	PodUID       string    `json:"pod_uid"`
	Container    string    `json:"container"`
	RestartCount int32     `json:"restart_count"`
	Mode         string    `json:"mode"`
	Shell        string    `json:"shell,omitempty"`
	Port         int32     `json:"port,omitempty"`
	Service      string    `json:"service,omitempty"`
	Status       string    `json:"status"`
	Reason       string    `json:"reason,omitempty"`
	Version      int64     `json:"version"`
	ExpiresAt    time.Time `json:"expires_at"`
	BytesIn      int64     `json:"bytes_in"`
	BytesOut     int64     `json:"bytes_out"`
}
type StreamTicket struct {
	Session         StreamSession `json:"session"`
	Ticket          string        `json:"ticket"`
	TicketExpiresAt time.Time     `json:"ticket_expires_at"`
	Protocol        string        `json:"protocol_version"`
	WebsocketURL    string        `json:"websocket_url"`
}

func streamDTO(row db.KubernetesStreamSession, binding StreamBinding) StreamSession {
	p, t := binding.Target.Pin, binding.Target
	result := StreamSession{ID: row.ID, ClusterID: row.ClusterID, NamespaceID: row.NamespaceID, Namespace: p.Namespace, NamespaceUID: p.NamespaceUID, Pod: p.Pod, PodUID: p.PodUID, Container: p.Container, RestartCount: p.RestartCount, Mode: row.Mode, Shell: t.Shell, Port: t.Port, Status: row.Status, Reason: row.EndReason, Version: row.Version, ExpiresAt: row.ExpiresAt.Time, BytesIn: row.BytesIn, BytesOut: row.BytesOut}
	if t.Service != nil {
		result.Service = t.Service.Name
	}
	return result
}
func streamRandomTicket() (string, []byte, error) {
	token := make([]byte, 32)
	if _, err := rand.Read(token); err != nil {
		return "", nil, err
	}
	defer clear(token)
	hash := sha256.Sum256(token)
	return base64.RawURLEncoding.EncodeToString(token), hash[:], nil
}
func streamAudit(ctx context.Context, q *db.Queries, row db.KubernetesStreamSession, event string) error {
	details := map[string]any{"cluster_id": row.ClusterID.String(), "namespace_id": row.NamespaceID.String(), "namespace_uid": row.NamespaceUid, "mode": row.Mode, "status": row.Status, "reason": row.EndReason, "bytes_in": row.BytesIn, "bytes_out": row.BytesOut}
	var binding StreamBinding
	if json.Unmarshal(row.Binding, &binding) == nil {
		pin := binding.Target.Pin
		details["pod"], details["pod_uid"], details["container"], details["restart_count"] = pin.Pod, pin.PodUID, pin.Container, pin.RestartCount
		details["request_reason"] = binding.Reason
		if binding.Target.Mode == "port_forward" {
			details["target_port"] = binding.Target.Port
		}
		if service := binding.Target.Service; service != nil {
			details["service"], details["service_uid"], details["service_port"] = service.Name, service.UID, service.Port
		}
	}
	_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: row.EnterpriseID, Valid: true}, ActorType: "enterprise_user", ActorID: row.ActorID.String(), Action: "kubernetes.stream." + event, ResourceType: "kubernetes_stream_session", ResourceID: row.ID.String(), Result: "success", Details: details})
	return err
}
func (s Service) CreateStreamSession(ctx context.Context, a Actor, httpID, clusterID uuid.UUID, in StreamInput) (StreamTicket, error) {
	if s.Store == nil || a.SubjectType != "user" {
		return StreamTicket{}, authorization.ErrAccessDenied
	}
	if err := authorizeInteractivePermission(ctx, s.Store.Queries, a, httpID, streamPermission(in.Mode)); err != nil {
		return StreamTicket{}, err
	}
	binding, cluster, err := s.resolveStream(ctx, a, clusterID, in)
	if err != nil {
		return StreamTicket{}, err
	}
	ticket, hash, err := streamRandomTicket()
	if err != nil {
		return StreamTicket{}, err
	}
	raw, _ := json.Marshal(binding)
	now := time.Now().UTC()
	var row db.KubernetesStreamSession
	err = s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if err := q.LockKubernetesLogSessionActor(ctx, a.SubjectID); err != nil {
			return err
		}
		count, err := q.CountActiveKubernetesStreamSessions(ctx, db.CountActiveKubernetesStreamSessionsParams{EnterpriseID: a.EnterpriseID, ActorID: a.SubjectID})
		if err != nil {
			return err
		}
		if count >= 10 {
			return kubernetesinventory.ErrBudget
		}
		row, err = q.CreateKubernetesStreamSession(ctx, db.CreateKubernetesStreamSessionParams{ID: uuid.New(), EnterpriseID: a.EnterpriseID, ClusterID: clusterID, NamespaceID: in.NamespaceID, NamespaceUid: binding.Target.Pin.NamespaceUID, ActorID: a.SubjectID, HttpSessionID: httpID, AuthorizationVersion: a.AuthorizationVersion, ClusterVersion: cluster.ResourceVersion, Mode: in.Mode, Binding: raw, TicketHash: hash, TicketExpiresAt: pgtype.Timestamptz{Time: now.Add(30 * time.Second), Valid: true}, ExpiresAt: pgtype.Timestamptz{Time: now.Add(15 * time.Minute), Valid: true}, ConnectorID: uuid.NullUUID{UUID: binding.Route.ConnectorID, Valid: binding.Route.ConnectorID != uuid.Nil}, ConnectorEpoch: pgtype.Int8{Int64: binding.Route.Epoch, Valid: binding.Route.Epoch > 0}})
		if err != nil {
			return err
		}
		if _, err = authorizeStreamRow(ctx, q, row); err != nil {
			return err
		}
		return streamAudit(ctx, q, row, "created")
	})
	if err != nil {
		return StreamTicket{}, err
	}
	return StreamTicket{Session: streamDTO(row, binding), Ticket: ticket, TicketExpiresAt: row.TicketExpiresAt.Time, Protocol: kubernetesstream.WebSocketProtocol}, nil
}
func (s Service) ownedStream(ctx context.Context, a Actor, httpID, cluster, id uuid.UUID) (StreamConnection, error) {
	row, err := s.Store.Queries.GetKubernetesStreamSession(ctx, id)
	if err != nil || row.EnterpriseID != a.EnterpriseID || row.ActorID != a.SubjectID || row.HttpSessionID != httpID || row.ClusterID != cluster {
		return StreamConnection{}, authorization.ErrAccessDenied
	}
	live := row.Status == "pending" || row.Status == "connecting" || row.Status == "active"
	return authorizeStream(ctx, s.Store.Queries, row, live)
}
func (s Service) GetStreamSession(ctx context.Context, a Actor, httpID, cluster, id uuid.UUID) (StreamSession, error) {
	value, err := s.ownedStream(ctx, a, httpID, cluster, id)
	if err != nil {
		return StreamSession{}, err
	}
	return streamDTO(value.Row, value.Binding), nil
}
func (s Service) IssueStreamTicket(ctx context.Context, a Actor, httpID, cluster, id uuid.UUID) (StreamTicket, error) {
	value, err := s.ownedStream(ctx, a, httpID, cluster, id)
	if err != nil {
		return StreamTicket{}, err
	}
	if value.Row.Status != "pending" {
		return StreamTicket{}, kubernetesinventory.ErrMutationConflict
	}
	ticket, hash, err := streamRandomTicket()
	if err != nil {
		return StreamTicket{}, err
	}
	var row db.KubernetesStreamSession
	err = s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if _, e := authorizeStreamRow(ctx, q, value.Row); e != nil {
			return e
		}
		var e error
		row, e = q.ReplaceKubernetesStreamTicket(ctx, db.ReplaceKubernetesStreamTicketParams{EnterpriseID: a.EnterpriseID, ID: id, TicketHash: hash, TicketExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(30 * time.Second), Valid: true}})
		if e != nil {
			return e
		}
		return streamAudit(ctx, q, row, "ticket_issued")
	})
	if err != nil {
		return StreamTicket{}, err
	}
	return StreamTicket{Session: streamDTO(row, value.Binding), Ticket: ticket, TicketExpiresAt: row.TicketExpiresAt.Time, Protocol: kubernetesstream.WebSocketProtocol}, nil
}
func (s Service) CloseStreamSession(ctx context.Context, a Actor, httpID, cluster, id uuid.UUID) (StreamSession, error) {
	value, err := s.ownedStream(ctx, a, httpID, cluster, id)
	if err != nil {
		return StreamSession{}, err
	}
	if value.Row.Status != "pending" && value.Row.Status != "connecting" && value.Row.Status != "active" {
		return streamDTO(value.Row, value.Binding), nil
	}
	if value.Row.Status == "pending" {
		err = s.FinishStream(ctx, id, value.Row.Fence, "closed", "client_close", 0, 0)
	} else {
		_, err = s.Store.Queries.RequestCloseKubernetesStreamSession(ctx, db.RequestCloseKubernetesStreamSessionParams{EnterpriseID: a.EnterpriseID, ID: id})
	}
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return StreamSession{}, err
	}
	value.Row, err = s.Store.Queries.GetKubernetesStreamSession(ctx, id)
	if err != nil {
		return StreamSession{}, err
	}
	return streamDTO(value.Row, value.Binding), nil
}
func (s Service) FinishStream(ctx context.Context, id uuid.UUID, fence int64, status, reason string, in, out int64) error {
	if status != "closed" && status != "invalidated" && status != "connection_lost" || len(reason) > 100 || in < 0 || out < 0 {
		return kubernetesinventory.ErrInvalid
	}
	return s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		row, err := q.EndKubernetesStreamSession(ctx, db.EndKubernetesStreamSessionParams{ID: id, Fence: fence, Status: status, EndReason: reason, BytesIn: in, BytesOut: out})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		if row.CredentialLeaseID.Valid {
			_, err = q.ConsumeCredentialLease(ctx, db.ConsumeCredentialLeaseParams{EnterpriseID: row.EnterpriseID, ID: row.CredentialLeaseID.UUID})
			if err != nil {
				return err
			}
		}
		return streamAudit(ctx, q, row, "ended")
	})
}

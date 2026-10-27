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
	"github.com/kakj-go/Argus/internal/kubernetespodlogs"
	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	corev1 "k8s.io/api/core/v1"
)

type LogSessionInput struct {
	NamespaceID  uuid.UUID `json:"namespace_id"`
	Pod          string    `json:"pod"`
	PodUID       string    `json:"pod_uid"`
	Container    string    `json:"container"`
	Previous     bool      `json:"previous"`
	TailLines    int       `json:"tail_lines"`
	SinceSeconds int       `json:"since_seconds"`
}
type LogSession struct {
	ID           uuid.UUID `json:"id"`
	ClusterID    uuid.UUID `json:"cluster_id"`
	NamespaceID  uuid.UUID `json:"namespace_id"`
	Namespace    string    `json:"namespace"`
	NamespaceUID string    `json:"namespace_uid"`
	Pod          string    `json:"pod"`
	PodUID       string    `json:"pod_uid"`
	Container    string    `json:"container"`
	RestartCount int32     `json:"restart_count"`
	Previous     bool      `json:"previous"`
	Status       string    `json:"status"`
	Version      int64     `json:"version"`
	ExpiresAt    time.Time `json:"expires_at"`
}
type LogTicket struct {
	Session         LogSession `json:"session"`
	Ticket          string     `json:"ticket"`
	TicketExpiresAt time.Time  `json:"ticket_expires_at"`
}
type LogBatch struct {
	Content     string    `json:"content"`
	NextCursor  string    `json:"next_cursor"`
	ObservedAt  time.Time `json:"observed_at"`
	Truncated   bool      `json:"truncated"`
	PendingLine bool      `json:"pending_line"`
	Redacted    bool      `json:"redacted"`
	Complete    bool      `json:"complete"`
}
type logBackend interface {
	ReadPinnedPodLogs(context.Context, db.KubernetesCluster, uuid.UUID, kubernetespodlogs.Query) (kubernetespodlogs.Batch, error)
}

func (s Service) GetLogSession(ctx context.Context, a Actor, httpID, cluster, id uuid.UUID) (LogSession, error) {
	row, query, err := s.logSession(ctx, a, httpID, cluster, id, true)
	if err != nil {
		return LogSession{}, err
	}
	return logSessionDTO(row, query), nil
}

func (s Service) CreateLogSession(ctx context.Context, a Actor, httpID, clusterID uuid.UUID, in LogSessionInput) (LogTicket, error) {
	if s.Store == nil || len(s.CursorKey) != 32 {
		return LogTicket{}, kubernetesinventory.ErrUnavailable
	}
	if a.SubjectType != "user" || httpID == uuid.Nil || in.SinceSeconds < 1 || in.SinceSeconds > 86400 || in.TailLines < 1 || in.TailLines > 1000 {
		return LogTicket{}, kubernetesinventory.ErrInvalid
	}
	if _, ok := s.Backend.(logBackend); !ok {
		return LogTicket{}, kubernetesinventory.ErrUnsupported
	}
	if err := authorizeLogPermission(ctx, s.Store.Queries, a, httpID); err != nil {
		return LogTicket{}, err
	}
	baseline, _, err := s.scope(ctx, a, clusterID)
	if err != nil {
		return LogTicket{}, err
	}
	object, err := s.Get(ctx, a, clusterID, ObjectInput{Type: "pod", NamespaceID: in.NamespaceID, Name: in.Pod, UID: in.PodUID})
	if err != nil {
		return LogTicket{}, err
	}
	var pod corev1.Pod
	if json.Unmarshal(object.Data, &pod) != nil {
		return LogTicket{}, kubernetesinventory.ErrInvalid
	}
	pin, err := kubernetespodlogs.PinPod(object.NamespaceUID, &pod, in.Container, in.Previous)
	if err != nil {
		return LogTicket{}, err
	}
	query := kubernetespodlogs.Query{Pin: pin, Since: time.Now().UTC().Truncate(time.Second).Add(-time.Duration(in.SinceSeconds) * time.Second), TailLines: in.TailLines}
	query.APIServer = baseline.ApiServer
	query.ContinuityKey = make([]byte, 32)
	if _, err = rand.Read(query.ContinuityKey); err != nil {
		return LogTicket{}, err
	}
	if err = query.Validate(); err != nil {
		return LogTicket{}, err
	}
	binding, _ := json.Marshal(query)
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return LogTicket{}, err
	}
	defer clear(token)
	hash := sha256.Sum256(token)
	now := time.Now().UTC()
	var row db.KubernetesLogSession
	// Capacity is serialized by the user row; read committed observes the
	// preceding creator after waiting for that lock. Kubernetes I/O stays outside.
	err = s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if err := q.LockKubernetesLogSessionActor(ctx, a.SubjectID); err != nil {
			return err
		}
		if err := authorizeLogPermission(ctx, q, a, httpID); err != nil {
			return err
		}
		count, err := q.CountActiveKubernetesLogSessions(ctx, db.CountActiveKubernetesLogSessionsParams{EnterpriseID: a.EnterpriseID, ActorID: a.SubjectID})
		if err != nil {
			return err
		}
		if count >= 10 {
			return kubernetesinventory.ErrBudget
		}
		cluster, err := q.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: a.EnterpriseID, ID: clusterID})
		if err != nil {
			return err
		}
		if cluster.ResourceVersion != baseline.ResourceVersion || cluster.Status != "active" {
			return kubernetesinventory.ErrIdentityChanged
		}
		row, err = q.CreateKubernetesLogSession(ctx, db.CreateKubernetesLogSessionParams{ID: uuid.New(), EnterpriseID: a.EnterpriseID, ClusterID: clusterID, NamespaceID: in.NamespaceID, NamespaceUid: pin.NamespaceUID, ActorID: a.SubjectID, HttpSessionID: httpID, AuthorizationVersion: a.AuthorizationVersion, ClusterVersion: cluster.ResourceVersion, Binding: binding, TicketHash: hash[:], TicketExpiresAt: pgtype.Timestamptz{Time: now.Add(30 * time.Second), Valid: true}, ExpiresAt: pgtype.Timestamptz{Time: now.Add(15 * time.Minute), Valid: true}})
		if err != nil {
			return err
		}
		if _, _, err = authorizeLogRow(ctx, q, a, httpID, row, false); err != nil {
			return err
		}
		return logAudit(ctx, q, row, "created", nil)
	})
	if err != nil {
		return LogTicket{}, err
	}
	return LogTicket{Session: logSessionDTO(row, query), Ticket: base64.RawURLEncoding.EncodeToString(token), TicketExpiresAt: row.TicketExpiresAt.Time}, nil
}

func (s Service) AttachLogSession(ctx context.Context, a Actor, httpID, cluster, id uuid.UUID, ticket string) (LogSession, error) {
	token, err := base64.RawURLEncoding.DecodeString(ticket)
	if err != nil || len(token) != 32 {
		return LogSession{}, kubernetesinventory.ErrInvalid
	}
	defer clear(token)
	hash := sha256.Sum256(token)
	row, query, err := s.logSession(ctx, a, httpID, cluster, id, false)
	if err != nil {
		return LogSession{}, err
	}
	if row.Status != "pending" || !time.Now().Before(row.TicketExpiresAt.Time) {
		return LogSession{}, kubernetesinventory.ErrExpired
	}
	err = s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		if _, _, err := authorizeLogRow(ctx, q, a, httpID, row, false); err != nil {
			return err
		}
		var e error
		row, e = q.AttachKubernetesLogSession(ctx, db.AttachKubernetesLogSessionParams{EnterpriseID: a.EnterpriseID, ID: id, ActorID: a.SubjectID, HttpSessionID: httpID, TicketHash: hash[:]})
		if errors.Is(e, pgx.ErrNoRows) {
			return kubernetesinventory.ErrMutationConflict
		}
		if e != nil {
			return e
		}
		return logAudit(ctx, q, row, "attached", nil)
	})
	if err != nil {
		return LogSession{}, err
	}
	return logSessionDTO(row, query), nil
}

func (s Service) CloseLogSession(ctx context.Context, a Actor, httpID, cluster, id uuid.UUID) (LogSession, error) {
	row, err := s.Store.Queries.GetKubernetesLogSession(ctx, db.GetKubernetesLogSessionParams{EnterpriseID: a.EnterpriseID, ID: id})
	if err != nil || row.ClusterID != cluster || row.ActorID != a.SubjectID || row.HttpSessionID != httpID {
		return LogSession{}, authorization.ErrAccessDenied
	}
	if err = authorizeLogPermission(ctx, s.Store.Queries, a, httpID); err != nil {
		return LogSession{}, err
	}
	var query kubernetespodlogs.Query
	if json.Unmarshal(row.Binding, &query) != nil {
		return LogSession{}, kubernetesinventory.ErrProtocol
	}
	err = s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		var e error
		row, e = q.CloseKubernetesLogSession(ctx, db.CloseKubernetesLogSessionParams{EnterpriseID: a.EnterpriseID, ID: id, ActorID: a.SubjectID, HttpSessionID: httpID})
		if e != nil {
			return e
		}
		return logAudit(ctx, q, row, "closed", nil)
	})
	if err != nil {
		return LogSession{}, err
	}
	return logSessionDTO(row, query), nil
}

func (s Service) ReadLogSession(ctx context.Context, a Actor, httpID, clusterID, id uuid.UUID, cursor string) (LogBatch, error) {
	row, query, err := s.logSession(ctx, a, httpID, clusterID, id, true)
	if err != nil {
		return LogBatch{}, err
	}
	backend, ok := s.Backend.(logBackend)
	if !ok {
		return LogBatch{}, kubernetesinventory.ErrUnsupported
	}
	binding := pagination.Binding{Audience: "kubernetes_log_session/v1", EnterpriseID: a.EnterpriseID.String(), SubjectType: a.SubjectType, SubjectID: a.SubjectID.String(), AuthorizationVersion: a.AuthorizationVersion, FilterHash: pagination.HashFilter([]any{id, httpID, row.ClusterVersion, query}), Sort: "log-boundary-second"}
	signer := pagination.Signer{Key: s.CursorKey}
	if cursor != "" {
		if len(cursor) > 4096 {
			return LogBatch{}, kubernetesinventory.ErrInvalid
		}
		position, e := signer.Decode(cursor, binding)
		if e != nil {
			return LogBatch{}, e
		}
		var mark kubernetespodlogs.Watermark
		if json.Unmarshal([]byte(position.ID), &mark) != nil || !mark.Second.Equal(position.Time) {
			return LogBatch{}, kubernetesinventory.ErrInvalid
		}
		query.Resume = &mark
	}
	if err = query.Validate(); err != nil {
		return LogBatch{}, err
	}
	cluster, err := s.Store.Queries.GetKubernetesCluster(ctx, db.GetKubernetesClusterParams{EnterpriseID: a.EnterpriseID, ID: clusterID})
	if err != nil {
		return LogBatch{}, err
	}
	data, err := backend.ReadPinnedPodLogs(ctx, cluster, id, query)
	if err != nil {
		if errors.Is(err, kubernetespodlogs.ErrGap) || errors.Is(err, kubernetesinventory.ErrIdentityChanged) {
			s.invalidateLogSession(ctx, row)
		}
		return LogBatch{}, err
	}
	if _, _, err = s.logSession(ctx, a, httpID, clusterID, id, true); err != nil {
		return LogBatch{}, err
	}
	if data.ObservedAt.IsZero() || len(data.Content) > kubernetespodlogs.MaxBytes*2 {
		return LogBatch{}, kubernetesinventory.ErrProtocol
	}
	validated := query
	validated.Resume = &data.Next
	if validated.Validate() != nil {
		return LogBatch{}, kubernetesinventory.ErrProtocol
	}
	projected, changed, err := datapolicy.Project(data.Content)
	if err != nil {
		return LogBatch{}, err
	}
	mark, _ := json.Marshal(data.Next)
	next, err := signer.Encode(binding, pagination.Position{Time: data.Next.Second, ID: string(mark)})
	if err != nil {
		return LogBatch{}, err
	}
	if err = logAudit(ctx, s.Store.Queries, row, "read", map[string]any{"bytes": len(data.Content), "truncated": data.Truncated, "redacted": changed}); err != nil {
		return LogBatch{}, err
	}
	return LogBatch{Content: projected.(string), NextCursor: next, ObservedAt: data.ObservedAt, Truncated: data.Truncated, PendingLine: data.PendingLine, Redacted: changed || data.Redacted, Complete: query.Pin.Previous || data.Truncated}, nil
}

func logSessionDTO(row db.KubernetesLogSession, query kubernetespodlogs.Query) LogSession {
	p := query.Pin
	return LogSession{ID: row.ID, ClusterID: row.ClusterID, NamespaceID: row.NamespaceID, Namespace: p.Namespace, NamespaceUID: p.NamespaceUID, Pod: p.Pod, PodUID: p.PodUID, Container: p.Container, RestartCount: p.RestartCount, Previous: p.Previous, Status: row.Status, Version: row.Version, ExpiresAt: row.ExpiresAt.Time}
}
func logAudit(ctx context.Context, q *db.Queries, row db.KubernetesLogSession, event string, details map[string]any) error {
	if details == nil {
		details = map[string]any{}
	}
	details["cluster_id"] = row.ClusterID.String()
	details["namespace_id"] = row.NamespaceID.String()
	details["version"] = row.Version
	_, err := audit.Append(ctx, q, audit.Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: row.EnterpriseID, Valid: true}, ActorType: "enterprise_user", ActorID: row.ActorID.String(), Action: "kubernetes.log_session." + event, ResourceType: "kubernetes_log_session", ResourceID: row.ID.String(), Result: "success", Details: details})
	return err
}

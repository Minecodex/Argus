package telemetry

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const CollectorDeliveryQueue = "alert.collector_delivery"

var ErrDeliveryReport = errors.New("COLLECTOR_DELIVERY_REPORT_INVALID")

type DeliveryCounts struct {
	LogRecords   int64 `json:"log_records"`
	Spans        int64 `json:"spans"`
	MetricPoints int64 `json:"metric_points"`
}

func (c DeliveryCounts) valid() bool { return c.LogRecords >= 0 && c.Spans >= 0 && c.MetricPoints >= 0 }
func (c DeliveryCounts) AtLeast(old DeliveryCounts) bool {
	return c.LogRecords >= old.LogRecords && c.Spans >= old.Spans && c.MetricPoints >= old.MetricPoints
}
func (c DeliveryCounts) Any() bool { return c.LogRecords > 0 || c.Spans > 0 || c.MetricPoints > 0 }

// This is a versioned control report. It contains no observed log/Trace body,
// application identity, target configuration, credential or error message.
type CollectorDeliveryReport struct {
	SchemaVersion     string         `json:"schema_version"`
	InstanceID        string         `json:"instance_id"`
	Sequence          int64          `json:"sequence"`
	ObservedSince     time.Time      `json:"observed_since"`
	ObservedAt        time.Time      `json:"observed_at"`
	LastFailureAt     *time.Time     `json:"last_failure_at,omitempty"`
	TerminalFailures  DeliveryCounts `json:"terminal_failures"`
	ReceiverRefusals  DeliveryCounts `json:"receiver_refusals"`
	EnqueueFailures   DeliveryCounts `json:"enqueue_failures"`
	RetryLimitSeconds int64          `json:"retry_limit_seconds"`
	Attribution       string         `json:"attribution"`
}

func (r CollectorDeliveryReport) Validate(now time.Time) error {
	id, err := uuid.Parse(r.InstanceID)
	if err != nil || id == uuid.Nil || r.SchemaVersion != "argus.collector_delivery/v1" || r.Sequence < 1 || r.RetryLimitSeconds != 300 || r.Attribution != "collector_shared_pipeline_exact_targets_unknown" || r.ObservedSince.IsZero() || r.ObservedAt.Before(r.ObservedSince) || r.ObservedAt.After(now.Add(time.Minute)) || !r.TerminalFailures.valid() || !r.ReceiverRefusals.valid() || !r.EnqueueFailures.valid() {
		return ErrDeliveryReport
	}
	failed := r.TerminalFailures.Any() || r.ReceiverRefusals.Any() || r.EnqueueFailures.Any()
	if failed != (r.LastFailureAt != nil) {
		return ErrDeliveryReport
	}
	if r.LastFailureAt != nil && (r.LastFailureAt.Before(r.ObservedSince) || r.LastFailureAt.After(r.ObservedAt)) {
		return ErrDeliveryReport
	}
	return nil
}
func (r CollectorDeliveryReport) AtLeast(old CollectorDeliveryReport) bool {
	return r.InstanceID == old.InstanceID && r.ObservedSince.Equal(old.ObservedSince) && !r.ObservedAt.Before(old.ObservedAt) && r.TerminalFailures.AtLeast(old.TerminalFailures) && r.ReceiverRefusals.AtLeast(old.ReceiverRefusals) && r.EnqueueFailures.AtLeast(old.EnqueueFailures)
}
func (r CollectorDeliveryReport) HasNewFailure(old CollectorDeliveryReport) bool {
	return r.TerminalFailures != old.TerminalFailures || r.ReceiverRefusals != old.ReceiverRefusals || r.EnqueueFailures != old.EnqueueFailures
}

type CollectorDeliveryService struct{ Store *postgres.Store }

func (s CollectorDeliveryService) ObserveDeliveryReport(ctx context.Context, identity TrustedIdentity, report CollectorDeliveryReport) error {
	if s.Store == nil {
		return ErrUnavailable
	}
	if err := report.Validate(time.Now().UTC()); err != nil {
		return err
	}
	return s.Store.InReadCommittedTx(ctx, func(q *db.Queries) error {
		// The owner lock serializes admission and the bounded reporter count.
		if _, err := q.LockCollectorDeliveryOwner(ctx, db.LockCollectorDeliveryOwnerParams{EnterpriseID: identity.EnterpriseID, ID: identity.CollectorID}); err != nil {
			return err
		}
		instance, _ := uuid.Parse(report.InstanceID)
		id := uuid.NewSHA1(identity.CollectorID, []byte("delivery:"+instance.String()))
		row, err := q.EnsureCollectorDeliveryReport(ctx, db.EnsureCollectorDeliveryReportParams{ID: id, EnterpriseID: identity.EnterpriseID, CollectorID: identity.CollectorID, InstanceID: instance})
		if err != nil {
			return err
		}
		var before CollectorDeliveryReport
		if row.Sequence > 0 {
			if json.Unmarshal(row.Report, &before) != nil {
				return ErrDeliveryReport
			}
			if report.Sequence < row.Sequence {
				return nil
			}
			if report.Sequence == row.Sequence {
				a, _ := json.Marshal(report)
				b, _ := json.Marshal(before)
				if string(a) != string(b) {
					return ErrDeliveryReport
				}
				return nil
			}
			if !report.AtLeast(before) {
				return ErrDeliveryReport
			}
		}
		raw, err := json.Marshal(report)
		if err != nil {
			return err
		}
		// The report and pending alert are one durable Outbox write. Ingest
		// needs no read or write privilege on private Agent runtime payloads.
		return q.UpdateCollectorDeliveryReport(ctx, db.UpdateCollectorDeliveryReportParams{EnterpriseID: identity.EnterpriseID, ID: id, Sequence: report.Sequence, Report: raw, PendingAlert: report.HasNewFailure(before)})
	})
}

type collectorDeliveryObserver interface {
	ObserveDeliveryReport(context.Context, TrustedIdentity, CollectorDeliveryReport) error
}

func (server *IngestServer) httpCollectorDelivery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.TLS == nil || len(r.TLS.PeerCertificates) != 1 || r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
		http.Error(w, "collector identity required", http.StatusUnauthorized)
		return
	}
	identity, err := server.resolveIdentity(r.Context(), r.TLS.PeerCertificates[0])
	if err != nil {
		admissionHTTPError(w, err, "collector identity fenced", http.StatusUnauthorized)
		return
	}
	if server.DeliveryReports == nil {
		http.Error(w, "collector delivery status unavailable", http.StatusServiceUnavailable)
		return
	}
	var report CollectorDeliveryReport
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&report) != nil || report.Validate(time.Now().UTC()) != nil {
		http.Error(w, "collector delivery status invalid", http.StatusBadRequest)
		return
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF {
		http.Error(w, "collector delivery status invalid", http.StatusBadRequest)
		return
	}
	if err = server.DeliveryReports.ObserveDeliveryReport(r.Context(), identity, report); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, ErrDeliveryReport) {
			status = http.StatusConflict
		}
		http.Error(w, "collector delivery status rejected", status)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusNoContent)
}

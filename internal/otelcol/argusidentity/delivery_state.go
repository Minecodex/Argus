package argusidentity

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/prometheus/common/expfmt"
	"github.com/prometheus/common/model"
)

const deliverySchema = "argus.collector_delivery/v1"
const maxDeliveryState = 768 << 10

// Counts are observed native terminal export failures, never a per-target loss
// estimate. Receiver refusals include retried attempts and are kept separate.
type deliveryCounts struct {
	LogRecords   int64 `json:"log_records"`
	Spans        int64 `json:"spans"`
	MetricPoints int64 `json:"metric_points"`
}

type deliveryReport struct {
	SchemaVersion     string         `json:"schema_version"`
	InstanceID        string         `json:"instance_id"`
	Sequence          int64          `json:"sequence"`
	ObservedSince     time.Time      `json:"observed_since"`
	ObservedAt        time.Time      `json:"observed_at"`
	LastFailureAt     *time.Time     `json:"last_failure_at,omitempty"`
	TerminalFailures  deliveryCounts `json:"terminal_failures"`
	ReceiverRefusals  deliveryCounts `json:"receiver_refusals"`
	EnqueueFailures   deliveryCounts `json:"enqueue_failures"`
	RetryLimitSeconds int64          `json:"retry_limit_seconds"`
	Attribution       string         `json:"attribution"`
}

type deliveryState struct {
	Report    deliveryReport   `json:"report"`
	ProcessID string           `json:"process_id"`
	Counters  map[string]int64 `json:"counters"`
}

type deliverySnapshot struct {
	ProcessID string
	Counters  map[string]int64
}

func loadDeliveryState(path string, now time.Time) (deliveryState, error) {
	var state deliveryState
	value, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		var identity [16]byte
		if _, err = rand.Read(identity[:]); err != nil {
			return state, err
		}
		state.Report = deliveryReport{SchemaVersion: deliverySchema, InstanceID: hex.EncodeToString(identity[:]), ObservedSince: now, ObservedAt: now, RetryLimitSeconds: 300, Attribution: "collector_shared_pipeline_exact_targets_unknown"}
		state.Counters = map[string]int64{}
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if len(value) > maxDeliveryState || json.Unmarshal(value, &state) != nil || state.Report.SchemaVersion != deliverySchema || len(state.Report.InstanceID) != 32 || state.Report.Sequence < 0 || len(state.Counters) > 4096 {
		return state, errors.New("collector delivery ledger is invalid; previous evidence was preserved")
	}
	if _, err = hex.DecodeString(state.Report.InstanceID); err != nil {
		return state, errors.New("collector delivery ledger identity is invalid")
	}
	r := state.Report
	if r.RetryLimitSeconds != 300 || r.Attribution != "collector_shared_pipeline_exact_targets_unknown" || r.ObservedSince.IsZero() || r.ObservedAt.Before(r.ObservedSince) || r.TerminalFailures.LogRecords < 0 || r.TerminalFailures.Spans < 0 || r.TerminalFailures.MetricPoints < 0 || r.ReceiverRefusals.LogRecords < 0 || r.ReceiverRefusals.Spans < 0 || r.ReceiverRefusals.MetricPoints < 0 || r.EnqueueFailures.LogRecords < 0 || r.EnqueueFailures.Spans < 0 || r.EnqueueFailures.MetricPoints < 0 {
		return state, errors.New("collector delivery ledger facts are invalid")
	}
	for key, value := range state.Counters {
		parts := strings.Split(key, ":")
		if value < 0 || len(parts) != 3 || len(parts[2]) != 64 {
			return state, errors.New("collector delivery counter ledger is invalid")
		}
		if _, err := hex.DecodeString(parts[2]); err != nil {
			return state, errors.New("collector delivery counter ledger is invalid")
		}
		if _, ok := deliveryMetricKind("otelcol_" + parts[0] + "_" + parts[1]); !ok {
			return state, errors.New("collector delivery counter ledger is invalid")
		}
	}
	if state.Counters == nil {
		state.Counters = map[string]int64{}
	}
	return state, nil
}

func saveDeliveryState(path string, state deliveryState) error {
	value, err := json.Marshal(state)
	if err != nil || len(value) > maxDeliveryState {
		return errors.New("collector delivery ledger exceeds its bounded storage")
	}
	return writeAtomic(path, value, 0o600)
}

func deliveryMetricKind(name string) (string, bool) {
	name = strings.TrimSuffix(name, "_total")
	for _, category := range []string{"exporter_send_failed", "receiver_refused", "exporter_enqueue_failed"} {
		for _, signal := range []string{"log_records", "spans", "metric_points"} {
			if name == "otelcol_"+category+"_"+signal {
				return category + ":" + signal, true
			}
		}
	}
	return "", false
}

func readDeliverySnapshot(reader io.Reader) (deliverySnapshot, error) {
	parser := expfmt.NewTextParser(model.UTF8Validation)
	families, err := parser.TextToMetricFamilies(reader)
	if err != nil {
		return deliverySnapshot{}, errors.New("native Collector metric response is invalid")
	}
	out := deliverySnapshot{Counters: map[string]int64{}}
	for name, family := range families {
		if name == "target_info" {
			for _, sample := range family.Metric {
				for _, label := range sample.Label {
					if label.GetName() == "service_instance_id" {
						if out.ProcessID != "" && out.ProcessID != label.GetValue() {
							return out, errors.New("multiple Collector process identities")
						}
						out.ProcessID = label.GetValue()
					}
				}
			}
			continue
		}
		kind, ok := deliveryMetricKind(name)
		if !ok {
			continue
		}
		for _, sample := range family.Metric {
			if sample.Counter == nil {
				return out, errors.New("native delivery metric is not a counter")
			}
			value := sample.Counter.GetValue()
			if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value >= float64(math.MaxInt64) || math.Trunc(value) != value {
				return out, errors.New("native delivery metric count is invalid")
			}
			labels := make([]string, 0, len(sample.Label))
			for _, label := range sample.Label {
				labels = append(labels, label.GetName()+"="+label.GetValue())
			}
			slices.Sort(labels)
			digest := sha256.Sum256([]byte(strings.Join(labels, "\x00")))
			key := kind + ":" + hex.EncodeToString(digest[:])
			if _, exists := out.Counters[key]; exists {
				return out, errors.New("duplicate native delivery metric")
			}
			out.Counters[key] = int64(value)
			if len(out.Counters) > 4096 {
				return out, errors.New("native delivery observation cardinality exceeds limit")
			}
		}
	}
	if out.ProcessID == "" || len(out.ProcessID) > 128 {
		return out, errors.New("native Collector process identity is unavailable")
	}
	return out, nil
}

func (state *deliveryState) observe(snapshot deliverySnapshot, now time.Time) error {
	// Work on a copy: validation/overflow must not partly advance the persisted
	// baseline, otherwise retrying the same observation could hide lost data.
	next := *state
	next.Counters = make(map[string]int64, len(state.Counters)+len(snapshot.Counters))
	if state.ProcessID == snapshot.ProcessID {
		for key, value := range state.Counters {
			next.Counters[key] = value
		}
	}
	next.ProcessID = snapshot.ProcessID
	for key, value := range snapshot.Counters {
		before := next.Counters[key]
		delta := value - before
		if delta < 0 {
			delta = value
		}
		parts := strings.Split(key, ":")
		if len(parts) != 3 || delta < 0 {
			return errors.New("invalid delivery observation key")
		}
		var counts *deliveryCounts
		switch parts[0] {
		case "exporter_send_failed":
			counts = &next.Report.TerminalFailures
		case "receiver_refused":
			counts = &next.Report.ReceiverRefusals
		case "exporter_enqueue_failed":
			counts = &next.Report.EnqueueFailures
		default:
			return errors.New("invalid delivery observation category")
		}
		var total *int64
		switch parts[1] {
		case "log_records":
			total = &counts.LogRecords
		case "spans":
			total = &counts.Spans
		case "metric_points":
			total = &counts.MetricPoints
		default:
			return errors.New("invalid delivery observation signal")
		}
		if *total > math.MaxInt64-delta {
			return errors.New("delivery observation count overflow")
		}
		*total += delta
		if delta > 0 {
			at := now
			next.Report.LastFailureAt = &at
		}
		next.Counters[key] = value
	}
	if len(next.Counters) > 4096 || next.Report.Sequence == math.MaxInt64 {
		return errors.New("delivery observation ledger limit exceeded")
	}
	next.Report.Sequence++
	next.Report.ObservedAt = now
	*state = next
	return nil
}

func deliveryReportError(status int) error {
	return fmt.Errorf("collector delivery status endpoint returned %d", status)
}

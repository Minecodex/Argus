//go:build m4e2e

package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func TestMetricsRequestIncludesProfileAndRecoveryMetrics(t *testing.T) {
	request := metricsRequest(&resourcepb.Resource{}, uint64(time.Now().UnixNano()), ".backlog")
	metrics := request.ResourceMetrics[0].ScopeMetrics[0].Metrics
	if len(metrics) != 5 {
		t.Fatalf("metric count = %d, want 5", len(metrics))
	}
	if metrics[0].Name != "system.cpu.utilization" || metrics[0].Unit != "1" {
		t.Fatalf("profile metric = %q %q", metrics[0].Name, metrics[0].Unit)
	}
	if metrics[1].Name != "argus.m7.e2e.gauge.backlog" {
		t.Fatalf("recovery metric = %q", metrics[1].Name)
	}
	if metrics[2].Name != "argus.m7.e2e.native.histogram.backlog" || metrics[2].GetExponentialHistogram() == nil {
		t.Fatalf("native histogram metric = %q", metrics[2].Name)
	}
	if metrics[3].Name != "argus.m7.e2e.summary.backlog" || metrics[3].GetSummary() == nil {
		t.Fatalf("summary metric = %q", metrics[3].Name)
	}
	histogram := metrics[4].GetHistogram()
	if histogram == nil || len(histogram.DataPoints) != 1 {
		t.Fatal("classic histogram needed for real bucket rendering")
	}
	point := histogram.DataPoints[0]
	var total uint64
	for _, count := range point.BucketCounts {
		total += count
	}
	if len(point.BucketCounts) != len(point.ExplicitBounds)+1 || total != point.Count || point.GetSum() != 16 {
		t.Fatal("histogram count, sum or bounds are inconsistent")
	}
}

func TestExportWithRetryWaitsForCollectorReadiness(t *testing.T) {
	var attempts atomic.Int32
	err := exportWithRetry(context.Background(), "127.0.0.1:4317", insecure.NewCredentials(), time.Millisecond, func(grpc.ClientConnInterface) error {
		if attempts.Add(1) < 3 {
			return errors.New("collector is starting")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := attempts.Load(); got != 3 {
		t.Fatalf("attempt count = %d, want 3", got)
	}
}

func TestExportWithRetryReturnsLastErrorAtDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	want := errors.New("collector unavailable")
	err := exportWithRetry(ctx, "127.0.0.1:4317", insecure.NewCredentials(), time.Millisecond, func(grpc.ClientConnInterface) error {
		return want
	})
	if !errors.Is(err, want) {
		t.Fatalf("export error = %v, want %v", err, want)
	}
}

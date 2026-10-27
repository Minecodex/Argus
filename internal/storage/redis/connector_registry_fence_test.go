package redis

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	redislib "github.com/redis/go-redis/v9"
)

func TestRedisConnectorRegistryRejectsStaleSetAndDisconnect(t *testing.T) {
	address := os.Getenv("ARGUS_PLAN8_REDIS_TEST_URL")
	if address == "" {
		t.Skip("requires explicit disposable Redis")
	}
	client, err := Open(t.Context(), address)
	if err != nil {
		t.Fatal("open disposable Redis")
	}
	t.Cleanup(func() { _ = client.Close() })
	id := uuid.NewString()
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := client.DeleteConnectorRegistry(ctx, id); err != nil {
			t.Error("owned cache key cleanup", err)
		}
	})
	put := func(epoch int64, gateway string) {
		t.Helper()
		if err := client.SetConnectorRegistry(t.Context(), id, ConnectorRegistryEntry{ConnectionEpoch: epoch, GatewayInstanceID: gateway}, time.Minute); err != nil {
			t.Fatal(err)
		}
	}
	expect := func(epoch int64, gateway string) {
		t.Helper()
		entry, err := client.GetConnectorRegistry(t.Context(), id)
		if err != nil || entry.ConnectionEpoch != epoch || entry.GatewayInstanceID != gateway {
			t.Fatalf("registry epoch/gateway mismatch: %d/%s err=%v", entry.ConnectionEpoch, entry.GatewayInstanceID, err)
		}
	}
	put(2, "new")
	put(1, "old")
	put(2, "wrong-gateway")
	expect(2, "new")
	if err = client.DeleteConnectorRegistryEpoch(t.Context(), id, 1); err != nil {
		t.Fatal(err)
	}
	expect(2, "new")
	if err = client.DeleteConnectorRegistryEpoch(t.Context(), id, 2); err != nil {
		t.Fatal(err)
	}
	if _, err = client.GetConnectorRegistry(t.Context(), id); !errors.Is(err, redislib.Nil) {
		t.Fatal("matching disconnect did not retire its registry")
	}
	var group sync.WaitGroup
	for epoch := int64(3); epoch <= 24; epoch++ {
		group.Go(func() {
			gateway := fmt.Sprintf("gateway-%d", epoch)
			if err := client.SetConnectorRegistry(t.Context(), id, ConnectorRegistryEntry{ConnectionEpoch: epoch, GatewayInstanceID: gateway}, time.Minute); err != nil {
				t.Error(err)
			}
			if epoch < 24 {
				if err := client.DeleteConnectorRegistryEpoch(t.Context(), id, epoch); err != nil {
					t.Error(err)
				}
			}
		})
	}
	group.Wait()
	expect(24, "gateway-24")
}

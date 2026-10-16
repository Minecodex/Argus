package connector

import (
	"context"
	"encoding/json"
	"github.com/kakj-go/Argus/internal/hostinventory"
	"testing"
	"time"
)

func TestInventoryCacheAndExplicitRefreshUseActualCollection(t *testing.T) {
	sentinel, _ := json.Marshal(hostinventory.Snapshot{SchemaVersion: hostinventory.SchemaVersion, CollectedAt: time.Now(), Hostname: "cache-sentinel", Scope: "machine"})
	inventoryCache.Lock()
	inventoryCache.raw, inventoryCache.collected = sentinel, time.Now()
	inventoryCache.Unlock()
	defer func() {
		inventoryCache.Lock()
		inventoryCache.raw = nil
		inventoryCache.collected = time.Time{}
		inventoryCache.Unlock()
	}()
	if string(observeInventory(context.Background(), false)) != string(sentinel) {
		t.Fatal("periodic observation did not reuse fresh snapshot")
	}
	fresh := observeInventory(context.Background(), true)
	snapshot, err := hostinventory.Decode(fresh)
	if err != nil || snapshot == nil || snapshot.Hostname == "cache-sentinel" {
		t.Fatalf("refresh did not collect local inventory: %v", err)
	}
	if snapshot.CPU == nil || snapshot.CPU.LogicalCores < 1 || snapshot.Memory == nil || snapshot.Memory.TotalBytes == 0 {
		t.Fatalf("local CPU and memory missing: %+v", snapshot)
	}
}

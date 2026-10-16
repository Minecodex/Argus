package connector

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/kakj-go/Argus/internal/hostinventory"
)

// inventory is the same bounded observer as the heartbeat and refresh command,
// exposed for local diagnostics without enrollment or configuration changes.
func runInventory(ctx context.Context, args []string) error {
	if len(args) != 0 {
		return errors.New("usage: argus-connector inventory")
	}
	return json.NewEncoder(os.Stdout).Encode(hostinventory.Collect(ctx))
}

var inventoryCache struct {
	sync.Mutex
	raw       []byte
	collected time.Time
}

func observeInventory(ctx context.Context, refresh bool) []byte {
	inventoryCache.Lock()
	defer inventoryCache.Unlock()
	if !refresh && len(inventoryCache.raw) > 0 && time.Since(inventoryCache.collected) < 5*time.Minute {
		return inventoryCache.raw
	}
	snapshot := hostinventory.Collect(ctx)
	raw, err := json.Marshal(snapshot)
	if err != nil || len(raw) > hostinventory.MaxBytes {
		return nil
	}
	inventoryCache.raw, inventoryCache.collected = raw, time.Now()
	return raw
}

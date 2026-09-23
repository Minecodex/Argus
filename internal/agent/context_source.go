package agent

import (
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// ContextSource travels with the assembled messages. Never reread the current
// active snapshot to infer which summary an earlier assembly actually used.
type ContextSource struct {
	Snapshot                      *db.ContextSnapshot
	FromSequence, ThroughSequence int64
}

func (source ContextSource) snapshotID() uuid.NullUUID {
	if source.Snapshot == nil {
		return uuid.NullUUID{}
	}
	return uuid.NullUUID{UUID: source.Snapshot.ID, Valid: true}
}
func (source ContextSource) snapshotHash() []byte {
	if source.Snapshot == nil {
		return nil
	}
	return source.Snapshot.SnapshotHash
}

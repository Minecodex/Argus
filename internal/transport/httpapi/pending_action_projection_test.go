package httpapi

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/stretchr/testify/require"

	"github.com/kakj-go/Argus/internal/gen/openapi/actionapi"
	"github.com/kakj-go/Argus/internal/kubernetesinventory"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestConvertPendingExposesStableActionType(t *testing.T) {
	now := time.Now().UTC()
	result := convertPending[actionapi.PendingActionPublicSchema](db.PendingAction{
		ActionRef:  "pa-test",
		ActionType: "bastion_scope.create",
		Title:      "Create bastion",
		Summary:    "Create a stable Bastion Scope",
		Risk:       "dangerous",
		Preview:    []byte(`{"name":"edge-gateway"}`),
		Diff:       []byte(`[{"kind":"add","text":"Create bastion edge-gateway"}]`),
		Status:     "awaiting_confirmation",
		ExpiresAt:  pgtype.Timestamptz{Time: now.Add(time.Minute), Valid: true},
		CreatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
		UpdatedAt:  pgtype.Timestamptz{Time: now, Valid: true},
	})

	require.Equal(t, "bastion_scope.create", result.ActionType)
}

func TestKubernetesDiffSurvivesPublicActionProjection(t *testing.T) {
	lines, err := kubernetesworkbench.PublicMutationDiff(kubernetesinventory.MutationResult{After: &kubernetesinventory.Object{Data: json.RawMessage(`{"metadata":{"name":"settings"},"data":{"mode":"reviewed"}}`)}})
	require.NoError(t, err)
	raw, err := json.Marshal(lines)
	require.NoError(t, err)
	result := convertPending[actionapi.PendingActionPublicSchema](db.PendingAction{ActionType: "kubernetes.object.create", Preview: []byte(`{"name":"settings"}`), Diff: raw})
	require.Len(t, result.Diff, len(lines))
	require.Contains(t, string(raw), "reviewed")
}

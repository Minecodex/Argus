package connector

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestRunUninstallLocalRejectsJournalForAnotherOperation(t *testing.T) {
	operationID := uuid.New()
	err := runUninstallLocal(context.Background(), []string{
		"--operation-id", operationID.String(),
		"--connector-id", uuid.New().String(),
		"--removal-generation", "1",
		"--journal-dir", filepath.Join(t.TempDir(), uuid.New().String()),
	})
	if err == nil || !strings.Contains(err.Error(), "arguments are invalid") {
		t.Fatalf("operation-scoped journal validation was not enforced: %v", err)
	}
}

func TestWriteLocalUninstallJournalPersistsStateAndHistory(t *testing.T) {
	directory := t.TempDir()
	result := localCleanupResult{CollectorServiceAbsent: true, CollectorFilesAbsent: true}
	if err := writeLocalUninstallJournal(directory, "uninstalling_workloads", "succeeded", &result); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalUninstallJournal(directory, "uninstalling_connector", "running", nil); err != nil {
		t.Fatal(err)
	}
	state, err := os.ReadFile(filepath.Join(directory, "state.json"))
	if err != nil {
		t.Fatal(err)
	}
	var current localUninstallJournalEntry
	if err = json.Unmarshal(state, &current); err != nil || current.Stage != "uninstalling_connector" || current.Status != "running" {
		t.Fatalf("unexpected current journal state: %+v, %v", current, err)
	}
	events, err := os.ReadFile(filepath.Join(directory, "events.jsonl"))
	if err != nil || strings.Count(strings.TrimSpace(string(events)), "\n") != 1 {
		t.Fatalf("journal history was not appended: %q, %v", events, err)
	}
}

func TestRemovalBindingRejectsNewInstallationAndResumesMissingIdentity(t *testing.T) {
	identityDir, journal := t.TempDir(), t.TempDir()
	expected := removalBinding{OperationID: uuid.New(), ConnectorID: uuid.New(), Generation: 1}
	identityPath := filepath.Join(identityDir, identityFile)
	writeIdentity := func(id uuid.UUID) {
		t.Helper()
		raw, _ := json.Marshal(map[string]string{"connector_id": id.String()})
		if err := os.WriteFile(identityPath, raw, 0600); err != nil {
			t.Fatal(err)
		}
	}
	writeIdentity(expected.ConnectorID)
	if err := ensureLocalRemovalBinding(identityDir, journal, expected); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(identityPath); err != nil {
		t.Fatal(err)
	}
	if err := ensureLocalRemovalBinding(identityDir, journal, expected); err != nil {
		t.Fatalf("retry after identity deletion failed: %v", err)
	}
	writeIdentity(uuid.New())
	if err := ensureLocalRemovalBinding(identityDir, journal, expected); err == nil {
		t.Fatal("old operation accepted new installation")
	}
	writeIdentity(expected.ConnectorID)
	expected.Generation++
	if err := ensureLocalRemovalBinding(identityDir, journal, expected); err == nil {
		t.Fatal("journal reused across removal generations")
	}
}

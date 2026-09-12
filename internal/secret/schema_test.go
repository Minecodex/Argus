package secret

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/kakj-go/Argus/internal/keywrap"
)

func TestSecretProviderMigrationMatchesRuntimeProviders(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	migration, err := os.ReadFile(filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	for _, provider := range []string{EnvelopeProviderLocal, keywrap.ProviderOpenBaoTransit} {
		if !strings.Contains(text, "'"+provider+"'") {
			t.Fatalf("Secret provider migration does not allow runtime provider %q", provider)
		}
	}
	if strings.Contains(text, "'vault'::text") {
		t.Fatal("Secret provider baseline retained unsupported provider values")
	}
}

func TestKeyWrappingStorageContractsCoverEveryPersistentModule(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	migration, err := os.ReadFile(filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	for _, required := range []string{
		"wrap_nonce bytea",
		"ai_model_credentials_provider_check",
		"idempotency_records_provider_check",
		"sandbox_backends_credential_envelope_check",
		"remote_access_recordings_key_provider_check",
		"provider = ANY (ARRAY['local'::text, 'openbao_transit'::text])",
		"response_provider = ANY (ARRAY['local_test'::text, 'openbao_transit'::text])",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("key wrapping storage migration is missing %q", required)
		}
	}
}

func TestSandboxCredentialContractRejectsPartialNullableEnvelope(t *testing.T) {
	_, current, _, _ := runtime.Caller(0)
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	migration, err := os.ReadFile(filepath.Join(root, "migrations", "postgresql", "00001_argus_baseline.sql"))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.SplitN(string(migration), "-- +goose Down", 2)[0]
	for _, column := range []string{"credential_wrapped_dek", "credential_nonce", "credential_ciphertext", "credential_value_hash"} {
		if !strings.Contains(text, "("+column+" IS NOT NULL)") {
			t.Fatalf("Sandbox credential contract does not require %s", column)
		}
	}
}

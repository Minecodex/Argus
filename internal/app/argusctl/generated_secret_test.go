package argusctl

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRandomSecretNeverStartsWithCLIOptionPrefix(t *testing.T) {
	for index := 0; index < 512; index++ {
		value, err := randomSecret(24)
		if err != nil {
			t.Fatal(err)
		}
		if len(value) != 32 {
			t.Fatalf("encoded secret length = %d, want 32", len(value))
		}
		if value[0] == '-' || value[0] == '_' {
			t.Fatalf("generated secret starts with unsafe CLI prefix: %q", value[:1])
		}
	}
}

func TestClickHouseMigrationAttachesPasswordValueToOption(t *testing.T) {
	path := filepath.Join("..", "..", "..", "deploy", "helm", "argus-telemetry-pipeline", "templates", "migration.yaml")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	if strings.Contains(text, `--password "$CLICKHOUSE_PASSWORD"`) || strings.Count(text, `--password="$CLICKHOUSE_PASSWORD"`) != 2 {
		t.Fatal("ClickHouse migration must attach the quoted password value to --password")
	}
}

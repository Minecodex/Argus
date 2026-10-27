package collectormanager

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

func TestPrivateMonitoringOverlayScopeAndLiteralCredentials(t *testing.T) {
	id := uuid.NewString()
	name := "mysql/collection_" + strings.ReplaceAll(id, "-", "")
	bundle, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, Collections: []configbundle.Collection{{ID: id, Integration: "mysql", Identity: json.RawMessage(`{"endpoint":"db:3306"}`), Config: json.RawMessage(`{}`), Enabled: true, HasCredential: true}}})
	command := &connectorv1.CollectorManagementCommand{RenderedConfig: bundle}
	if _, err := monitoringPrivateConfig(command); err == nil {
		t.Fatal("missing private credentials accepted")
	}
	command.PrivateMonitoringConfig, _ = json.Marshal(map[string]any{"receivers": map[string]any{name: map[string]string{"username": "monitor", "password": "literal-${env:HOME}-$value"}}})
	private, err := monitoringPrivateConfig(command)
	if err != nil || !bytes.Contains(private, []byte("literal-$${env:HOME}-$$value")) {
		t.Fatalf("literal password handling: %v", err)
	}
	clear(private)
	for _, value := range []any{
		map[string]any{"receivers": map[string]any{name: map[string]string{"username": "monitor", "password": "x", "endpoint": "attacker:3306"}}},
		map[string]any{"receivers": map[string]any{"mysql/other": map[string]string{"username": "monitor", "password": "x"}}},
		map[string]any{"receivers": map[string]any{name: map[string]string{"username": "monitor", "password": "x"}}, "exporters": map[string]any{}},
	} {
		command.PrivateMonitoringConfig, _ = json.Marshal(value)
		if _, err := monitoringPrivateConfig(command); err == nil {
			t.Fatal("private overlay escaped authentication scope")
		}
	}
}

func TestOraclePrivateOverlayPreservesPasswordBytesAcrossAuthenticationBoundary(t *testing.T) {
	for _, length := range []int{17, 25, 30, 31, 41, 42} {
		id := uuid.NewString()
		name := "oracledb/collection_" + strings.ReplaceAll(id, "-", "")
		password := strings.Repeat("a", length-3) + "B9!"
		bundle, _ := json.Marshal(configbundle.Bundle{SchemaVersion: configbundle.SchemaVersion, Collections: []configbundle.Collection{{ID: id, Integration: "oracle", Identity: json.RawMessage(`{"endpoint":"oracle:1521","database":"FREEPDB1"}`), Config: json.RawMessage(`{"interval":"5s","timeout":"5s"}`), Enabled: true, HasCredential: true}}})
		privateInput, _ := json.Marshal(map[string]any{"receivers": map[string]any{name: map[string]string{"username": "monitor", "password": password}}})
		out, err := monitoringPrivateConfig(&connectorv1.CollectorManagementCommand{RenderedConfig: bundle, PrivateMonitoringConfig: privateInput})
		if err != nil {
			t.Fatal("Oracle credential compilation rejected", length, err)
		}
		var decoded struct {
			Receivers map[string]map[string]string `json:"receivers"`
		}
		if json.Unmarshal(out, &decoded) != nil || decoded.Receivers[name]["password"] != password || decoded.Receivers[name]["username"] != "monitor" {
			t.Fatal("Oracle credential bytes changed", length)
		}
		clear(out)
		clear(privateInput)
	}
}

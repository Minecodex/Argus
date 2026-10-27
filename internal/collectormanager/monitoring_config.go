package collectormanager

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
)

// monitoringPrivateConfig accepts authentication values only for the exact
// enabled assignments in the public bundle. It cannot inject other components.
func monitoringPrivateConfig(command *connectorv1.CollectorManagementCommand) ([]byte, error) {
	var bundle configbundle.Bundle
	if json.Unmarshal(command.GetRenderedConfig(), &bundle) != nil || bundle.SchemaVersion != configbundle.SchemaVersion {
		return nil, ErrInvalidCommand
	}
	expected := map[string]configbundle.Collection{}
	for _, collection := range bundle.Collections {
		if !collection.Enabled || !collection.HasCredential {
			continue
		}
		entry, exists := monitoringcatalog.Lookup(collection.Integration)
		if !exists {
			return nil, ErrInvalidCommand
		}
		expected[entry.Component+"/collection_"+strings.ReplaceAll(collection.ID, "-", "")] = collection
	}
	var private struct {
		Receivers map[string]json.RawMessage `json:"receivers"`
	}
	raw := command.GetPrivateMonitoringConfig()
	if len(raw) == 0 && len(expected) == 0 {
		return []byte(`{"receivers":{}}`), nil
	}
	if len(raw) == 0 || len(raw) > 4<<20 {
		return nil, ErrInvalidCommand
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&private) != nil || decoder.Decode(new(any)) != io.EOF || len(private.Receivers) != len(expected) {
		return nil, ErrInvalidCommand
	}
	out := map[string]map[string]any{}
	for receiver, config := range private.Receivers {
		collection, ok := expected[receiver]
		if !ok {
			return nil, ErrInvalidCommand
		}
		overlay, err := monitoringcatalog.CredentialOverlay(collection.Integration, collection.Identity, collection.Config, config)
		if err != nil {
			return nil, ErrInvalidCommand
		}
		out[receiver] = overlay
	}
	return json.Marshal(map[string]any{"receivers": out})
}

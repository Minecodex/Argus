package configbundle

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/google/uuid"
)

// ApplicationEndpoint is a public, frozen routing contract, never a Key list.
// Multiple applications share one authenticated receiver per protocol.
type ApplicationEndpoint struct {
	ID            string   `json:"id"`
	ApplicationID string   `json:"application_id"`
	Protocol      string   `json:"protocol"`
	Signals       []string `json:"signals"`
	Version       int64    `json:"version"`
}

func addApplicationEndpoints(config map[string]any, input RenderInput) ([]Source, error) {
	if len(input.ApplicationEndpoints) == 0 {
		return nil, nil
	}
	if len(input.ApplicationEndpoints) > 256 || !slices.Contains(DistributionComponents(input.Platform), "argus_application_identity") {
		return nil, errors.New("application collection capability unavailable")
	}
	generation, err := uuid.Parse(input.SourceGeneration)
	if err != nil || generation == uuid.Nil {
		return nil, errors.New("application source generation required")
	}
	receivers := config["receivers"].(map[string]any)
	processors := config["processors"].(map[string]any)
	extensions := config["extensions"].(map[string]any)
	service := config["service"].(map[string]any)
	pipelines := service["pipelines"].(map[string]any)
	identityDirectory := "/var/lib/argus-otelcol/identity"
	if input.Platform == "windows_amd64" {
		identityDirectory = `C:\ProgramData\Argus\Collector\identity`
	}
	if identity, ok := extensions["argus_identity"].(map[string]any); ok {
		// Use the exact target paths of the existing identity extension.
		if path, ok := identity["server_certificate_file"].(string); ok {
			identityDirectory = strings.TrimSuffix(strings.TrimSuffix(path, "/server.pem"), `\server.pem`)
		}
	}
	tls := map[string]any{"cert_file": targetPath(identityDirectory, "server.pem"), "key_file": targetPath(identityDirectory, "server-key.pem")}
	byProtocol := map[string][]map[string]any{}
	signals := map[string][]string{}
	result := []Source{}
	seen := map[string]bool{}
	for _, endpoint := range input.ApplicationEndpoints {
		id, e := uuid.Parse(endpoint.ID)
		app, e2 := uuid.Parse(endpoint.ApplicationID)
		if e != nil || e2 != nil || id == uuid.Nil || app == uuid.Nil || endpoint.Version < 1 || seen[endpoint.ID] || !slices.Contains([]string{"otlp", "jaeger", "skywalking"}, endpoint.Protocol) || len(endpoint.Signals) == 0 {
			return nil, errors.New("invalid application endpoint")
		}
		seen[endpoint.ID] = true
		key := "application/" + endpoint.ID
		sourceID := uuid.NewSHA1(generation, []byte(key)).String()
		for _, signal := range endpoint.Signals {
			if !slices.Contains([]string{"metrics", "logs", "traces"}, signal) || endpoint.Protocol != "otlp" && signal != "traces" {
				return nil, errors.New("invalid application signal")
			}
			if !slices.Contains(signals[endpoint.Protocol], signal) {
				signals[endpoint.Protocol] = append(signals[endpoint.Protocol], signal)
			}
		}
		byProtocol[endpoint.Protocol] = append(byProtocol[endpoint.Protocol], map[string]any{"endpoint_id": endpoint.ID, "endpoint_version": endpoint.Version, "source_id": sourceID, "source_revision": input.ConfigRevision})
		result = append(result, Source{ID: sourceID, Generation: generation.String(), Key: key, Type: endpoint.Protocol, Signals: endpoint.Signals, Revision: input.ConfigRevision, CapabilityVersion: "application/v1", ResourceType: "application", ResourceID: endpoint.ApplicationID, IngestEndpointID: endpoint.ID, IngestEndpointVersion: endpoint.Version})
	}
	for _, protocol := range []string{"otlp", "jaeger", "skywalking"} {
		bindings := byProtocol[protocol]
		if len(bindings) == 0 {
			continue
		}
		authName := "argus_application_auth/" + protocol
		processorName := "argus_application_identity/" + protocol
		receiverName := protocol + "/application"
		extensions[authName] = map[string]any{"protocol": protocol, "identity_extension": "argus_identity"}
		service["extensions"] = append(service["extensions"].([]string), authName)
		processors[processorName] = map[string]any{"bindings": bindings}
		server := func(port int32) map[string]any {
			return map[string]any{"endpoint": fmt.Sprintf("0.0.0.0:%d", port), "tls": tls, "auth": map[string]any{"authenticator": authName}}
		}
		protocols := map[string]any{}
		for _, port := range ApplicationProtocolPorts(protocol) {
			if input.Transport != "direct" && (input.TunnelLoopbackPort == int(port.Port) || input.TunnelLoopbackPort+1 == int(port.Port)) {
				return nil, errors.New("SDK listener conflicts with Collector tunnel")
			}
			protocols[port.Transport] = server(port.Port)
		}
		receivers[receiverName] = map[string]any{"protocols": protocols}
		for _, signal := range signals[protocol] {
			pipelines[signal+"/application_"+protocol] = map[string]any{"receivers": []string{receiverName}, "processors": []string{processorName, "memory_limiter"}, "exporters": []string{"otlp/argus"}}
		}
	}
	return result, nil
}

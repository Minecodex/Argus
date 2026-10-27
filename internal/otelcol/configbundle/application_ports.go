package configbundle

import "encoding/json"

type ApplicationPort struct {
	Name      string `json:"name"`
	Port      int32  `json:"port"`
	Transport string `json:"transport"`
	Path      string `json:"path"`
}

// SDK listeners are separate from Collector-to-Collector mTLS and tunnel ports.
func ApplicationProtocolPorts(protocol string) []ApplicationPort {
	switch protocol {
	case "otlp":
		return []ApplicationPort{{Name: "sdk-otlp-grpc", Port: 24317, Transport: "grpc"}, {Name: "sdk-otlp-http", Port: 24318, Transport: "http"}}
	case "jaeger":
		return []ApplicationPort{{Name: "sdk-jaeger-grpc", Port: 24250, Transport: "grpc"}, {Name: "sdk-jaeger-http", Port: 24268, Transport: "thrift_http", Path: "/api/traces"}}
	case "skywalking":
		return []ApplicationPort{{Name: "sdk-sky-grpc", Port: 21800, Transport: "grpc"}, {Name: "sdk-sky-http", Port: 21801, Transport: "http", Path: "/v3/segments"}}
	default:
		return nil
	}
}
func ApplicationPortsFromBundle(raw []byte) []ApplicationPort {
	var bundle Bundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.SchemaVersion != SchemaVersion {
		return nil
	}
	seen := map[string]bool{}
	for _, endpoint := range bundle.ApplicationEndpoints {
		seen[endpoint.Protocol] = true
	}
	result := []ApplicationPort{}
	for _, protocol := range []string{"otlp", "jaeger", "skywalking"} {
		if seen[protocol] {
			result = append(result, ApplicationProtocolPorts(protocol)...)
		}
	}
	return result
}

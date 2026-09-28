package configbundle

// Native receivers remain local to the monitored workload. Encrypted, authenticated
// OTLP forwarding and the existing source registration enforce the Argus boundary.
func nativeTraceReceivers(profiles map[string]bool, receivers map[string]any) {
	if profiles["skywalking-receiver"] {
		receivers["skywalking"] = map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "127.0.0.1:11800"}}}
	}
	if profiles["jaeger-receiver"] {
		receivers["jaeger"] = map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "127.0.0.1:14250"}, "thrift_http": map[string]any{"endpoint": "127.0.0.1:14268"}}}
	}
}

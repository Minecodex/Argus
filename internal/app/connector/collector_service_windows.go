//go:build windows

package connector

import (
	"context"
	"errors"
	"flag"
	"os"
	"os/exec"
)

func runCollectorService(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("argus-connector collector-service", flag.ContinueOnError)
	binary := flags.String("binary", "", "Collector executable")
	config := flags.String("config", "", "Collector configuration")
	tokenFile := flags.String("token-file", "", "Collector enrollment token file")
	enrollment := flags.String("enrollment-endpoint", "", "Telemetry enrollment endpoint")
	grpcEndpoint := flags.String("ingest-grpc-endpoint", "", "Telemetry gRPC endpoint")
	httpEndpoint := flags.String("ingest-http-endpoint", "", "Telemetry HTTP endpoint")
	if err := flags.Parse(args); err != nil {
		return err
	}
	for _, value := range []string{*binary, *config, *tokenFile, *enrollment, *grpcEndpoint, *httpEndpoint} {
		if value == "" {
			return errors.New("Collector Service configuration is incomplete")
		}
	}
	if _, err := os.Stat(*binary); err != nil {
		return err
	}
	command := exec.CommandContext(ctx, *binary, "--config="+*config)
	command.Env = append(os.Environ(),
		"ARGUS_TELEMETRY_ENROLLMENT_TOKEN_FILE="+*tokenFile,
		"ARGUS_TELEMETRY_ENROLLMENT_ENDPOINT="+*enrollment,
		"ARGUS_TELEMETRY_INGEST_GRPC_ENDPOINT="+*grpcEndpoint,
		"ARGUS_TELEMETRY_INGEST_HTTP_ENDPOINT="+*httpEndpoint,
	)
	return command.Run()
}

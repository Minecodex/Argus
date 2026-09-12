// Package connector contains the enterprise-side Connector runtime.
package connector

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

const softwareVersion = "0.1.0-m3"

func Run(ctx context.Context, logger *slog.Logger) error {
	command := "run"
	args := os.Args[1:]
	if len(args) > 0 && (args[0] == "run" || args[0] == "enroll" || args[0] == "repair" || args[0] == "repair-collector" || args[0] == "probe" || args[0] == "bootstrap-state" || args[0] == "collector-service" || args[0] == "privileged-helper" || args[0] == "verify-artifact" || args[0] == "self-test" || args[0] == "uninstall-local") {
		command, args = args[0], args[1:]
	}
	switch command {
	case "enroll":
		return runEnroll(ctx, logger, args)
	case "repair":
		return runRepair(ctx, logger, args)
	case "repair-collector":
		return runRepairCollector(ctx, logger, args)
	case "run":
		return runConnector(ctx, logger, args)
	case "probe":
		return runProbe(ctx, args)
	case "bootstrap-state":
		return runBootstrapState(args)
	case "collector-service":
		return runCollectorService(ctx, args)
	case "privileged-helper":
		return runPrivilegedHelper(ctx, args)
	case "verify-artifact":
		return runVerifyArtifact(args)
	case "self-test":
		return runSelfTest(ctx, args)
	case "uninstall-local":
		return runUninstallLocal(ctx, args)
	default:
		return fmt.Errorf("unknown connector command %q", command)
	}
}

func runRepair(ctx context.Context, logger *slog.Logger, args []string) error {
	flags := flag.NewFlagSet("argus-connector repair", flag.ContinueOnError)
	server := flags.String("server", os.Getenv("ARGUS_CONNECTOR_ENROLLMENT_URL"), "Argus enrollment API URL")
	token := flags.String("token", os.Getenv("ARGUS_CONNECTOR_ENROLLMENT_TOKEN"), "one-time PKI repair token")
	caFile := flags.String("ca-file", os.Getenv("ARGUS_CONNECTOR_CA_FILE"), "current Argus Trust Bundle file")
	dataDirectory := flags.String("data-dir", defaultDataDirectory(), "Connector identity directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *server == "" || *token == "" || *caFile == "" {
		return errors.New("--server, --token, and --ca-file are required")
	}
	store := localStore{directory: *dataDirectory}
	current, err := store.loadIdentity()
	if err != nil {
		return fmt.Errorf("load existing Connector identity: %w", err)
	}
	result, err := enroll(ctx, enrollOptions{Server: *server, ConnectorID: current.ConnectorID, Token: *token, Role: current.Role,
		Name: current.Name, DataDirectory: *dataDirectory, CAFile: *caFile, InstanceID: current.InstanceID,
		Capabilities: append([]string(nil), current.Capabilities...)})
	if err != nil {
		return err
	}
	if os.Getenv("ARGUS_CONNECTOR_KUBERNETES_STATE") == "1" {
		mirror, mirrorErr := newKubernetesStateMirror(os.Getenv("ARGUS_CONNECTOR_KUBERNETES_NAMESPACE"))
		if mirrorErr != nil {
			return mirrorErr
		}
		store.mirror = mirror.sync
		if mirrorErr = store.syncMirror(); mirrorErr != nil {
			return fmt.Errorf("persist repaired Kubernetes Connector identity: %w", mirrorErr)
		}
	}
	logger.Info("Connector identity repaired", "connector_id", result.ConnectorID, "role", result.Role, "gateway", result.GatewayEndpoint)
	return nil
}

func runBootstrapState(args []string) error {
	flags := flag.NewFlagSet("argus-connector bootstrap-state", flag.ContinueOnError)
	sourceDirectory := flags.String("source", "", "Read-only bootstrap identity directory")
	dataDirectory := flags.String("data-dir", defaultDataDirectory(), "Writable Connector identity directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *sourceDirectory == "" || filepath.Clean(*sourceDirectory) == filepath.Clean(*dataDirectory) {
		return errors.New("distinct --source and --data-dir directories are required")
	}
	source := localStore{directory: *sourceDirectory}
	identity, err := source.loadIdentity()
	if err != nil {
		return fmt.Errorf("load bootstrap Connector identity: %w", err)
	}
	certificate, privateKey, caBundle, err := source.identityMaterial()
	if err != nil {
		return fmt.Errorf("load bootstrap Connector material: %w", err)
	}
	return (localStore{directory: *dataDirectory}).saveIdentity(identity, privateKey, certificate, caBundle)
}

func runProbe(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("argus-connector probe", flag.ContinueOnError)
	dataDirectory := flags.String("data-dir", defaultDataDirectory(), "Connector identity directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	return probeGateway(ctx, localStore{directory: *dataDirectory})
}

func runEnroll(ctx context.Context, logger *slog.Logger, args []string) error {
	flags := flag.NewFlagSet("argus-connector enroll", flag.ContinueOnError)
	server := flags.String("server", os.Getenv("ARGUS_CONNECTOR_ENROLLMENT_URL"), "Argus enrollment API URL")
	caFile := flags.String("ca-file", os.Getenv("ARGUS_CONNECTOR_CA_FILE"), "current Argus Trust Bundle file")
	connectorID := flags.String("connector-id", "", "preallocated Connector ID")
	token := flags.String("token", os.Getenv("ARGUS_CONNECTOR_ENROLLMENT_TOKEN"), "one-time enrollment token")
	tokenFile := flags.String("token-file", "", "file containing the one-time enrollment token")
	role := flags.String("role", "host", "Connector role: host, bastion, or kubernetes")
	name := flags.String("name", hostname(), "Connector display name")
	dataDirectory := flags.String("data-dir", defaultDataDirectory(), "Connector identity directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *tokenFile != "" {
		value, readErr := os.ReadFile(*tokenFile)
		if readErr != nil {
			return errors.New("read Connector enrollment token file")
		}
		*token = strings.TrimSpace(string(value))
		clear(value)
	}
	if *server == "" || *caFile == "" || *connectorID == "" || *token == "" || (*role != "host" && *role != "bastion" && *role != "kubernetes") {
		return errors.New("--server, --ca-file, --connector-id, --token, and a valid --role are required")
	}
	result, err := enroll(ctx, enrollOptions{Server: *server, ConnectorID: *connectorID, Token: *token, Role: *role,
		Name: *name, DataDirectory: *dataDirectory, CAFile: *caFile})
	if err != nil {
		return err
	}
	logger.Info("Connector enrolled", "connector_id", result.ConnectorID, "role", result.Role, "gateway", result.GatewayEndpoint)
	return nil
}

func runConnector(ctx context.Context, logger *slog.Logger, args []string) error {
	flags := flag.NewFlagSet("argus-connector run", flag.ContinueOnError)
	dataDirectory := flags.String("data-dir", defaultDataDirectory(), "Connector identity directory")
	if err := flags.Parse(args); err != nil {
		return err
	}
	store := localStore{directory: *dataDirectory}
	if os.Getenv("ARGUS_CONNECTOR_KUBERNETES_STATE") == "1" {
		mirror, err := newKubernetesStateMirror(os.Getenv("ARGUS_CONNECTOR_KUBERNETES_NAMESPACE"))
		if err != nil {
			return err
		}
		store.mirror = mirror.sync
		if err = store.syncMirror(); err != nil {
			return fmt.Errorf("persist Kubernetes Connector identity: %w", err)
		}
	}
	identity, err := store.loadIdentity()
	if err != nil {
		return fmt.Errorf("load Connector identity: %w", err)
	}
	var relay *bastionRelay
	if identity.Role == "bastion" {
		relay, err = startBastionRelay(ctx, store, identity, logger)
		if err != nil {
			return err
		}
		defer relay.Close()
		status := relay.snapshot()
		logger.Info("Bastion TLS relay initialized", "status", status.GetStatus(), "address", status.GetAdvertiseAddress(),
			"generation", status.GetGeneration(), "listeners", status.GetListeners(), "error_code", status.GetErrorCode())
	}
	return (connectorClient{store: store, logger: logger, relay: relay}).run(ctx)
}

func hostname() string {
	value, err := os.Hostname()
	if err != nil || value == "" {
		return "argus-connector"
	}
	return filepath.Base(value)
}

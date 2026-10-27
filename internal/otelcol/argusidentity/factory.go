package argusidentity

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"strconv"
	"strings"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
)

var componentType = component.MustNewType("argus_identity")

type Config struct {
	CollectorID                      string        `mapstructure:"collector_id"`
	EnrollmentEndpoint               string        `mapstructure:"enrollment_endpoint"`
	RotationEndpoint                 string        `mapstructure:"rotation_endpoint"`
	TrustBundleEndpoint              string        `mapstructure:"trust_bundle_endpoint"`
	ApplicationAuthorizationEndpoint string        `mapstructure:"application_authorization_endpoint"`
	DialAddress                      string        `mapstructure:"dial_address"`
	EnrollmentTokenFile              string        `mapstructure:"enrollment_token_file"`
	CertificateFile                  string        `mapstructure:"certificate_file"`
	PrivateKeyFile                   string        `mapstructure:"private_key_file"`
	ServerCertificateFile            string        `mapstructure:"server_certificate_file"`
	ServerPrivateKeyFile             string        `mapstructure:"server_private_key_file"`
	CABundleFile                     string        `mapstructure:"ca_bundle_file"`
	ServerCAFile                     string        `mapstructure:"server_ca_file"`
	TrustBundleStateFile             string        `mapstructure:"trust_bundle_state_file"`
	BootstrapIdentityDir             string        `mapstructure:"bootstrap_identity_directory"`
	RotateBefore                     time.Duration `mapstructure:"rotate_before"`
	CheckInterval                    time.Duration `mapstructure:"check_interval"`
	DeliveryEndpoint                 string        `mapstructure:"delivery_endpoint"`
	DeliveryStateFile                string        `mapstructure:"delivery_state_file"`
	DeliveryInterval                 time.Duration `mapstructure:"delivery_interval"`
}

func (config *Config) Validate() error {
	if config.CollectorID == "" || config.EnrollmentEndpoint == "" || config.RotationEndpoint == "" || config.TrustBundleEndpoint == "" ||
		config.EnrollmentTokenFile == "" || config.CertificateFile == "" || config.PrivateKeyFile == "" || config.ServerCertificateFile == "" ||
		config.ServerPrivateKeyFile == "" || config.CABundleFile == "" || config.TrustBundleStateFile == "" {
		return errors.New("argus_identity requires collector, endpoints, token, certificate, key, and CA paths")
	}
	if config.RotateBefore <= 0 || config.CheckInterval <= 0 || config.RotateBefore < config.CheckInterval {
		return errors.New("argus_identity rotation intervals are invalid")
	}
	if config.DeliveryEndpoint != "" {
		delivery, deliveryErr := url.Parse(config.DeliveryEndpoint)
		rotation, rotationErr := url.Parse(config.RotationEndpoint)
		if deliveryErr != nil || rotationErr != nil || delivery.Scheme != "https" || delivery.Host == "" || delivery.Host != rotation.Host || delivery.User != nil || delivery.Fragment != "" || delivery.RawQuery != "" || delivery.ForceQuery || delivery.Path != strings.TrimSuffix(rotation.Path, "/v1/identity/rotate")+"/v1/collector-delivery" {
			return errors.New("collector delivery endpoint must use its trusted identity HTTPS authority")
		}
	}
	if config.DeliveryEndpoint != "" && (config.DeliveryStateFile == "" || config.DeliveryInterval < time.Second || config.DeliveryInterval > time.Minute) {
		return errors.New("collector delivery reporting configuration is invalid")
	}
	if config.DialAddress != "" && !validIdentityDialAddress(config.DialAddress) {
		return errors.New("argus_identity dial address must be a loopback endpoint")
	}
	if config.BootstrapIdentityDir != "" && strings.TrimSpace(config.BootstrapIdentityDir) != config.BootstrapIdentityDir {
		return errors.New("argus_identity bootstrap identity directory is invalid")
	}
	return nil
}

func validIdentityDialAddress(value string) bool {
	host, rawPort, err := net.SplitHostPort(strings.TrimSpace(value))
	if err != nil {
		return false
	}
	address, err := netip.ParseAddr(host)
	if err != nil || !address.IsLoopback() {
		return false
	}
	port, err := strconv.Atoi(rawPort)
	return err == nil && port >= 1 && port <= 65535
}

func NewFactory() extension.Factory {
	return extension.NewFactory(componentType, createDefaultConfig, createExtension, component.StabilityLevelBeta)
}

func createDefaultConfig() component.Config {
	return &Config{
		EnrollmentTokenFile:   "/var/lib/argus-otelcol/identity/enrollment-token",
		CertificateFile:       "/var/lib/argus-otelcol/identity/client.pem",
		PrivateKeyFile:        "/var/lib/argus-otelcol/identity/client-key.pem",
		ServerCertificateFile: "/var/lib/argus-otelcol/identity/server.pem",
		ServerPrivateKeyFile:  "/var/lib/argus-otelcol/identity/server-key.pem",
		CABundleFile:          "/var/lib/argus-otelcol/identity/ca.pem",
		TrustBundleStateFile:  "/var/lib/argus-otelcol/identity/trust-bundle.json",
		RotateBefore:          8 * time.Hour,
		CheckInterval:         5 * time.Minute,
		DeliveryInterval:      15 * time.Second,
	}
}

func createExtension(_ context.Context, settings extension.Settings, raw component.Config) (extension.Extension, error) {
	config := raw.(*Config)
	return newIdentityExtension(*config, settings.Logger), nil
}

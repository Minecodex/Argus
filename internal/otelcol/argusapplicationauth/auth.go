package argusapplicationauth

import (
	"context"
	"errors"
	"slices"
	"strings"

	"go.opentelemetry.io/collector/client"
	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/extension"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type Config struct {
	Protocol          string `mapstructure:"protocol"`
	IdentityExtension string `mapstructure:"identity_extension"`
}

func (c *Config) Validate() error {
	if !slices.Contains([]string{"otlp", "jaeger", "skywalking"}, c.Protocol) || c.IdentityExtension == "" {
		return errors.New("application authentication configuration invalid")
	}
	return nil
}

type authorizer interface {
	AuthorizeApplication(context.Context, string, string) (map[string]string, error)
}
type authentication struct {
	config    Config
	authority authorizer
}
type attributes map[string]string

func (a attributes) GetAttribute(key string) any { return a[key] }
func (a attributes) GetAttributeNames() []string {
	result := []string{}
	for key := range a {
		result = append(result, key)
	}
	return result
}

func NewFactory() extension.Factory {
	return extension.NewFactory(component.MustNewType("argus_application_auth"), func() component.Config { return &Config{IdentityExtension: "argus_identity"} }, func(_ context.Context, _ extension.Settings, c component.Config) (extension.Extension, error) {
		return &authentication{config: *c.(*Config)}, nil
	}, component.StabilityLevelBeta)
}
func (a *authentication) Start(_ context.Context, host component.Host) error {
	var id component.ID
	if err := id.UnmarshalText([]byte(a.config.IdentityExtension)); err != nil {
		return err
	}
	value, ok := host.GetExtensions()[id].(authorizer)
	if !ok {
		return errors.New("Collector application identity authority unavailable")
	}
	a.authority = value
	return nil
}
func (*authentication) Shutdown(context.Context) error { return nil }
func (a *authentication) Authenticate(ctx context.Context, headers map[string][]string) (context.Context, error) {
	rejected := errors.New("application ingestion authentication rejected")
	values := []string{}
	for key, items := range headers {
		if strings.EqualFold(key, "authorization") {
			for _, item := range items {
				if !strings.HasPrefix(item, "Bearer ") {
					return nil, rejected
				}
				values = append(values, strings.TrimPrefix(item, "Bearer "))
			}
		}
		if strings.EqualFold(key, "authentication") && a.config.Protocol == "skywalking" {
			values = append(values, items...)
		}
	}
	if len(values) != 1 || len(values[0]) > 256 || !strings.HasPrefix(values[0], "argus_app.") || a.authority == nil {
		return nil, rejected
	}
	result, err := a.authority.AuthorizeApplication(ctx, a.config.Protocol, values[0])
	if err != nil {
		if code := status.Code(err); code == codes.Unavailable || code == codes.ResourceExhausted {
			// HTTP authentication errors become 401 upstream. Defer a transient
			// failure to the mandatory identity processor, which rejects payloads
			// retryably and cannot produce an authorized source from this marker.
			info := client.FromContext(ctx)
			info.Auth = attributes{"transient_error": "application_auth_unavailable"}
			return client.NewContext(ctx, info), nil
		}
		return nil, rejected
	}
	for _, key := range []string{"key_id", "endpoint_id", "endpoint_version", "signals"} {
		if result[key] == "" {
			return nil, rejected
		}
	}
	info := client.FromContext(ctx)
	info.Auth = attributes(result)
	return client.NewContext(ctx, info), nil
}

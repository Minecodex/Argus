package kubernetesreader

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	connectorv1 "github.com/kakj-go/Argus/internal/gen/proto/argus/connector/v1"
	"github.com/kakj-go/Argus/internal/kuberneteshelm"
	"github.com/kakj-go/Argus/internal/kubernetesworkbench"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (r Reader) HelmReleases(ctx context.Context, cluster db.KubernetesCluster, request kuberneteshelm.Request) ([]kuberneteshelm.Release, error) {
	var releases []kuberneteshelm.Release
	_, err := r.helm(ctx, cluster, request, "query", &releases)
	return releases, err
}
func (r Reader) PreviewHelm(ctx context.Context, c db.KubernetesCluster, q kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	return r.helm(ctx, c, q, "preview", nil)
}
func (r Reader) ExecuteHelm(ctx context.Context, c db.KubernetesCluster, q kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	return r.helm(ctx, c, q, "apply", nil)
}
func (r Reader) ObserveHelm(ctx context.Context, c db.KubernetesCluster, q kuberneteshelm.Request) (kuberneteshelm.Result, error) {
	return r.helm(ctx, c, q, "observe", nil)
}

func (r Reader) helm(ctx context.Context, c db.KubernetesCluster, q kuberneteshelm.Request, mode string, releases *[]kuberneteshelm.Release) (kuberneteshelm.Result, error) {
	var out kuberneteshelm.Result
	if q.APIServer != c.ApiServer {
		return out, kuberneteshelm.ErrInvalid
	}
	id, err := uuid.Parse(q.OperationID)
	if err != nil {
		return out, kuberneteshelm.ErrInvalid
	}
	domain := kubernetesworkbench.Service{Store: r.Store, Backend: r}
	if mode == "apply" || mode == "observe" {
		if err = domain.ValidateHelmDelivery(ctx, c.EnterpriseID, c.ID, id, q, mode == "apply"); err != nil {
			return out, err
		}
	}
	if c.ConnectionMode != "direct" {
		raw, _ := json.Marshal(q)
		payload := &connectorv1.KubernetesHelmCommand{SchemaVersion: kuberneteshelm.SchemaVersion, ClusterId: c.ID.String(), OperationId: q.OperationID, Mode: mode, RequestJson: raw}
		var response connectorv1.KubernetesHelmResult
		if err = r.connectorCommand(ctx, c, "kubernetes_helm", payload, &response); err != nil {
			if mode == "apply" {
				return out, kuberneteshelm.ErrUnknown
			}
			return out, err
		}
		if response.SchemaVersion != kuberneteshelm.SchemaVersion || len(response.ResultJson) > 768<<10 {
			return out, kuberneteshelm.ErrBudget
		}
		if err = kuberneteshelm.ErrorFromCode(response.ErrorCode); err != nil {
			return out, err
		}
		if mode == "query" {
			err = json.Unmarshal(response.ResultJson, releases)
		} else {
			err = json.Unmarshal(response.ResultJson, &out)
		}
		if err != nil {
			return out, kuberneteshelm.ErrInvalid
		}
		if mode == "query" {
			return out, kuberneteshelm.ValidateReleases(*releases, q)
		}
		if out.SchemaVersion != kuberneteshelm.SchemaVersion || len(out.Objects) > kuberneteshelm.MaxObjects || len(out.Writes) > 1000 || mode != "preview" && (len(out.SealedPlan) > 0 || len(out.EncryptedChart) > 0) || out.Release.Namespace != "" && out.Release.Namespace != q.Namespace.Name || out.Release.Name != "" && out.Release.Name != q.Name {
			return out, kuberneteshelm.ErrInvalid
		}
		return out, nil
	}
	config, _, cleanup, err := r.directConfigTTL(ctx, c, max(time.Minute, time.Duration(q.TimeoutSeconds+20)*time.Second))
	if err != nil {
		return out, err
	}
	defer cleanup()
	// Keep DNS revalidation on all SDK-created clients, including discovery/storage.
	target, addresses, err := r.Validator.ResolveHTTPS(ctx, c.ApiServer)
	if err != nil {
		return out, err
	}
	config.Wrap(func(base http.RoundTripper) http.RoundTripper {
		return revalidatingTransport{base: base, validator: r.Validator, host: target.Hostname(), addresses: addresses}
	})
	engine := kuberneteshelm.Engine{Config: config, SourcePolicy: r.Validator, AuthorizeWrite: func(call context.Context, request kuberneteshelm.Request, w kuberneteshelm.Write) error {
		return domain.ValidateHelmWrite(call, c.EnterpriseID, c.ID, id, request, w)
	}}
	if mode == "query" {
		*releases, err = engine.Releases(ctx, q)
		return out, err
	}
	recipient := "worker"
	if mode == "preview" {
		recipient = "server"
	}
	material, err := r.Secrets.LoadHelmInput(ctx, c.EnterpriseID, c.ID, q, mode == "preview", recipient, c.ID.String())
	if err != nil {
		return out, err
	}
	defer material.Clear()
	switch mode {
	case "preview":
		return engine.Preview(ctx, q, material)
	case "apply":
		return engine.Execute(ctx, q, material)
	case "observe":
		return engine.Observe(ctx, q, material)
	default:
		return out, kuberneteshelm.ErrInvalid
	}
}

package dashboard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

const TemplateRuntimeVersion = 1

var templateKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_.-]{0,119}$`)

type TemplateReference struct {
	Title       string `json:"title"`
	URL         string `json:"url"`
	DashboardID string `json:"grafana_dashboard_id"`
}
type TemplateDefinition struct {
	Key             string              `json:"key"`
	Version         int64               `json:"version"`
	Name            string              `json:"name"`
	Description     string              `json:"description"`
	IntegrationKeys []string            `json:"integration_keys"`
	Spec            Spec                `json:"spec"`
	References      []TemplateReference `json:"references"`
}

func validateTemplate(definition TemplateDefinition) ([]byte, string, error) {
	if !templateKeyPattern.MatchString(definition.Key) || definition.Version < 1 || len(definition.Name) == 0 || len(definition.Name) > 120 || len(definition.Description) > 8000 || len(definition.IntegrationKeys) == 0 {
		return nil, "", ErrInvalid
	}
	for _, key := range definition.IntegrationKeys {
		if _, ok := builtincollection.Lookup(key); ok {
			continue
		}
		entry, exists := monitoringcatalog.Lookup(key)
		if !exists || entry.Selection != "candidate" {
			return nil, "", ErrInvalid
		}
	}
	raw, err := json.Marshal(definition.Spec)
	if err != nil {
		return nil, "", ErrInvalid
	}
	if _, err = DecodeSpec(raw); err != nil {
		return nil, "", err
	}
	for _, panel := range definition.Spec.Panels {
		for _, target := range append(slices.Clone(panel.Targets), panel.DetailQueryTargets...) {
			if _, err := compileConcreteTarget(target, false); err != nil {
				return nil, "", err
			}
		}
	}
	content, _ := json.Marshal(definition)
	hash := sha256.Sum256(content)
	return raw, hex.EncodeToString(hash[:]), nil
}

// SyncTemplates installs immutable definitions and advances heads monotonically.
// An older server cannot replace a newer head during a rolling deployment.
func (s Service) SyncTemplates(ctx context.Context, definitions []TemplateDefinition) error {
	if s.Store == nil {
		return ErrUnavailable
	}
	for _, definition := range definitions {
		raw, hash, err := validateTemplate(definition)
		if err != nil {
			return err
		}
		references, _ := json.Marshal(definition.References)
		if err = s.Store.InTx(ctx, func(q *db.Queries) error {
			if err := q.InsertDashboardTemplateVersion(ctx, db.InsertDashboardTemplateVersionParams{TemplateKey: definition.Key, Version: definition.Version, RuntimeVersion: TemplateRuntimeVersion, Name: definition.Name, Description: definition.Description, IntegrationKeys: definition.IntegrationKeys, Spec: raw, SpecHash: hash, ReferencesJson: references}); err != nil {
				return err
			}
			row, err := q.GetDashboardTemplateVersion(ctx, db.GetDashboardTemplateVersionParams{TemplateKey: definition.Key, Version: definition.Version})
			if err != nil {
				return err
			}
			if row.SpecHash != hash {
				return ErrConflict
			}
			return q.PromoteDashboardTemplateHead(ctx, db.PromoteDashboardTemplateHeadParams{TemplateKey: definition.Key, Version: definition.Version})
		}); err != nil {
			return err
		}
	}
	return nil
}

func templateDashboardID(enterprise uuid.UUID, key string) uuid.UUID {
	return uuid.NewSHA1(enterprise, []byte("argus-managed-dashboard/"+key))
}

// ApplyManagedTemplate changes only a managed definition. Scope, samples and
// user copies remain independent; no runtime Actor receives a data grant here.
func (s Service) ApplyManagedTemplate(ctx context.Context, enterprise uuid.UUID, key string) (db.Dashboard, error) {
	var result db.Dashboard
	if s.Store == nil || enterprise == uuid.Nil || !templateKeyPattern.MatchString(key) {
		return result, ErrInvalid
	}
	err := s.Store.InTx(ctx, func(q *db.Queries) error {
		if err := q.LockManagedDashboardTemplate(ctx, db.LockManagedDashboardTemplateParams{EnterpriseID: enterprise, TemplateKey: key}); err != nil {
			return err
		}
		template, err := q.GetDashboardTemplateHead(ctx, key)
		if err != nil {
			return err
		}
		if template.RuntimeVersion != TemplateRuntimeVersion {
			return ErrUnavailable
		}
		if _, err = DecodeSpec(template.Spec); err != nil {
			return err
		}
		id := templateDashboardID(enterprise, key)
		instance, err := q.GetManagedDashboardInstance(ctx, db.GetManagedDashboardInstanceParams{EnterpriseID: enterprise, TemplateKey: key})
		newInstance := errors.Is(err, pgx.ErrNoRows)
		if err != nil && !newInstance {
			return err
		}
		if err = q.AllowManagedDashboardWrite(ctx, id); err != nil {
			return err
		}
		if newInstance {
			// Stable IDs may not adopt a user-created object on collision.
			if _, e := q.GetDashboard(ctx, db.GetDashboardParams{ID: id, EnterpriseID: enterprise}); !errors.Is(e, pgx.ErrNoRows) {
				if e != nil {
					return e
				}
				return ErrConflict
			}
			result, err = q.CreateDashboard(ctx, db.CreateDashboardParams{ID: id, EnterpriseID: enterprise, Name: template.Name, Description: template.Description})
		} else {
			if instance.DashboardID != id {
				return ErrConflict
			}
			result, err = q.LockDashboard(ctx, db.LockDashboardParams{ID: id, EnterpriseID: enterprise})
		}
		if err != nil {
			return err
		}
		if !newInstance && instance.AppliedVersion == template.Version {
			return nil
		}
		if !newInstance && instance.AppliedVersion > template.Version {
			return ErrConflict
		}
		number := int64(1)
		if result.ActiveRevisionID.Valid {
			current, e := q.GetDashboardRevision(ctx, db.GetDashboardRevisionParams{ID: result.ActiveRevisionID.UUID, EnterpriseID: enterprise})
			if e != nil {
				return e
			}
			number = current.RevisionNumber + 1
		}
		revisionID := uuid.NewSHA1(id, []byte(template.SpecHash))
		specHash := sha256.Sum256(template.Spec)
		revision, err := q.CreateDashboardRevision(ctx, db.CreateDashboardRevisionParams{ID: revisionID, EnterpriseID: enterprise, DashboardID: id, RevisionNumber: number, SchemaVersion: SchemaVersion, Name: template.Name, Description: template.Description, Spec: template.Spec, SpecHash: hex.EncodeToString(specHash[:]), ValidationReport: json.RawMessage(`{"valid":true,"basis":"locked_system_definition","sample_verified":false}`), SampleReport: json.RawMessage(`{"status":"not_run","basis":"template_installation_does_not_generate_samples"}`)})
		if err != nil {
			return err
		}
		result, err = q.ActivateDashboardRevision(ctx, db.ActivateDashboardRevisionParams{ID: id, EnterpriseID: enterprise, ActiveRevisionID: nullID(revision.ID), Name: template.Name, Description: template.Description, Version: result.Version})
		if err != nil {
			return err
		}
		if err = q.PutManagedDashboardInstance(ctx, db.PutManagedDashboardInstanceParams{EnterpriseID: enterprise, TemplateKey: key, DashboardID: id, AppliedVersion: template.Version}); err != nil {
			return err
		}
		return appendDashboardAudit(ctx, q, Actor{EnterpriseID: enterprise, SubjectType: "system"}, "dashboard.template.applied", "dashboard", id, template.Name, "success", map[string]any{"template_key": key, "template_version": template.Version, "template_hash": template.SpecHash, "revision_id": revision.ID})
	})
	return result, err
}

package monitoring

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/kakj-go/Argus/internal/authorization"
	"github.com/kakj-go/Argus/internal/builtincollection"
	"github.com/kakj-go/Argus/internal/logcollection"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func (s Service) prepareCollection(ctx context.Context, q *db.Queries, a Actor, m Mutation, lock bool) (actionPlan, error) {
	p := actionPlan{SchemaVersion: "argus.monitoring_action/v1", Mutation: m, Actor: a, References: map[string]int64{}}
	permission := "monitoring.manage"
	if m.Collection != nil {
		if _, ok := builtincollection.Lookup(m.Collection.Integration); ok {
			permission = "telemetry.collector.manage"
		}
	}
	if err := authorization.Authorize(ctx, q, a, permission); err != nil {
		return p, err
	}
	if m.Application != nil || m.Binding != nil || m.Collection == nil || m.ID == uuid.Nil {
		return p, ErrInvalid
	}
	input := *m.Collection
	in := &input
	var current db.MonitoringCollection
	if m.Operation != "create" {
		var err error
		if lock {
			current, err = q.LockMonitoringCollection(ctx, db.LockMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		} else {
			current, err = q.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		}
		if err != nil {
			return p, lookupError(err)
		}
		if current.Version != m.ExpectedVersion || current.Status != "active" {
			return p, ErrConflict
		}
		if err = requireLogCollection(ctx, q, a, current); err != nil {
			return p, err
		}
		p.References["collection:"+current.ID.String()] = current.Version
		if m.Operation == "set_enabled" || m.Operation == "archive" {
			enabled := in.Enabled && m.Operation != "archive"
			in = &CollectionInput{Name: current.Name, Integration: current.IntegrationKey, Target: ResourceRef{Kind: current.TargetType, ID: current.TargetID}, CollectorID: current.CollectorID.UUID, ApplicationID: current.ApplicationID.UUID, ServiceID: current.ServiceID.UUID, Config: current.Config, Enabled: enabled, CredentialUpdate: "keep"}
		} else if m.Operation != "update" {
			return p, ErrInvalid
		}
		if in.Integration != current.IntegrationKey || in.Target.Kind != current.TargetType || in.Target.ID != current.TargetID {
			return p, ErrInvalid
		}
	}
	if _, ok := builtincollection.Lookup(in.Integration); ok {
		p.Mutation.Collection = in
		return s.prepareBuiltinCollection(ctx, q, p, current)
	}
	if logcollection.IsIntegration(in.Integration) {
		p.Mutation.Collection = in
		return s.prepareLogCollection(ctx, q, p, current, lock)
	}
	entry, exists := monitoringcatalog.Lookup(in.Integration)
	if !exists || entry.Selection != "candidate" || !entry.AdapterReady {
		return p, ErrUnsupported
	}
	if !validName(in.Name, 200) || in.CollectorID == uuid.Nil || len(in.Bindings) > 100 {
		return p, ErrInvalid
	}
	identity := in.TargetIdentity
	if in.Target.ID == uuid.Nil {
		if m.Operation != "create" || in.Target.Kind != "" || !validName(in.TargetName, 200) {
			return p, ErrInvalid
		}
	} else {
		if in.Target.Kind != "monitoring_target" {
			return p, ErrInvalid
		}
		version, err := requireTarget(ctx, q, a, in.Target)
		if err != nil {
			return p, err
		}
		target, err := q.GetMonitoringTarget(ctx, db.GetMonitoringTargetParams{EnterpriseID: a.EnterpriseID, ID: in.Target.ID})
		if err != nil {
			return p, err
		}
		if target.Engine != entry.Key || target.Kind != entry.Family || len(in.TargetIdentity) > 0 || in.TargetName != "" {
			return p, ErrInvalid
		}
		identity = target.IdentityConfig
		p.References["monitoring_target:"+target.ID.String()] = version
		if m.Operation == "create" {
			_, err = q.GetMonitoringCollectionForTarget(ctx, db.GetMonitoringCollectionForTargetParams{EnterpriseID: a.EnterpriseID, TargetType: in.Target.Kind, TargetID: in.Target.ID, IntegrationKey: in.Integration})
			if err == nil {
				return p, ErrConflict
			}
			if !errors.Is(err, pgx.ErrNoRows) {
				return p, err
			}
		}
	}
	_, config, err := monitoringcatalog.Parse(in.Integration, identity, in.Config)
	if err != nil {
		return p, ErrInvalid
	}
	in.Config, _ = json.Marshal(config)
	if err = authorization.Authorize(ctx, q, a, "collector.use"); err != nil {
		return p, err
	}
	collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: a.EnterpriseID, ID: in.CollectorID})
	if err != nil {
		return p, lookupError(err)
	}
	if collector.Status == "uninstalled" || collector.Status == "uninstalling" || !slices.Contains(entry.Platforms, collector.Platform) {
		return p, ErrUnsupported
	}
	if err = authorization.RequireResource(ctx, q, a, collector.ResourceType, collector.ResourceID); err != nil {
		return p, err
	}
	distribution, err := q.GetMonitoringDistribution(ctx, collector.DistributionVersionID)
	if err != nil {
		return p, err
	}
	if distribution.SupportStatus != "supported" || distribution.CollectorVersion != monitoringcatalog.CollectorVersion || !slices.Contains(distribution.Components, entry.Component) {
		return p, ErrUnsupported
	}
	p.References["collector:"+collector.ID.String()] = collector.Version
	p.References["distribution:"+distribution.ID.String()] = int64(distribution.CatalogRevision)
	if in.ApplicationID != uuid.Nil {
		if err = s.collectionApplication(ctx, q, a, in.ApplicationID, in.ServiceID, p.References); err != nil {
			return p, err
		}
	} else if in.ServiceID != uuid.Nil {
		return p, ErrInvalid
	}
	for _, binding := range in.Bindings {
		if m.Operation != "create" || binding.Target.ID != uuid.Nil || binding.Target.Kind != "" {
			return p, ErrInvalid
		}
		if err = s.collectionApplication(ctx, q, a, binding.ApplicationID, binding.ServiceID, p.References); err != nil {
			return p, err
		}
	}
	if in.CredentialUpdate == "" {
		in.CredentialUpdate = "keep"
	}
	hasCredential := current.CredentialID.Valid
	switch in.CredentialUpdate {
	case "keep":
		if in.CredentialDraftID != uuid.Nil {
			return p, ErrCredential
		}
	case "remove":
		if in.CredentialDraftID != uuid.Nil {
			return p, ErrCredential
		}
		hasCredential = false
	case "replace":
		draft, err := q.LockMonitoringCredentialDraft(ctx, db.LockMonitoringCredentialDraftParams{EnterpriseID: a.EnterpriseID, ID: in.CredentialDraftID})
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return p, ErrCredential
			}
			return p, err
		}
		hash, err := credentialTargetHash(in)
		if err != nil || draft.EditorID != a.SubjectID || draft.EditorType != a.SubjectType || draft.CollectorID != in.CollectorID || draft.ConsumedAt.Valid || !s.now().Before(draft.ExpiresAt.Time) || !bytes.Equal(hash, draft.TargetHash) {
			return p, ErrCredential
		}
		p.References["credential_draft:"+draft.ID.String()] = 1
		hasCredential = true
	default:
		return p, ErrCredential
	}
	if _, err = monitoringcatalog.ReceiverConfig(in.Integration, m.ID, identity, in.Config, hasCredential); err != nil {
		return p, ErrCredential
	}
	if current.CredentialID.Valid {
		credential, err := q.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: a.EnterpriseID, ID: current.CredentialID.UUID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return p, err
		}
		if err != nil || credential.Protocol != "monitoring" || credential.Status != "active" {
			return p, ErrCredential
		}
		p.References["credential:"+credential.ID.String()] = credential.Version
	}
	p.Mutation.Collection = in
	if s.Runtime == nil {
		return p, ErrUnavailable
	}
	collectors := []uuid.UUID{in.CollectorID}
	if current.CollectorID.Valid && current.CollectorID.UUID != in.CollectorID {
		collectors = append(collectors, current.CollectorID.UUID)
	}
	for _, id := range collectors {
		raw, err := s.Runtime.Plan(ctx, q, a.EnterpriseID, id)
		if err != nil {
			return p, err
		}
		digest := sha256.Sum256(raw)
		p.References["runtime:"+hex.EncodeToString(digest[:])] = 1
		// Freeze both routes for confirmation, but configure the new executor
		// first. Retirement of the old effective executor follows its success.
		if id == in.CollectorID {
			p.RuntimePlans = append(p.RuntimePlans, raw)
		}
	}
	return p, nil
}

func (s Service) collectionApplication(ctx context.Context, q *db.Queries, a Actor, appID, serviceID uuid.UUID, refs map[string]int64) error {
	if err := authorization.Authorize(ctx, q, a, "application.manage"); err != nil {
		return err
	}
	if err := authorization.RequireResource(ctx, q, a, "application", appID); err != nil {
		return err
	}
	app, err := q.GetMonitoringApplication(ctx, db.GetMonitoringApplicationParams{EnterpriseID: a.EnterpriseID, ID: appID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err != nil || app.Status != "active" {
		return ErrInvalid
	}
	refs["application:"+app.ID.String()] = app.Version
	if serviceID != uuid.Nil {
		service, err := q.GetMonitoringService(ctx, db.GetMonitoringServiceParams{EnterpriseID: a.EnterpriseID, ID: serviceID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		if err != nil || service.ApplicationID != app.ID || service.Status != "active" {
			return ErrInvalid
		}
		refs["service:"+service.ID.String()] = service.Version
	}
	return nil
}

func (s Service) commitCollection(ctx context.Context, q *db.Queries, p actionPlan, approvedAction uuid.UUID) (resource.ActionCommitResult, error) {
	m, a, in := p.Mutation, p.Actor, p.Mutation.Collection
	result := resource.ActionCommitResult{ResourceType: "collection", ResourceID: m.ID, Summary: "Collection desired configuration saved"}
	if in == nil {
		return result, ErrInvalid
	}
	var current, row db.MonitoringCollection
	var err error
	if m.Operation != "create" {
		current, err = q.LockMonitoringCollection(ctx, db.LockMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: m.ID})
		if err != nil {
			return result, err
		}
	}
	target := in.Target
	if target.ID == uuid.Nil {
		target = ResourceRef{Kind: "monitoring_target", ID: uuid.NewSHA1(m.ID, []byte("target"))}
		entry, _ := monitoringcatalog.Lookup(in.Integration)
		identity, _, err := monitoringcatalog.Parse(in.Integration, in.TargetIdentity, in.Config)
		if err != nil {
			return result, err
		}
		raw, _ := json.Marshal(identity)
		_, err = q.CreateMonitoringTarget(ctx, db.CreateMonitoringTargetParams{ID: target.ID, EnterpriseID: a.EnterpriseID, Name: in.TargetName, Kind: entry.Family, Engine: in.Integration, IdentityConfig: raw, CreatedBy: a.SubjectID})
		if err != nil {
			return result, err
		}
		if err = authorization.GrantCreatedResource(ctx, q, a, target.Kind, target.ID); err != nil {
			return result, err
		}
	}
	credentialID := current.CredentialID
	if in.CredentialUpdate == "replace" {
		draft, err := q.LockMonitoringCredentialDraft(ctx, db.LockMonitoringCredentialDraftParams{EnterpriseID: a.EnterpriseID, ID: in.CredentialDraftID})
		if err != nil {
			return result, err
		}
		consumed, err := q.ConsumeMonitoringCredentialDraft(ctx, db.ConsumeMonitoringCredentialDraftParams{EnterpriseID: a.EnterpriseID, ID: draft.ID, EditorType: a.SubjectType, EditorID: a.SubjectID})
		if err != nil {
			return result, err
		}
		if consumed != 1 {
			return result, ErrCredential
		}
		credential, err := s.Credentials.BindMonitoringWithQueries(ctx, q, a.EnterpriseID, draft.ID, draft.SecretID, m.ID)
		if err != nil {
			return result, err
		}
		credentialID = nullID(credential.ID)
	} else if in.CredentialUpdate == "remove" || m.Operation == "archive" {
		credentialID = uuid.NullUUID{}
	}
	if m.Operation == "create" {
		version := monitoringcatalog.Version
		if _, ok := builtincollection.Lookup(in.Integration); ok {
			version = builtincollection.Version
		}
		row, err = q.CreateMonitoringCollection(ctx, db.CreateMonitoringCollectionParams{ID: m.ID, EnterpriseID: a.EnterpriseID, Name: in.Name, IntegrationKey: in.Integration, IntegrationVersion: version, TargetType: target.Kind, TargetID: target.ID, CollectorID: nullID(in.CollectorID), ApplicationID: nullID(in.ApplicationID), ServiceID: nullID(in.ServiceID), Config: in.Config, CredentialID: credentialID, DesiredEnabled: in.Enabled, CreatedBy: a.SubjectID})
	} else {
		status := "active"
		if m.Operation == "archive" {
			status = "archived"
		}
		row, err = q.UpdateMonitoringCollection(ctx, db.UpdateMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, ID: m.ID, Version: m.ExpectedVersion, Name: in.Name, Config: in.Config, CredentialID: credentialID, DesiredEnabled: in.Enabled, CollectorID: nullID(in.CollectorID), ApplicationID: nullID(in.ApplicationID), ServiceID: nullID(in.ServiceID), Status: status})
	}
	if err != nil {
		return result, err
	}
	if current.CredentialID.Valid && credentialID != current.CredentialID {
		if err = s.Credentials.RetireMonitoringWithQueries(ctx, q, a.EnterpriseID, m.ID, current.CredentialID.UUID); err != nil {
			return result, err
		}
	}
	if _, err = q.SnapshotMonitoringCollection(ctx, db.SnapshotMonitoringCollectionParams{EnterpriseID: a.EnterpriseID, CollectionID: m.ID, ActorID: a.SubjectID}); err != nil {
		return result, err
	}
	if m.Operation == "create" || current.CollectorID != row.CollectorID || m.Operation == "archive" {
		if err = q.AbortPendingMonitoringAssignments(ctx, db.AbortPendingMonitoringAssignmentsParams{EnterpriseID: a.EnterpriseID, CollectionID: m.ID}); err != nil {
			return result, err
		}
		if row.Status == "active" {
			if err = q.PrepareMonitoringAssignment(ctx, db.PrepareMonitoringAssignmentParams{EnterpriseID: a.EnterpriseID, CollectionID: m.ID, CollectorID: in.CollectorID, Epoch: row.AssignmentEpoch, ApprovedActionID: nullID(approvedAction)}); err != nil {
				return result, err
			}
		}
	}
	bindings := slices.Clone(in.Bindings)
	if m.Operation == "create" && in.ApplicationID != uuid.Nil && target.Kind != "application" {
		bindings = append(bindings, BindingInput{ApplicationID: in.ApplicationID, ServiceID: in.ServiceID})
	}
	for _, binding := range bindings {
		_, err = q.CreateMonitoringBinding(ctx, db.CreateMonitoringBindingParams{ID: uuid.New(), EnterpriseID: a.EnterpriseID, ApplicationID: binding.ApplicationID, ServiceID: nullID(binding.ServiceID), TargetType: target.Kind, TargetID: target.ID})
		if err != nil {
			return result, err
		}
	}
	payload, _ := json.Marshal(map[string]any{"schema_version": "argus.monitoring_reconcile/v1", "enterprise_id": a.EnterpriseID, "collection_id": m.ID, "revision": row.DesiredRevision, "collector_id": row.CollectorID.UUID, "previous_collector_id": current.CollectorID.UUID, "actor_id": a.SubjectID, "actor_type": a.SubjectType, "authorization_version": a.AuthorizationVersion})
	err = q.InsertOutboxEvent(ctx, db.InsertOutboxEventParams{ID: uuid.New(), Topic: "monitoring.collection.changed", AggregateType: "monitoring_collection", AggregateID: m.ID.String(), Payload: payload})
	result.ResourceVersion = row.Version
	return result, err
}

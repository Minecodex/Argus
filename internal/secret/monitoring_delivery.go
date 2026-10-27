package secret

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// MonitoringDelivery is constructed by the Collector command planner after it
// resolves the execution route. It is not an API request or a model tool input.
type MonitoringDelivery struct {
	CollectionID       uuid.UUID
	CollectionRevision int64
	AssignmentEpoch    int64
	CollectorID        uuid.UUID
	CollectorRevision  int64
	OperationRef       string
	RecipientType      string
	RecipientID        string
	TTL                time.Duration
}

func monitoringDeliveryCurrent(ctx context.Context, q *db.Queries, enterprise uuid.UUID, in MonitoringDelivery) (db.MonitoringCollection, error) {
	collection, err := q.GetMonitoringCollection(ctx, db.GetMonitoringCollectionParams{EnterpriseID: enterprise, ID: in.CollectionID})
	if err != nil {
		return db.MonitoringCollection{}, leaseReadFailure(err, ErrInvalidLease)
	}
	if collection.Status != "active" || !collection.DesiredEnabled {
		return db.MonitoringCollection{}, ErrInvalidLease
	}
	revision, err := q.GetMonitoringCollectionRevision(ctx, db.GetMonitoringCollectionRevisionParams{EnterpriseID: enterprise, CollectionID: collection.ID, Revision: in.CollectionRevision})
	if err != nil {
		return db.MonitoringCollection{}, leaseReadFailure(err, ErrInvalidLease)
	}
	if !revision.CredentialID.Valid || !revision.DesiredEnabled || revision.CollectorID.UUID != in.CollectorID || revision.AssignmentEpoch != in.AssignmentEpoch {
		return db.MonitoringCollection{}, ErrInvalidLease
	}
	desired := collection.CollectorID.UUID == in.CollectorID && collection.DesiredRevision == in.CollectionRevision && collection.AssignmentEpoch == in.AssignmentEpoch
	effective := collection.EffectiveCollectorID.UUID == in.CollectorID && collection.EffectiveRevision == in.CollectionRevision && collection.EffectiveAssignmentEpoch == in.AssignmentEpoch
	if !desired && !effective {
		return db.MonitoringCollection{}, ErrInvalidLease
	}
	if effective && !desired {
		collection.CollectorID, collection.DesiredRevision, collection.AssignmentEpoch = revision.CollectorID, revision.Revision, revision.AssignmentEpoch
		collection.CredentialID = revision.CredentialID
	}
	collector, err := q.GetCollectorInstance(ctx, db.GetCollectorInstanceParams{EnterpriseID: enterprise, ID: in.CollectorID})
	if err != nil {
		return db.MonitoringCollection{}, leaseReadFailure(err, ErrInvalidLease)
	}
	if collector.DesiredRevision != in.CollectorRevision || collector.Status == "uninstalled" || collector.Status == "uninstalling" {
		return db.MonitoringCollection{}, ErrInvalidLease
	}
	if revision.CredentialID != collection.CredentialID || revision.AssignmentEpoch != collection.AssignmentEpoch {
		return db.MonitoringCollection{}, ErrInvalidLease
	}
	credential, err := q.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: enterprise, ID: collection.CredentialID.UUID})
	if err != nil {
		return db.MonitoringCollection{}, leaseReadFailure(err, ErrCredentialUnavailable)
	}
	if credential.Status != "active" || credential.Protocol != "monitoring" || credential.Version != revision.CredentialVersion.Int64 {
		return db.MonitoringCollection{}, ErrCredentialUnavailable
	}
	return collection, nil
}

func (s Service) PrepareMonitoringLeaseWithQueries(ctx context.Context, q *db.Queries, actor string, enterprise uuid.UUID, in MonitoringDelivery) (db.CredentialLease, error) {
	if in.TTL <= 0 || in.TTL > 5*time.Minute || in.OperationRef == "" || in.RecipientID == "" || (in.RecipientType != "connector" && in.RecipientType != "direct_executor") {
		return db.CredentialLease{}, ErrInvalidLease
	}
	collection, err := monitoringDeliveryCurrent(ctx, q, enterprise, in)
	if err != nil {
		return db.CredentialLease{}, err
	}
	credential, err := q.GetCredential(ctx, db.GetCredentialParams{EnterpriseID: enterprise, ID: collection.CredentialID.UUID})
	if err != nil {
		return db.CredentialLease{}, err
	}
	record, err := q.GetSecret(ctx, db.GetSecretParams{EnterpriseID: enterprise, ID: credential.SecretID})
	if err != nil {
		return db.CredentialLease{}, leaseReadFailure(err, ErrCredentialUnavailable)
	}
	if record.Status != "active" || record.OwnerType != "monitoring_collection" || record.Purpose != "monitoring" || record.OwnerID.UUID != in.CollectionID {
		return db.CredentialLease{}, ErrCredentialUnavailable
	}
	version, err := q.GetCurrentSecretVersion(ctx, db.GetCurrentSecretVersionParams{EnterpriseID: enterprise, SecretID: record.ID})
	if err != nil {
		return db.CredentialLease{}, err
	}
	lease, err := q.CreateCredentialLease(ctx, db.CreateCredentialLeaseParams{ID: newUUID(), EnterpriseID: enterprise, CredentialID: credential.ID, SecretVersionID: version.ID, OperationRef: in.OperationRef, TargetResourceType: "monitoring_collection", TargetResourceID: in.CollectionID, RecipientType: in.RecipientType, RecipientID: in.RecipientID, Protocol: "monitoring", ExpiresAt: pgtype.Timestamptz{Time: time.Now().UTC().Add(in.TTL), Valid: true}})
	if err != nil {
		return db.CredentialLease{}, err
	}
	if err = q.CreateMonitoringCredentialDelivery(ctx, db.CreateMonitoringCredentialDeliveryParams{LeaseID: lease.ID, EnterpriseID: enterprise, CollectionID: collection.ID, CollectionRevision: collection.DesiredRevision, AssignmentEpoch: collection.AssignmentEpoch, CollectorID: in.CollectorID, CollectorRevision: in.CollectorRevision}); err != nil {
		return db.CredentialLease{}, err
	}
	if err = q.MarkSecretAccessed(ctx, db.MarkSecretAccessedParams{EnterpriseID: enterprise, ID: record.ID}); err != nil {
		return db.CredentialLease{}, err
	}
	err = appendAudit(ctx, q, actor, enterprise, "credential.use", "monitoring_collection", in.CollectionID, map[string]any{"summary": "monitoring credential delivery authorized", "collector_id": in.CollectorID, "collection_revision": in.CollectionRevision, "assignment_epoch": in.AssignmentEpoch})
	return lease, err
}

func validateMonitoringDelivery(ctx context.Context, q *db.Queries, lease db.CredentialLease, record db.GetSecretRow) error {
	if record.Purpose != "monitoring" || record.OwnerType != "monitoring_collection" || record.OwnerID.UUID != lease.TargetResourceID || lease.TargetResourceType != "monitoring_collection" {
		return ErrInvalidLease
	}
	delivery, err := q.GetMonitoringCredentialDelivery(ctx, db.GetMonitoringCredentialDeliveryParams{EnterpriseID: lease.EnterpriseID, LeaseID: lease.ID})
	if err != nil {
		return leaseReadFailure(err, ErrInvalidLease)
	}
	if delivery.CollectionID != lease.TargetResourceID {
		return ErrInvalidLease
	}
	collection, err := monitoringDeliveryCurrent(ctx, q, lease.EnterpriseID, MonitoringDelivery{CollectionID: delivery.CollectionID, CollectionRevision: delivery.CollectionRevision, AssignmentEpoch: delivery.AssignmentEpoch, CollectorID: delivery.CollectorID, CollectorRevision: delivery.CollectorRevision})
	if err != nil {
		return err
	}
	if collection.CredentialID.UUID != lease.CredentialID {
		return ErrInvalidLease
	}
	return nil
}

package secret

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/monitoringcatalog"
	"github.com/kakj-go/Argus/internal/otelcol/configbundle"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// MonitoringConfigForCollector is an internal execution-channel adapter. Its
// result is a private overlay, not the public or hash-addressed configuration.
func (s Service) MonitoringConfigForCollector(ctx context.Context, actor string, enterprise uuid.UUID, recipientType, recipientID, operation string, revision int64, raw []byte) ([]byte, error) {
	if s.Store == nil {
		return nil, ErrCredentialUnavailable
	}
	var bundle configbundle.Bundle
	if json.Unmarshal(raw, &bundle) != nil || bundle.SchemaVersion != configbundle.SchemaVersion {
		return nil, ErrInvalidLease
	}
	collector, err := uuid.Parse(bundle.CollectorID)
	if err != nil {
		return nil, ErrInvalidLease
	}
	receivers := map[string]map[string]string{}
	for _, item := range bundle.Collections {
		if !item.Enabled || !item.HasCredential {
			continue
		}
		id, err := uuid.Parse(item.ID)
		if err != nil {
			return nil, ErrInvalidLease
		}
		var lease db.CredentialLease
		err = s.Store.InTx(ctx, func(q *db.Queries) error {
			var e error
			lease, e = s.PrepareMonitoringLeaseWithQueries(ctx, q, actor, enterprise, MonitoringDelivery{CollectionID: id, CollectionRevision: item.Revision, AssignmentEpoch: item.AssignmentEpoch, CollectorID: collector, CollectorRevision: revision, OperationRef: operation, RecipientType: recipientType, RecipientID: recipientID, TTL: time.Minute})
			return e
		})
		if err != nil {
			return nil, err
		}
		material, err := s.FulfillLease(ctx, enterprise, lease.ID, recipientType, recipientID)
		if err != nil {
			return nil, err
		}
		var credentials monitoringcatalog.Credentials
		valid := monitoringcatalog.ValidateCredentials(item.Integration, material.Value) == nil && json.Unmarshal(material.Value, &credentials) == nil
		clear(material.Value)
		if !valid {
			return nil, ErrCredentialUnavailable
		}
		entry, ok := monitoringcatalog.Lookup(item.Integration)
		if !ok {
			return nil, ErrCredentialUnavailable
		}
		receivers[entry.Component+"/collection_"+strings.ReplaceAll(item.ID, "-", "")] = map[string]string{"username": credentials.Username, "password": credentials.Password}
	}
	return json.Marshal(map[string]any{"receivers": receivers})
}

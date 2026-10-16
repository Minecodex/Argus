package alerting

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

func TestPostgresChannelDefaultsRequireConfirmationAndPreservePolicies(t *testing.T) {
	f := newAlertFixture(t)
	secretID, credentialID := uuid.New(), uuid.New()
	f.exec("INSERT INTO secrets(id,enterprise_id,name,type,created_by) VALUES($1,$2,'Default channel','notification',$3)", secretID, f.a.EnterpriseID, f.a.SubjectID)
	f.exec("INSERT INTO secret_versions(id,secret_id,enterprise_id,version,key_id,key_version,wrapped_dek,wrap_nonce,nonce,ciphertext,value_hash) VALUES($1,$2,$3,1,'test',1,decode('01','hex'),decode(repeat('00',12),'hex'),decode(repeat('00',12),'hex'),decode('01','hex'),decode(repeat('00',32),'hex'))", uuid.New(), secretID, f.a.EnterpriseID)
	f.exec("INSERT INTO credentials(id,enterprise_id,name,protocol,secret_id) VALUES($1,$2,'Default channel','notification',$3)", credentialID, f.a.EnterpriseID, secretID)
	spec := ChannelSpec{Enabled: true, Provider: "feishu", CredentialID: credentialID, MentionIDs: []string{}}
	raw, _ := json.Marshal(spec)
	channel, err := f.s.Store.Queries.CreateAlertConfiguration(t.Context(), db.CreateAlertConfigurationParams{ID: uuid.New(), EnterpriseID: f.a.EnterpriseID, Kind: "channel", Name: "Tenant default", Spec: raw, CreatedBy: f.a.SubjectID})
	if err != nil {
		t.Fatal(err)
	}
	policy := f.createPolicy()
	spec.DefaultForNewPolicies = true
	raw, _ = json.Marshal(spec)
	pending, err := f.s.Preview(t.Context(), f.a, Mutation{Kind: "channel", Operation: "update", ID: channel.ID, Name: channel.Name, Spec: raw, ExpectedVersion: channel.Version}, uuid.NewString())
	if err != nil {
		t.Fatal(err)
	}
	read := func() ChannelSpec {
		t.Helper()
		row, err := f.s.Store.Queries.GetAlertConfiguration(t.Context(), db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: channel.ID})
		if err != nil {
			t.Fatal(err)
		}
		value, err := decodeSpec[ChannelSpec](row.Spec)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	if read().DefaultForNewPolicies {
		t.Fatal("preview modified channel defaults")
	}
	if _, err = f.commit(pending); err != nil {
		t.Fatal(err)
	}
	if !read().DefaultForNewPolicies {
		t.Fatal("default flag lost in serialization or commit")
	}
	row, err := f.s.Store.Queries.GetAlertConfiguration(t.Context(), db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: channel.ID})
	if err != nil || row.Version != channel.Version+1 {
		t.Fatal("channel submitted more than once", err)
	}
	unchanged, err := f.s.Store.Queries.GetAlertConfiguration(t.Context(), db.GetAlertConfigurationParams{EnterpriseID: f.a.EnterpriseID, ID: policy.ID})
	if err != nil || unchanged.Version != policy.Version || string(unchanged.Spec) != string(policy.Spec) {
		t.Fatal("channel default rewrote an existing policy", err)
	}
}

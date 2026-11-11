package pagination

import (
	"errors"
	"testing"
)

func TestIDCursorNeedsNoFabricatedTimestampAndRetainsBindings(t *testing.T) {
	signer := Signer{Key: []byte("01234567890123456789012345678901")}
	binding := Binding{Audience: "operations", EnterpriseID: "tenant", SubjectType: "user", SubjectID: "user", AuthorizationVersion: 4, FilterHash: HashFilter("scope"), Sort: "id"}
	token, err := signer.Encode(binding, Position{ID: "last-id"})
	if err != nil {
		t.Fatal(err)
	}
	position, err := signer.Decode(token, binding)
	if err != nil || position.ID != "last-id" || !position.Time.IsZero() {
		t.Fatal("ID cursor required a timestamp", position, err)
	}
	changed := binding
	changed.AuthorizationVersion++
	if _, err = signer.Decode(token, changed); !errors.Is(err, ErrAuthorizationVersionStale) {
		t.Fatal("authorization version was not bound", err)
	}
	changed = binding
	changed.FilterHash = HashFilter("other-scope")
	if _, err = signer.Decode(token, changed); !errors.Is(err, ErrInvalid) {
		t.Fatal("scope was not bound", err)
	}
	changed = binding
	changed.Sort = "created_at_asc"
	if _, err = signer.Encode(changed, Position{ID: "last-id"}); !errors.Is(err, ErrInvalid) {
		t.Fatal("time-ordered cursor accepted an absent timestamp", err)
	}
}

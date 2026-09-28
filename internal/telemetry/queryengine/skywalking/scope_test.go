package skywalking

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSpanMembershipUsesAuthorizedResources(t *testing.T) {
	resource := uuid.New()
	state := executionState{request: Request{Start: time.Unix(1, 0), End: time.Unix(2, 0), Scope: Scope{ResourceIDs: []uuid.UUID{resource}}}}
	predicate, args := state.spanFilterScope()
	if !strings.Contains(predicate, "resource_id IN (?)") || len(args) != 3 || !reflect.DeepEqual(args[2], []uuid.UUID{resource}) {
		t.Fatalf("membership query lost authorization scope: %s %v", predicate, args)
	}
}

func TestEmptyTraceScopeFailsBeforeStorage(t *testing.T) {
	_, err := (Engine{}).Execute(context.Background(), Request{})
	if err == nil || !strings.Contains(err.Error(), "resource scope required") {
		t.Fatalf("empty scope must not become enterprise-wide access: %v", err)
	}
}

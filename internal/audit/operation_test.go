package audit

import (
	"context"
	"github.com/google/uuid"
	"testing"
)

func TestOperationContextCannotAssociateAnotherTenantOrPlatformEvent(t *testing.T) {
	enterprise := uuid.New()
	operation := OperationContext{EnterpriseID: enterprise, ActionID: uuid.New(), ActionRef: "act_bound", ActionType: "host.update", ExecutionID: uuid.New()}
	ctx := WithOperation(context.Background(), operation)
	for _, entry := range []Entry{{Domain: "platform"}, {Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: uuid.New(), Valid: true}}} {
		values := map[string]any{}
		operationDetails(ctx, entry, values)
		if len(values) != 0 {
			t.Fatal("operation context crossed audit domain")
		}
	}
	values := map[string]any{"action_ref": "untrusted_override", "password": "private"}
	operationDetails(ctx, Entry{Domain: "enterprise", EnterpriseID: uuid.NullUUID{UUID: enterprise, Valid: true}}, values)
	clean := sanitizeDetails(values)
	if clean["action_ref"] != "act_bound" || clean["action_type"] != "host.update" || clean["password"] != nil {
		t.Fatal("trusted association or sanitization lost")
	}
}

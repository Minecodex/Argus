package toolgateway

import (
	"context"
	"errors"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/connector"
	"github.com/kakj-go/Argus/internal/hostremoval"
	"github.com/kakj-go/Argus/internal/mcp"
	"github.com/kakj-go/Argus/internal/resource"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

type LifecycleTools struct {
	Base    ResourceTools
	Bastion connector.BastionService
	Removal hostremoval.Service
}

type nativePreview struct {
	id, permission, schema, idField string
	run                             func(context.Context, resource.Subject, uuid.UUID, uuid.UUID, map[string]any, string) (db.PendingAction, error)
}

func registerNativePreview(registry *mcp.Registry, base ResourceTools, document *openapi3.T, item nativePreview) error {
	schema, err := previewSchema(document, item.schema, item.idField)
	if err != nil {
		return err
	}
	metadata := base.preview(item.id, item.permission, schema, nil, func(ctx context.Context, call mcp.Call) (mcp.Result, error) {
		subject, enterprise, err := base.subject(ctx, call)
		if err != nil {
			return mcp.Result{}, err
		}
		id := uuid.Nil
		if item.idField != "" {
			id, err = callID(call, item.idField)
			if err != nil {
				return mcp.Result{}, err
			}
		}
		pending, err := item.run(ctx, subject, enterprise, id, call.Input, idempotency(call))
		if err != nil {
			return mcp.Result{}, err
		}
		return mcp.Result{Structured: pendingProjection(pending)}, nil
	})
	if err := registry.Register(metadata); err != nil {
		return err
	}
	return registry.Register(mcp.Metadata{ID: strings.TrimSuffix(item.id, ".preview") + ".commit", Risk: "dangerous", Visibility: mcp.Hidden, ExecutionMode: mcp.Sequential, Required: []string{"internal.action_executor"}, InputVersion: "argus.private_action_plan/v1", OutputVersion: "argus.execution/v1", MaxResultBytes: 1024, InputSchema: emptyObjectSchema(), Execute: func(context.Context, mcp.Call) (mcp.Result, error) {
		return mcp.Result{}, errors.New("commit is executed from the immutable action plan")
	}})
}

func (tools LifecycleTools) Register(registry *mcp.Registry) error {
	hostDocument, err := nativePreviewDocument()
	if err != nil {
		return err
	}
	for _, item := range []nativePreview{
		{"host.install.retry.preview", "host.manage", "HostPreviewCreate", "host_id", func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			return tools.Base.Resources.PreviewRetryHost(ctx, s, e, id, hostInput(in), key)
		}},
		{"host.removal.preview", "host.manage", "HostRemovalPreview", "", func(ctx context.Context, s resource.Subject, e, _ uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			target, err := uuid.Parse(stringValue(in, "target_id"))
			if err != nil {
				return db.PendingAction{}, err
			}
			return tools.Removal.Preview(ctx, s, e, hostremoval.PreviewInput{TargetType: stringValue(in, "target_type"), TargetID: target, ExpectedVersion: intValue(in, "expected_version"), Mode: stringValue(in, "mode"), CredentialID: nullID(in, "credential_id"), ConnectionTestID: nullID(in, "connection_test_id"), ConfirmationName: stringValue(in, "confirmation_name")}, key)
		}},
		{"host.windows_rdp.enable.preview", "host.manage", "ResourcePreviewDelete", "host_id", func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			return tools.Bastion.PreviewWindowsRDPEnable(ctx, s, e, id, intValue(in, "expected_version"), key)
		}},
	} {
		if err := registerNativePreview(registry, tools.Base, hostDocument, item); err != nil {
			return err
		}
	}
	document, err := nativePreviewDocument()
	if err != nil {
		return err
	}
	for _, item := range []nativePreview{
		{"connector.bastion.create.preview", "bastion_scope.manage", "BastionPreviewCreate", "", func(ctx context.Context, s resource.Subject, e, _ uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			return tools.Bastion.PreviewCreate(ctx, s, e, bastionInput(in), key)
		}},
		{"connector.bastion.update.preview", "bastion_scope.manage", "ResourcePreviewUpdate", "scope_id", func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			return tools.Bastion.PreviewUpdate(ctx, s, e, id, bastionInput(in), key)
		}},
		{"connector.bastion.delete.preview", "bastion_scope.manage", "ResourcePreviewDelete", "scope_id", func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			return tools.Bastion.PreviewLifecycle(ctx, s, e, id, intValue(in, "expected_version"), "delete", key)
		}},
		{"connector.bastion.enrollment_rotate.preview", "connector.manage", "ResourcePreviewDelete", "scope_id", func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			return tools.Bastion.PreviewLifecycle(ctx, s, e, id, intValue(in, "expected_version"), "rotate", key)
		}},
		{"connector.bastion.replacement.preview", "connector.manage", "BastionConnectorReplacementPreview", "scope_id", func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			return tools.Bastion.PreviewReplacement(ctx, s, e, id, bastionInput(in), key)
		}},
		{"connector.install.retry.preview", "connector.manage", "EmptyInput", "operation_id", func(ctx context.Context, s resource.Subject, e, id uuid.UUID, _ map[string]any, key string) (db.PendingAction, error) {
			return tools.Bastion.PreviewRetryInstall(ctx, s, e, id, key)
		}},
		{"connector.uninstall.preview", "connector.manage", "ResourcePreviewDelete", "connector_id", func(ctx context.Context, s resource.Subject, e, id uuid.UUID, in map[string]any, key string) (db.PendingAction, error) {
			return tools.Bastion.PreviewConnectorUninstall(ctx, s, e, id, intValue(in, "expected_version"), key)
		}},
	} {
		if err := registerNativePreview(registry, tools.Base, document, item); err != nil {
			return err
		}
	}
	return registry.ValidatePairs()
}

func bastionInput(in map[string]any) connector.BastionInput {
	return connector.BastionInput{Name: stringValue(in, "name"), Environment: stringValue(in, "environment"), Architecture: stringValue(in, "architecture"), Labels: labelsValue(in), ExpectedVersion: intValue(in, "expected_version"), InstallMode: stringValue(in, "install_mode"), Address: stringValue(in, "address"), Port: int32(intValue(in, "port")), Username: stringValue(in, "username"), Platform: stringValue(in, "platform"), CredentialID: nullID(in, "credential_id"), ConnectionTestID: nullID(in, "connection_test_id")}
}

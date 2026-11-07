package platform

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/kakj-go/Argus/internal/audit"
	"github.com/kakj-go/Argus/internal/identity"
	"github.com/kakj-go/Argus/internal/storage/postgres"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

var (
	ErrAlreadyInitialized = errors.New("platform is already initialized")
	ErrSetupInvalid       = errors.New("invalid setup input")
)

const (
	setupPlatformName = "Argus"
	setupLocale       = "zh-CN"
)

type SetupInput struct {
	EnterpriseName string
	Timezone       string
	ExternalURL    string
	Username       string
	DisplayName    string
	Password       string
}

type SetupResult struct {
	PlatformUserID   uuid.UUID
	EnterpriseID     uuid.UUID
	EnterpriseUserID uuid.UUID
}

type SetupService struct {
	Store       *postgres.Store
	Idempotency postgres.Idempotency
}

func (service SetupService) Status(ctx context.Context) (db.PlatformState, *db.PlatformSetting, error) {
	state, err := service.Store.Queries.GetPlatformState(ctx)
	if err != nil {
		return db.PlatformState{}, nil, err
	}
	settings, err := service.Store.Queries.GetPlatformSettings(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.PlatformState{State: state.State, InitializedAt: state.InitializedAt, UpdatedAt: state.UpdatedAt}, nil, nil
	}
	if err != nil {
		return db.PlatformState{}, nil, err
	}
	return db.PlatformState{State: state.State, InitializedAt: state.InitializedAt, UpdatedAt: state.UpdatedAt}, &settings, nil
}

func (service SetupService) Initialize(ctx context.Context, input SetupInput, idempotencyKey string) (SetupResult, error) {
	input.EnterpriseName = strings.TrimSpace(input.EnterpriseName)
	input.Username = strings.TrimSpace(input.Username)
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if input.EnterpriseName == "" || input.Username == "" || input.DisplayName == "" || input.Timezone == "" || input.Timezone == "Local" {
		return SetupResult{}, ErrSetupInvalid
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return SetupResult{}, ErrSetupInvalid
	}
	if err := identity.ValidatePassword(input.Password, input.Username, ""); err != nil {
		return SetupResult{}, err
	}
	result, err := postgres.ExecuteIdempotent(ctx, service.Store, service.Idempotency, "setup", "platform", "setup.initialize", idempotencyKey, input, 201, func(queries *db.Queries) (SetupResult, error) {
		state, err := queries.LockPlatformState(ctx)
		if err != nil {
			return SetupResult{}, err
		}
		if state.State != "uninitialized" {
			return SetupResult{}, ErrAlreadyInitialized
		}
		rows, err := queries.MarkPlatformInitializing(ctx)
		if err != nil {
			return SetupResult{}, err
		}
		if rows != 1 {
			return SetupResult{}, ErrAlreadyInitialized
		}
		platformPassword, err := identity.HashPassword(input.Password)
		if err != nil {
			return SetupResult{}, err
		}
		enterprisePassword, err := identity.HashPassword(input.Password)
		if err != nil {
			return SetupResult{}, err
		}
		userID := newUUID()
		if _, err := queries.CreatePlatformSettings(ctx, db.CreatePlatformSettingsParams{
			PlatformName: setupPlatformName, DefaultLocale: setupLocale,
			Timezone: input.Timezone, ExternalUrl: input.ExternalURL,
		}); err != nil {
			return SetupResult{}, err
		}
		if _, err := queries.CreatePlatformUser(ctx, db.CreatePlatformUserParams{
			ID: userID, Username: input.Username, DisplayName: input.DisplayName,
		}); err != nil {
			return SetupResult{}, err
		}
		if _, err := queries.CreatePasswordCredential(ctx, db.CreatePasswordCredentialParams{
			ID: newUUID(), Audience: "platform", SubjectID: userID, EncodedHash: platformPassword,
		}); err != nil {
			return SetupResult{}, err
		}
		if err := audit.InitializeChain(ctx, queries, "platform", uuid.NullUUID{}); err != nil {
			return SetupResult{}, err
		}
		if _, err := audit.Append(ctx, queries, audit.Entry{
			Domain: "platform", ActorType: "platform_user", ActorID: userID.String(),
			Action: "setup.initialize", ResourceType: "platform", ResourceID: "singleton",
			Result: "success", Details: map[string]any{"summary": "platform initialized", "username": input.Username},
		}); err != nil {
			return SetupResult{}, err
		}
		enterprise, err := createEnterprise(ctx, queries, userID.String(), CreateEnterpriseInput{
			Name: input.EnterpriseName, Timezone: input.Timezone, DefaultLocale: setupLocale,
		})
		if err != nil {
			return SetupResult{}, err
		}
		admin, err := createEnterpriseAdmin(ctx, queries, userID.String(), enterprise.ID,
			input.Username, input.DisplayName, "", enterprisePassword, time.Time{})
		if err != nil {
			return SetupResult{}, err
		}
		if err := queries.MarkPlatformInitialized(ctx); err != nil {
			return SetupResult{}, err
		}
		return SetupResult{PlatformUserID: userID, EnterpriseID: enterprise.ID, EnterpriseUserID: admin.ID}, nil
	})
	if err != nil {
		return SetupResult{}, fmt.Errorf("initialize platform: %w", err)
	}
	return result, nil
}

package agent

import (
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/kakj-go/Argus/internal/integration/modelprovider"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
)

// Invalid provider counts are diagnostic data, not a trustworthy charge. Keep
// the existing conservative reservation and its invalid provenance instead.
func usageAmount(usage modelprovider.TokenUsage, revision db.AiModelRevision, reserved pgtype.Numeric) pgtype.Numeric {
	if usage.Invalid() {
		return reserved
	}
	return tokenAmount(usage.Input, usage.Output, revision.InputPricePerMillion, revision.OutputPricePerMillion)
}

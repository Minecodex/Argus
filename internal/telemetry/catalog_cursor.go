package telemetry

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"slices"
	"strings"

	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/datapolicy"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine"
)

type catalogCursor struct {
	Context string `json:"context"`
	After   string `json:"after"`
}

func catalogContext(request DataCatalogRequest) string {
	request.Cursor, request.Limit, request.SelectedValues, request.Budget = "", 0, nil, queryengine.Budget{}
	request.ResourceIDs = slices.Clone(request.ResourceIDs)
	slices.SortFunc(request.ResourceIDs, func(a, b uuid.UUID) int { return strings.Compare(a.String(), b.String()) })
	request.SourceKeys = slices.Clone(request.SourceKeys)
	slices.Sort(request.SourceKeys)
	encoded, _ := json.Marshal([]any{datapolicy.Version, request})
	hash := sha256.Sum256(encoded)
	return hex.EncodeToString(hash[:])
}

func readCatalogCursor(request DataCatalogRequest) (string, error) {
	if request.Cursor == "" {
		return "", nil
	}
	if len(request.Cursor) > 8192 {
		return "", ErrQueryInvalid
	}
	raw, err := base64.RawURLEncoding.DecodeString(request.Cursor)
	if err != nil {
		return "", ErrQueryInvalid
	}
	var cursor catalogCursor
	if json.Unmarshal(raw, &cursor) != nil || cursor.Context != catalogContext(request) || cursor.After == "" || len(cursor.After) > 4096 {
		return "", ErrQueryInvalid
	}
	return cursor.After, nil
}

func writeCatalogCursor(request DataCatalogRequest, after string) string {
	raw, _ := json.Marshal(catalogCursor{Context: catalogContext(request), After: after})
	return base64.RawURLEncoding.EncodeToString(raw)
}

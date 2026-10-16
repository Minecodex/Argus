package presentation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/kakj-go/Argus/internal/pagination"
	"github.com/kakj-go/Argus/internal/storage/postgres/db"
	"github.com/kakj-go/Argus/internal/toolruntime"
)

var ErrReadInvalid = toolruntime.Error{Kind: "PRESENTATION_READ_INVALID"}

// PageData reads only saved public data. Its cursor is bound to the exact
// presentation, immutable data version, user and current authorization scope.
func PageData(signer pagination.Signer, owner pagination.Binding, p db.GetToolPresentationRow, data map[string]any, version string, parameters map[string]any) (map[string]any, string, error) {
	for key := range parameters {
		if key != "cursor" && key != "limit" && key != "collection" && key != "index" && key != "field" && key != "panel_id" && key != "target_id" && key != "search" && key != "filters" && key != "sort" && key != "direction" {
			return nil, "", ErrReadInvalid
		}
	}
	limit := 50
	if raw, ok := parameters["limit"]; ok {
		n, ok := raw.(float64)
		if !ok || n != float64(int(n)) || n < 1 || n > 100 {
			return nil, "", ErrReadInvalid
		}
		limit = int(n)
	}
	collection, _ := parameters["collection"].(string)
	if raw, ok := parameters["collection"]; ok {
		if _, valid := raw.(string); !valid {
			return nil, "", ErrReadInvalid
		}
	}
	if raw, ok := parameters["field"]; ok {
		if _, valid := raw.(string); !valid {
			return nil, "", ErrReadInvalid
		}
	}
	if collection == "" {
		collection = "items"
	}
	if collection != "items" && collection != "page.items" && collection != "data" && collection != "panels" && collection != "result" {
		return nil, "", ErrReadInvalid
	}
	selected, selection, err := selectPageData(data, parameters)
	if err != nil {
		return nil, "", err
	}
	data = selected
	var value any = data
	if collection == "page.items" {
		page, _ := data["page"].(map[string]any)
		value = page["items"]
	} else {
		value = data[collection]
	}
	items, _ := value.([]any)
	if items == nil && collection == "items" {
		if page, ok := data["page"].(map[string]any); ok {
			items, _ = page["items"].([]any)
		}
	}
	if field, _ := parameters["field"].(string); field != "" {
		index, ok := parameters["index"].(float64)
		if !ok || index != float64(int(index)) || index < 0 || int(index) >= len(items) {
			return nil, "", ErrReadInvalid
		}
		row, ok := items[int(index)].(map[string]any)
		if !ok {
			return nil, "", ErrReadInvalid
		}
		if field != "values" && field != "spans" && field != "rows" && field != "targets" && field != "nodes" && field != "edges" {
			return nil, "", ErrReadInvalid
		}
		items, _ = row[field].([]any)
		selection += fmt.Sprintf(":%d:%s", int(index), field)
	} else if parameters["index"] != nil {
		return nil, "", ErrReadInvalid
	}
	sourceTotal := len(items)
	facets := map[string][]string{}
	filterKey := ""
	filterable := selection == "" && (collection == "items" || collection == "page.items")
	if filterable {
		items, facets, filterKey, err = filterPageItems(items, parameters)
		if err != nil {
			return nil, "", err
		}
	} else {
		for _, key := range []string{"search", "filters", "sort", "direction"} {
			if parameters[key] != nil {
				return nil, "", ErrReadInvalid
			}
		}
	}
	filter := sha256.Sum256([]byte(p.ID.String() + p.TemplateHash + p.AuthorizationScope + version + collection + selection + filterKey))
	owner.FilterHash, owner.Sort = hex.EncodeToString(filter[:]), "presentation-offset"
	offset := 0
	if raw, ok := parameters["cursor"]; ok {
		cursor, ok := raw.(string)
		if !ok || len(cursor) > 4096 {
			return nil, "", ErrReadInvalid
		}
		position, e := signer.Decode(cursor, owner)
		if e != nil {
			return nil, "", ErrReadInvalid
		}
		offset, e = strconv.Atoi(position.ID)
		if e != nil || offset < 0 || offset > len(items) {
			return nil, "", ErrReadInvalid
		}
	}
	end := min(offset+limit, len(items))
	for end > offset {
		raw, _ := json.Marshal(items[offset:end])
		if len(raw) <= 1<<20 {
			break
		}
		end--
	}
	if end == offset && offset < len(items) {
		return nil, "", toolruntime.Error{Kind: "PRESENTATION_ROW_TOO_LARGE"}
	}
	next := ""
	if end < len(items) {
		var err error
		next, err = signer.Encode(owner, pagination.Position{Time: p.CreatedAt.Time, ID: strconv.Itoa(end)})
		if err != nil {
			return nil, "", err
		}
	}
	return ViewData(map[string]any{"items": items[offset:end], "total": len(items), "source_total": sourceTotal, "facets": facets, "filterable": filterable, "offset": offset, "complete": end == len(items), "source_version": version}), next, nil
}

// PublicSnapshot removes authority and credential values without truncating
// rows needed by authorized paging. Templates receive bounded views of it.
func PublicSnapshot(value map[string]any) map[string]any {
	raw, _ := json.Marshal(value)
	copy := map[string]any{}
	_ = json.Unmarshal(raw, &copy)
	var clean func(any) any
	clean = func(value any) any {
		switch item := value.(type) {
		case map[string]any:
			for key, child := range item {
				if privatePresentationKey(key) {
					delete(item, key)
				} else {
					item[key] = clean(child)
				}
			}
		case []any:
			for i := range item {
				item[i] = clean(item[i])
			}
		}
		return value
	}
	return clean(copy).(map[string]any)
}

func privatePresentationKey(key string) bool {
	switch strings.ToLower(key) {
	case "action_ref", "commit_token", "argus__token", "private", "frozen_plan", "approval_internal", "one_time_result", "api_key", "password", "secret_value", "credential_ciphertext", "private_key", "access_token", "csrf_token", "authorization", "cookie", "encrypted_data", "encrypted_value":
		return true
	}
	return false
}

func SnapshotVersion(data []byte) string {
	hash := sha256.Sum256(data)
	return fmt.Sprintf("sha256:%s", hex.EncodeToString(hash[:]))
}

func Observed(data map[string]any) map[string]any {
	data["observed_at"] = time.Now().UTC().Format(time.RFC3339Nano)
	return data
}

package telemetry

import (
	"testing"

	clickhouse "github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/ext"
	"github.com/google/uuid"
	"github.com/kakj-go/Argus/internal/telemetry/queryengine/chscope"
	"github.com/kakj-go/Argus/internal/telemetry/sourcekey"
)

func TestClickHouseSourceTransportKeepsEmptyFacetedMixedAndDuplicateSets(t *testing.T) {
	conn := sourceTestClickHouse(t)
	first, second := uuid.NewString()+":1", uuid.NewString()+":1"
	nsA, nsB := uuid.NewString(), uuid.NewString()
	rows, err := ext.NewTable("argus_scope_reference_rows", ext.Column("source_key", "String"), ext.Column("namespace_uid", "String"))
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range [][2]string{{first, nsA}, {first, nsB}, {second, nsB}} {
		if err = rows.Append(row[0], row[1]); err != nil {
			t.Fatal(err)
		}
	}
	base := clickhouse.Context(t.Context(), clickhouse.WithExternalTable(rows))
	for _, test := range []struct {
		name string
		keys []string
		want uint64
	}{
		{"empty", nil, 0},
		{"whole source", []string{first}, 2},
		{"namespace facet", []string{first + "@" + nsA}, 1},
		{"wrong namespace UID", []string{first + "@" + uuid.NewString()}, 0},
		{"mixed source and facet", []string{second, first + "@" + nsA}, 2},
		{"duplicate selectors", []string{second, second, first + "@" + nsA, first + "@" + nsA}, 2},
		{"selector is data", []string{"') OR true --"}, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, err := chscope.WithSources(base, test.keys)
			if err != nil {
				t.Fatal(err)
			}
			var actual, reference uint64
			predicate := chscope.FacetedPredicate("source_key", "namespace_uid")
			if err = conn.QueryRow(ctx, "SELECT count() FROM argus_scope_reference_rows WHERE "+predicate).Scan(&actual); err != nil {
				t.Fatal(err)
			}
			old, args := sourcekey.Predicate("source_key", "namespace_uid", test.keys)
			if err = conn.QueryRow(base, "SELECT count() FROM argus_scope_reference_rows WHERE "+old, args...).Scan(&reference); err != nil {
				t.Fatal(err)
			}
			if actual != test.want || actual != reference {
				t.Fatalf("source selection changed: binary=%d reference=%d want=%d", actual, reference, test.want)
			}
		})
	}
}

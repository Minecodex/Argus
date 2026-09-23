package postgres_test

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// PostgreSQL validates every generated statement against the migrated schema.
// SQLC's type generation alone does not catch all UPDATE target-column errors.
func TestGeneratedQueriesPrepareAgainstMigratedBaseline(t *testing.T) {
	address := os.Getenv("ARGUS_P5_TEST_DATABASE_URL")
	if address == "" {
		t.Skip("requires a disposable database with the current clean migration baseline")
	}
	connection, err := pgx.Connect(t.Context(), address)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close(t.Context())
	files, err := filepath.Glob(filepath.Join("..", "..", "internal", "storage", "postgres", "db", "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		for _, declaration := range parsed.Decls {
			group, ok := declaration.(*ast.GenDecl)
			if !ok || group.Tok != token.CONST {
				continue
			}
			for _, specification := range group.Specs {
				value := specification.(*ast.ValueSpec)
				if len(value.Values) != 1 {
					continue
				}
				literal, ok := value.Values[0].(*ast.BasicLit)
				if !ok || literal.Kind != token.STRING {
					continue
				}
				query, err := strconv.Unquote(literal.Value)
				if err != nil || !strings.HasPrefix(query, "-- name:") {
					continue
				}
				count++
				name := fmt.Sprintf("p5_schema_%d", count)
				if _, err := connection.Prepare(t.Context(), name, query); err != nil {
					t.Errorf("query %s: %v", value.Names[0].Name, err)
				}
			}
		}
	}
	if count == 0 {
		t.Fatal("no generated SQL queries were checked")
	}
	t.Logf("PostgreSQL prepared %d generated queries against the clean baseline", count)
}

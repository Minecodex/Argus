package skywalking

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/ClickHouse/clickhouse-go/v2"
	"github.com/ClickHouse/clickhouse-go/v2/lib/driver"
	"github.com/google/uuid"
	graphgophers "github.com/graph-gophers/graphql-go"
	graphqlparser "github.com/graphql-go/graphql/language/parser"

	"github.com/kakj-go/Argus/internal/telemetry/queryengine/chstats"
)

type Scope struct {
	SourceKeys   []string
	EnterpriseID uuid.UUID
	ResourceIDs  []uuid.UUID
}

var ErrBudget = errors.New("trace query budget exceeded")

type Budget struct {
	MaxRows               int
	MaxScanBytes          int64
	Timeout               time.Duration
	MaxDepth              int
	MaxFields             int
	MaxResultBytes        int64
	MaxRelationExpansions int
	progress              *chstats.Tracker
}

type Request struct {
	Document, OperationName string
	Variables               map[string]any
	Start, End              time.Time
	Scope                   Scope
	Budget                  Budget
}

type Result struct {
	LatestSampleAt *time.Time
	Partial        bool
	ResultType     string
	Warnings       []string
	Data           map[string]any
	Errors         []string
	Elapsed        time.Duration
	ScannedRows    int64
	ScannedBytes   int64
	ReturnedRows   int64
}

type TableRouter interface {
	Table(string, uuid.UUID) (string, error)
}

type Engine struct {
	Conn   driver.Conn
	Router TableRouter
}

//go:embed schema/trace.graphql
var schemaFS embed.FS

var traceSchema = graphgophers.MustParseSchema(
	mustSchemaSource(),
	&rootResolver{},
	graphgophers.MaxDepth(64),
	graphgophers.MaxParallelism(1),
	graphgophers.DisableIntrospection(),
)

func mustSchemaSource() string {
	data, err := schemaFS.ReadFile("schema/trace.graphql")
	if err != nil {
		panic(err)
	}
	return string(data)
}

func (e Engine) Execute(ctx context.Context, request Request) (Result, error) {
	if len(request.Scope.ResourceIDs) == 0 {
		return Result{}, fmt.Errorf("trace graphql resource scope required")
	}
	if e.Conn == nil || e.Router == nil || request.Scope.EnterpriseID == uuid.Nil {
		return Result{}, fmt.Errorf("trace graphql storage unavailable")
	}
	if request.Budget.MaxRows <= 0 {
		request.Budget.MaxRows = 50_000
	}
	if request.Budget.MaxDepth <= 0 {
		request.Budget.MaxDepth = 8
	}
	if request.Budget.MaxFields <= 0 {
		request.Budget.MaxFields = 100
	}
	if request.Budget.MaxResultBytes <= 0 {
		request.Budget.MaxResultBytes = 8 << 20
	}
	if request.Budget.MaxRelationExpansions <= 0 {
		request.Budget.MaxRelationExpansions = request.Budget.MaxRows
	}
	if request.Budget.Timeout <= 0 {
		request.Budget.Timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, request.Budget.Timeout)
	defer cancel()
	request.Budget.progress = &chstats.Tracker{}
	document, err := graphqlparser.Parse(graphqlparser.ParseParams{Source: request.Document})
	if err != nil {
		return Result{}, fmt.Errorf("graphql parse failed: %w", err)
	}
	depth, fields, err := validateDocument(document)
	if err != nil {
		return Result{}, err
	}
	if depth > request.Budget.MaxDepth {
		return Result{}, fmt.Errorf("graphql query depth exceeded")
	}
	if fields > request.Budget.MaxFields {
		return Result{}, fmt.Errorf("graphql field budget exceeded")
	}
	tables, err := e.tables(request.Scope.EnterpriseID)
	if err != nil {
		return Result{}, err
	}
	state := &executionState{engine: e, request: request, tables: tables}
	started := time.Now()
	response := traceSchema.Exec(withExecutionState(ctx, state), request.Document, request.OperationName, request.Variables)
	if len(response.Errors) > 0 {
		cause := errors.New("graphql query failed")
		if state.lastError != nil {
			cause = fmt.Errorf("graphql query failed: %w", state.lastError)
		}
		return Result{Errors: schemaErrors(response.Errors), Elapsed: time.Since(started), ScannedRows: request.Budget.progress.Rows(), ScannedBytes: request.Budget.progress.Bytes()}, cause
	}
	var data map[string]any
	if err := json.Unmarshal(response.Data, &data); err != nil {
		return Result{}, fmt.Errorf("graphql result invalid: %w", err)
	}
	if int64(len(response.Data)) > request.Budget.MaxResultBytes {
		return Result{}, ErrBudget
	}
	if request.Budget.MaxScanBytes > 0 && request.Budget.progress.Bytes() > request.Budget.MaxScanBytes {
		return Result{}, ErrBudget
	}
	kind, err := ResultKind(request.Document, request.OperationName)
	if err != nil {
		return Result{}, err
	}
	return Result{LatestSampleAt: request.Budget.progress.LatestEvent(), Partial: state.partial, ResultType: kind, Warnings: state.warnings, Data: data, Elapsed: time.Since(started), ScannedRows: request.Budget.progress.Rows(), ScannedBytes: request.Budget.progress.Bytes(), ReturnedRows: int64(state.rows + state.relations)}, nil
}

func (e Engine) tables(enterpriseID uuid.UUID) (traceTables, error) {
	spans, err := e.Router.Table("traces", enterpriseID)
	if err != nil {
		return traceTables{}, err
	}
	return traceTables{spans: spans}, nil
}

func queryContext(ctx context.Context, budget Budget) context.Context {
	settings := clickhouse.Settings{"max_result_rows": max(budget.MaxRows, budget.MaxRelationExpansions) + 1, "max_execution_time": max(1, int(budget.Timeout.Seconds()))}
	if budget.MaxScanBytes > 0 {
		remaining := budget.MaxScanBytes
		if budget.progress != nil {
			remaining -= budget.progress.Bytes()
		}
		settings["max_bytes_to_read"] = max(int64(1), remaining)
	}
	if budget.progress == nil {
		return clickhouse.Context(ctx, clickhouse.WithSettings(settings))
	}
	return budget.progress.Context(ctx, settings)
}

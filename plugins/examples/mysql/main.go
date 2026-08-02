// Command mysql is the fourth reference example plugin demonstrating a
// real connector-consuming action, mirroring plugins/examples/postgresql
// for a second SQL engine: it contributes one connector type,
// "mysql.connection@1", and two actions — "mysql.query@1" and
// "mysql.execute@1" — the same query/execute pair the vision document
// names for PostgreSQL (section 8.3), so a workflow author switching
// engines only changes which connector/action ids they bind, not the shape
// of a step.
//
// Like plugins/examples/postgresql, a connection is opened per action call
// and closed before Run returns — see that plugin's doc comment for why.
//
// It depends only on the SDK (sdk/go-plugin), never on any internal/
// package of the agent.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"

	"github.com/go-sql-driver/mysql"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

// driverName is the database/sql driver registered by go-sql-driver/mysql.
const driverName = "mysql"

const defaultPort = 3306

// sqlOpen is a seam for tests: it lets main_test.go substitute a
// database/sql-mock-backed *sql.DB without ever dialing a real server, the
// same role httptest.NewServer plays for plugins/examples/http.
var sqlOpen = sql.Open

// buildDSN turns a mysql.connection@1 connector's config and resolved
// password secret into a go-sql-driver/mysql DSN. Kept pure (no I/O) so
// construction is unit testable without a real server, the same reasoning
// as postgresql's buildDSN. ParseTime is always enabled so DATE/DATETIME/
// TIMESTAMP columns scan as time.Time rather than []byte, matching what
// normalizeValue expects to see.
func buildDSN(config map[string]any, password string) (string, error) {
	host, _ := config["host"].(string)
	if host == "" {
		return "", fmt.Errorf("connector config %q must be a non-empty string", "host")
	}
	database, _ := config["database"].(string)
	if database == "" {
		return "", fmt.Errorf("connector config %q must be a non-empty string", "database")
	}
	user, _ := config["user"].(string)
	if user == "" {
		return "", fmt.Errorf("connector config %q must be a non-empty string", "user")
	}

	port := defaultPort
	if p, ok := config["port"].(float64); ok && p > 0 {
		// Every number crossing the plugin protocol arrives as float64
		// (protobuf Struct's only numeric kind) — same note as openai's
		// max_tokens handling.
		port = int(p)
	}

	cfg := mysql.NewConfig()
	cfg.User = user
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = fmt.Sprintf("%s:%d", host, port)
	cfg.DBName = database
	cfg.ParseTime = true

	if params, ok := config["params"].(map[string]any); ok {
		cfg.Params = make(map[string]string, len(params))
		for key, value := range params {
			if s, ok := value.(string); ok {
				cfg.Params[key] = s
			}
		}
	}

	return cfg.FormatDSN(), nil
}

// normalizeValue converts a value scanned from *sql.Rows into a form
// structpb.NewStruct can encode. database/sql's driver.Value contract
// guarantees a scanned interface{} is always one of int64, float64, bool,
// []byte, string, time.Time, or nil — every case here corresponds to one of
// those, with the last three (float64, bool, string) and nil already
// structpb-safe by default.
func normalizeValue(v any) any {
	switch val := v.(type) {
	case []byte:
		return string(val)
	case int64:
		return float64(val)
	case time.Time:
		return val.Format(time.RFC3339Nano)
	default:
		return val
	}
}

// prepareCall validates the bound connector and action input shared by both
// actions, and opens a connection. The caller owns closing the returned
// *sql.DB.
func prepareCall(actionID string, connector *patchcord.ConnectorConfig, input patchcord.ActionInput) (*sql.DB, string, []any, error) {
	if connector == nil {
		return nil, "", nil, fmt.Errorf("action %q requires a bound connector", actionID)
	}
	password, _ := connector.Secrets["password"].(string)
	dsn, err := buildDSN(connector.Config, password)
	if err != nil {
		return nil, "", nil, err
	}

	sqlText, ok := input["sql"].(string)
	if !ok || sqlText == "" {
		return nil, "", nil, fmt.Errorf("input %q must be a non-empty string", "sql")
	}
	var args []any
	if raw, ok := input["args"].([]any); ok {
		args = raw
	}

	db, err := sqlOpen(driverName, dsn)
	if err != nil {
		return nil, "", nil, fmt.Errorf("open connection: %w", err)
	}
	return db, sqlText, args, nil
}

// runQuery executes a SELECT-shaped statement and collects every row into a
// structpb-safe slice of maps.
func runQuery(ctx context.Context, db *sql.DB, sqlText string, args []any) (patchcord.ActionOutput, error) {
	rows, err := db.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("execute query: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("read columns: %w", err)
	}

	result := make([]any, 0)
	for rows.Next() {
		values := make([]any, len(columns))
		pointers := make([]any, len(columns))
		for i := range values {
			pointers[i] = &values[i]
		}
		if err := rows.Scan(pointers...); err != nil {
			return nil, fmt.Errorf("scan row: %w", err)
		}

		row := make(map[string]any, len(columns))
		for i, column := range columns {
			row[column] = normalizeValue(values[i])
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return patchcord.ActionOutput{
		"rows":      result,
		"row_count": len(result),
	}, nil
}

// runExecute runs an INSERT/UPDATE/DELETE/DDL-shaped statement.
func runExecute(ctx context.Context, db *sql.DB, sqlText string, args []any) (patchcord.ActionOutput, error) {
	res, err := db.ExecContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("execute statement: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return nil, fmt.Errorf("read rows affected: %w", err)
	}
	return patchcord.ActionOutput{"rows_affected": affected}, nil
}

type queryAction struct{}

func (queryAction) ID() string { return "mysql.query@1" }

func (queryAction) Run(ctx context.Context, input patchcord.ActionInput, connector *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	db, sqlText, args, err := prepareCall("mysql.query@1", connector, input)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return runQuery(ctx, db, sqlText, args)
}

type executeAction struct{}

func (executeAction) ID() string { return "mysql.execute@1" }

func (executeAction) Run(ctx context.Context, input patchcord.ActionInput, connector *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	db, sqlText, args, err := prepareCall("mysql.execute@1", connector, input)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return runExecute(ctx, db, sqlText, args)
}

// connectorTester implements patchcord.ConnectorTester by opening a
// connection and pinging it, backing `patchcord connector test` — no query
// is run, so it works whether or not the caller has any table to query.
type connectorTester struct{}

func (connectorTester) TestConnector(ctx context.Context, connector patchcord.ConnectorConfig) error {
	password, _ := connector.Secrets["password"].(string)
	dsn, err := buildDSN(connector.Config, password)
	if err != nil {
		return err
	}

	db, err := sqlOpen(driverName, dsn)
	if err != nil {
		return fmt.Errorf("open connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping: %w", err)
	}
	return nil
}

func main() {
	plugin := patchcord.Plugin{
		Manifest: patchcord.Manifest{
			ID:      "io.patchcord.example-mysql",
			Version: "1.0.0",
		},
		Actions:     []patchcord.Action{queryAction{}, executeAction{}},
		Connectors:  []string{"mysql.connection@1"},
		Tester:      connectorTester{},
		Permissions: []string{"network.outbound"},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

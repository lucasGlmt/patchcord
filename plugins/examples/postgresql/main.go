// Command postgresql is the third reference example plugin demonstrating a
// real connector-consuming action (vision document sections 7.3/8.3): it
// contributes one connector type, "postgresql.connection@1", and two
// actions — "postgresql.query@1" and "postgresql.execute@1" — the exact
// pair named as the worked example in the vision document (section 8.3).
//
// Unlike plugins/examples/http and plugins/examples/openai, which reuse
// net/http's connection pooling transparently, a SQL connection is
// explicitly opened per action call and closed before Run returns. That
// keeps the action stateless and avoids the agent having to manage a
// plugin-wide connection pool's lifetime (reconnects, idle eviction) for a
// reference example — a real production plugin would likely cache a
// *sql.DB per connector instead.
//
// It depends only on the SDK (sdk/go-plugin), never on any internal/
// package of the agent.
package main

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

// driverName is the database/sql driver registered by pgx's stdlib package.
const driverName = "pgx"

const (
	defaultPort    = 5432
	defaultSSLMode = "disable"
)

// sqlOpen is a seam for tests: it lets main_test.go substitute a
// database/sql-mock-backed *sql.DB without ever dialing a real server, the
// same role httptest.NewServer plays for plugins/examples/http.
var sqlOpen = sql.Open

// buildDSN turns a postgresql.connection@1 connector's config and resolved
// password secret into a libpq connection string. Kept pure (no I/O) so
// construction — in particular username/password escaping — is unit
// testable without a real server, the same reasoning as openai's
// resolveBaseURL.
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

	sslmode, _ := config["sslmode"].(string)
	if sslmode == "" {
		sslmode = defaultSSLMode
	}

	dsn := url.URL{
		Scheme: "postgres",
		Host:   fmt.Sprintf("%s:%d", host, port),
		Path:   "/" + database,
	}
	if password != "" {
		dsn.User = url.UserPassword(user, password)
	} else {
		dsn.User = url.User(user)
	}
	query := dsn.Query()
	query.Set("sslmode", sslmode)
	dsn.RawQuery = query.Encode()

	return dsn.String(), nil
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

func (queryAction) ID() string { return "postgresql.query@1" }

func (queryAction) Run(ctx context.Context, input patchcord.ActionInput, connector *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	db, sqlText, args, err := prepareCall("postgresql.query@1", connector, input)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	return runQuery(ctx, db, sqlText, args)
}

type executeAction struct{}

func (executeAction) ID() string { return "postgresql.execute@1" }

func (executeAction) Run(ctx context.Context, input patchcord.ActionInput, connector *patchcord.ConnectorConfig) (patchcord.ActionOutput, error) {
	db, sqlText, args, err := prepareCall("postgresql.execute@1", connector, input)
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
			ID:      "io.patchcord.example-postgresql",
			Version: "1.0.0",
		},
		Actions:     []patchcord.Action{queryAction{}, executeAction{}},
		Connectors:  []string{"postgresql.connection@1"},
		Tester:      connectorTester{},
		Permissions: []string{"network.outbound"},
	}

	if err := patchcord.Serve(plugin); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

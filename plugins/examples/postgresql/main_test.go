package main

import (
	"context"
	"database/sql"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	patchcord "github.com/lucasglmt/patchcord/sdk/go-plugin"
)

func TestBuildDSN(t *testing.T) {
	tests := []struct {
		name    string
		config  map[string]any
		want    map[string]string // substrings expected in the DSN
		wantErr bool
	}{
		{
			name:    "requires host",
			config:  map[string]any{"database": "app", "user": "admin"},
			wantErr: true,
		},
		{
			name:    "requires database",
			config:  map[string]any{"host": "localhost", "user": "admin"},
			wantErr: true,
		},
		{
			name:    "requires user",
			config:  map[string]any{"host": "localhost", "database": "app"},
			wantErr: true,
		},
		{
			name:   "applies default port and sslmode",
			config: map[string]any{"host": "localhost", "database": "app", "user": "admin"},
			want: map[string]string{
				"host":    "localhost:5432",
				"sslmode": "sslmode=disable",
			},
		},
		{
			name:   "uses a custom port",
			config: map[string]any{"host": "db.internal", "database": "app", "user": "admin", "port": float64(6543)},
			want:   map[string]string{"host": "db.internal:6543"},
		},
		{
			name:   "uses a custom sslmode",
			config: map[string]any{"host": "localhost", "database": "app", "user": "admin", "sslmode": "require"},
			want:   map[string]string{"sslmode": "sslmode=require"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dsn, err := buildDSN(tt.config, "s3cr3t")

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("buildDSN() error = %v", err)
			}
			for _, want := range tt.want {
				if !strings.Contains(dsn, want) {
					t.Fatalf("dsn = %q, want it to contain %q", dsn, want)
				}
			}
		})
	}
}

func TestBuildDSN_EscapesCredentials(t *testing.T) {
	config := map[string]any{"host": "localhost", "database": "app", "user": "ad min"}

	dsn, err := buildDSN(config, "p@ss/word")
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}

	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("dsn is not a valid URL: %v", err)
	}
	if got := parsed.User.Username(); got != "ad min" {
		t.Fatalf("username = %q, want %q", got, "ad min")
	}
	password, _ := parsed.User.Password()
	if password != "p@ss/word" {
		t.Fatalf("password = %q, want %q", password, "p@ss/word")
	}
}

func TestBuildDSN_NoPassword(t *testing.T) {
	config := map[string]any{"host": "localhost", "database": "app", "user": "admin"}

	dsn, err := buildDSN(config, "")
	if err != nil {
		t.Fatalf("buildDSN() error = %v", err)
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("dsn is not a valid URL: %v", err)
	}
	if _, hasPassword := parsed.User.Password(); hasPassword {
		t.Fatal("expected no password in the dsn")
	}
}

func TestNormalizeValue(t *testing.T) {
	when := time.Date(2026, 8, 2, 10, 30, 0, 0, time.UTC)

	tests := []struct {
		name string
		in   any
		want any
	}{
		{"nil passes through", nil, nil},
		{"bool passes through", true, true},
		{"float64 passes through", 3.14, 3.14},
		{"string passes through", "hello", "hello"},
		{"bytes become a string", []byte("hello"), "hello"},
		{"int64 becomes a float64", int64(42), float64(42)},
		{"time becomes RFC3339Nano", when, when.Format(time.RFC3339Nano)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeValue(tt.in); got != tt.want {
				t.Fatalf("normalizeValue(%v) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func withMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	t.Helper()
	db, mock, err := sqlmock.New(sqlmock.MonitorPingsOption(true))
	if err != nil {
		t.Fatalf("sqlmock.New() error = %v", err)
	}
	t.Cleanup(func() { db.Close() })

	original := sqlOpen
	sqlOpen = func(_, _ string) (*sql.DB, error) { return db, nil }
	t.Cleanup(func() { sqlOpen = original })

	return db, mock
}

func validConnector() *patchcord.ConnectorConfig {
	return &patchcord.ConnectorConfig{
		Type:    "postgresql.connection@1",
		Config:  map[string]any{"host": "localhost", "database": "app", "user": "admin"},
		Secrets: map[string]any{"password": "s3cr3t"},
	}
}

func TestQueryAction_Run(t *testing.T) {
	t.Run("requires a bound connector", func(t *testing.T) {
		_, err := queryAction{}.Run(context.Background(), patchcord.ActionInput{"sql": "select 1"}, nil)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("requires sql in the input", func(t *testing.T) {
		_, err := queryAction{}.Run(context.Background(), patchcord.ActionInput{}, validConnector())
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns rows and row_count on success", func(t *testing.T) {
		_, mock := withMockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id, name FROM users WHERE id = $1")).
			WithArgs(float64(1)).
			WillReturnRows(sqlmock.NewRows([]string{"id", "name"}).AddRow(int64(1), "Alice"))

		output, err := queryAction{}.Run(context.Background(), patchcord.ActionInput{
			"sql":  "SELECT id, name FROM users WHERE id = $1",
			"args": []any{float64(1)},
		}, validConnector())
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}

		if output["row_count"] != 1 {
			t.Fatalf("row_count = %v, want 1", output["row_count"])
		}
		rows, ok := output["rows"].([]any)
		if !ok || len(rows) != 1 {
			t.Fatalf("rows = %v, want a single-element slice", output["rows"])
		}
		row, ok := rows[0].(map[string]any)
		if !ok {
			t.Fatalf("rows[0] = %v, want a map", rows[0])
		}
		if row["id"] != float64(1) || row["name"] != "Alice" {
			t.Fatalf("rows[0] = %v, want id=1 name=Alice", row)
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("returns an empty rows slice when there are no matches", func(t *testing.T) {
		_, mock := withMockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT id FROM users WHERE id = $1")).
			WithArgs(float64(999)).
			WillReturnRows(sqlmock.NewRows([]string{"id"}))

		output, err := queryAction{}.Run(context.Background(), patchcord.ActionInput{
			"sql":  "SELECT id FROM users WHERE id = $1",
			"args": []any{float64(999)},
		}, validConnector())
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if output["row_count"] != 0 {
			t.Fatalf("row_count = %v, want 0", output["row_count"])
		}
	})

	t.Run("propagates a driver error", func(t *testing.T) {
		_, mock := withMockDB(t)
		mock.ExpectQuery(regexp.QuoteMeta("SELECT 1")).WillReturnError(errors.New("connection refused"))

		_, err := queryAction{}.Run(context.Background(), patchcord.ActionInput{"sql": "SELECT 1"}, validConnector())
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestExecuteAction_Run(t *testing.T) {
	t.Run("requires a bound connector", func(t *testing.T) {
		_, err := executeAction{}.Run(context.Background(), patchcord.ActionInput{"sql": "delete from users"}, nil)
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("returns rows_affected on success", func(t *testing.T) {
		_, mock := withMockDB(t)
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM users WHERE id = $1")).
			WithArgs(float64(1)).
			WillReturnResult(sqlmock.NewResult(0, 1))

		output, err := executeAction{}.Run(context.Background(), patchcord.ActionInput{
			"sql":  "DELETE FROM users WHERE id = $1",
			"args": []any{float64(1)},
		}, validConnector())
		if err != nil {
			t.Fatalf("Run() error = %v", err)
		}
		if output["rows_affected"] != int64(1) {
			t.Fatalf("rows_affected = %v, want 1", output["rows_affected"])
		}

		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("propagates a driver error", func(t *testing.T) {
		_, mock := withMockDB(t)
		mock.ExpectExec(regexp.QuoteMeta("DELETE FROM users")).WillReturnError(errors.New("constraint violation"))

		_, err := executeAction{}.Run(context.Background(), patchcord.ActionInput{"sql": "DELETE FROM users"}, validConnector())
		if err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

func TestConnectorTester_TestConnector(t *testing.T) {
	t.Run("rejects an invalid connector config before opening a connection", func(t *testing.T) {
		connector := patchcord.ConnectorConfig{Config: map[string]any{}}
		if err := (connectorTester{}).TestConnector(context.Background(), connector); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})

	t.Run("succeeds when the connection pings", func(t *testing.T) {
		_, mock := withMockDB(t)
		mock.ExpectPing()

		connector := *validConnector()
		if err := (connectorTester{}).TestConnector(context.Background(), connector); err != nil {
			t.Fatalf("TestConnector() error = %v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("unmet expectations: %v", err)
		}
	})

	t.Run("fails when the ping fails", func(t *testing.T) {
		_, mock := withMockDB(t)
		mock.ExpectPing().WillReturnError(errors.New("connection refused"))

		connector := *validConnector()
		if err := (connectorTester{}).TestConnector(context.Background(), connector); err == nil {
			t.Fatal("expected an error, got nil")
		}
	})
}

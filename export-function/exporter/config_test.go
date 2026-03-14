package exporter

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadConfig_Valid(t *testing.T) {
	path := writeConfigFile(t, `
tables:
  games:
    source_table: games
    bigquery_table: games
    timestamp_column: created_at
    query: "SELECT * FROM games WHERE created_at >= $1 AND created_at < $2"
  players:
    source_table: players
    bigquery_table: players
    timestamp_column: updated_at
    query: "SELECT * FROM players WHERE updated_at >= $1 AND updated_at < $2"
`)

	config, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(config.Tables) != 2 {
		t.Fatalf("expected 2 tables, got %d", len(config.Tables))
	}

	games := config.Tables["games"]
	if games.SourceTable != "games" {
		t.Errorf("source_table: got %q, want %q", games.SourceTable, "games")
	}
	if games.BigQueryTable != "games" {
		t.Errorf("bigquery_table: got %q, want %q", games.BigQueryTable, "games")
	}
	if games.TimestampColumn != "created_at" {
		t.Errorf("timestamp_column: got %q, want %q", games.TimestampColumn, "created_at")
	}
	if games.Query == "" {
		t.Error("query should not be empty")
	}
}

func TestLoadConfig_MissingSourceTable(t *testing.T) {
	path := writeConfigFile(t, `
tables:
  games:
    bigquery_table: games
    query: "SELECT 1"
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing source_table")
	}
}

func TestLoadConfig_MissingBigQueryTable(t *testing.T) {
	path := writeConfigFile(t, `
tables:
  games:
    source_table: games
    query: "SELECT 1"
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing bigquery_table")
	}
}

func TestLoadConfig_MissingQuery(t *testing.T) {
	path := writeConfigFile(t, `
tables:
  games:
    source_table: games
    bigquery_table: games
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for missing query")
	}
}

func TestLoadConfig_NoTables(t *testing.T) {
	path := writeConfigFile(t, `tables:`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for empty tables")
	}
}

func TestLoadConfig_FileNotFound(t *testing.T) {
	_, err := LoadConfig("/nonexistent/path/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestLoadConfig_InvalidYAML(t *testing.T) {
	path := writeConfigFile(t, `{{{invalid yaml`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

func TestLoadConfig_SourceTableMismatchReportedWithTableName(t *testing.T) {
	path := writeConfigFile(t, `
tables:
  my_custom_table:
    bigquery_table: bq_table
    query: "SELECT 1"
`)

	_, err := LoadConfig(path)
	if err == nil {
		t.Fatal("expected error")
	}
	// Error message should identify which table has the problem
	if got := err.Error(); !contains(got, "my_custom_table") {
		t.Errorf("error should mention table name 'my_custom_table', got: %s", got)
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

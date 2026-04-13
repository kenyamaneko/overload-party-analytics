package adapter

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// mockRows は pgRowsToMaps テスト用の pgx.Rows 実装です。
type mockRows struct {
	fields []pgconn.FieldDescription
	data   [][]any
	index  int
	err    error
}

func (m *mockRows) Close()                                       {}
func (m *mockRows) Err() error                                   { return m.err }
func (m *mockRows) CommandTag() pgconn.CommandTag                { return pgconn.CommandTag{} }
func (m *mockRows) FieldDescriptions() []pgconn.FieldDescription { return m.fields }
func (m *mockRows) Next() bool {
	m.index++
	return m.index <= len(m.data)
}
func (m *mockRows) Scan(dest ...any) error { return nil }
func (m *mockRows) Values() ([]any, error) {
	if m.index < 1 || m.index > len(m.data) {
		return nil, fmt.Errorf("no current row")
	}
	return m.data[m.index-1], nil
}
func (m *mockRows) RawValues() [][]byte { return nil }
func (m *mockRows) Conn() *pgx.Conn    { return nil }

func newMockRows(fields []string, data [][]any) *mockRows {
	fds := make([]pgconn.FieldDescription, len(fields))
	for i, name := range fields {
		fds[i] = pgconn.FieldDescription{Name: name}
	}
	return &mockRows{fields: fds, data: data, index: 0}
}

func TestPgRowsToMaps_BasicTypes(t *testing.T) {
	rows := newMockRows(
		[]string{"name", "count", "active", "nullable"},
		[][]any{{"alice", int64(42), true, nil}},
	)

	result, err := pgRowsToMaps(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 row, got %d", len(result))
	}

	row := result[0]
	if row["name"] != "alice" {
		t.Errorf("name: got %v, want alice", row["name"])
	}
	if row["count"] != int64(42) {
		t.Errorf("count: got %v, want 42", row["count"])
	}
	if row["active"] != true {
		t.Errorf("active: got %v, want true", row["active"])
	}
	if row["nullable"] != nil {
		t.Errorf("nullable: got %v, want nil", row["nullable"])
	}
}

func TestPgRowsToMaps_TimeConversion(t *testing.T) {
	ts := time.Date(2025, 6, 15, 10, 30, 0, 123456789, time.UTC)
	rows := newMockRows(
		[]string{"created_at"},
		[][]any{{ts}},
	)

	result, err := pgRowsToMaps(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, ok := result[0]["created_at"].(string)
	if !ok {
		t.Fatalf("expected string, got %T", result[0]["created_at"])
	}

	want := ts.Format(time.RFC3339Nano)
	if got != want {
		t.Errorf("created_at: got %q, want %q", got, want)
	}
}

func TestPgRowsToMaps_JSONBBytes(t *testing.T) {
	jsonData := []byte(`{"faction":"tech","cards":[1,2,3]}`)
	rows := newMockRows(
		[]string{"deck_snapshot"},
		[][]any{{jsonData}},
	)

	result, err := pgRowsToMaps(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val := result[0]["deck_snapshot"]
	raw, ok := val.(json.RawMessage)
	if !ok {
		t.Fatalf("expected json.RawMessage, got %T", val)
	}
	if string(raw) != string(jsonData) {
		t.Errorf("deck_snapshot: got %s, want %s", raw, jsonData)
	}
}

func TestPgRowsToMaps_JSONBMap(t *testing.T) {
	mapData := map[string]interface{}{"key": "value"}
	rows := newMockRows(
		[]string{"event_data"},
		[][]any{{mapData}},
	)

	result, err := pgRowsToMaps(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	val, ok := result[0]["event_data"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map[string]interface{}, got %T", result[0]["event_data"])
	}
	if val["key"] != "value" {
		t.Errorf("event_data.key: got %v, want value", val["key"])
	}
}

func TestPgRowsToMaps_EmptyResult(t *testing.T) {
	rows := newMockRows([]string{"id"}, nil)

	result, err := pgRowsToMaps(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected 0 rows, got %d", len(result))
	}
}

func TestPgRowsToMaps_MultipleRows(t *testing.T) {
	rows := newMockRows(
		[]string{"id", "name"},
		[][]any{
			{int64(1), "alice"},
			{int64(2), "bob"},
			{int64(3), "charlie"},
		},
	)

	result, err := pgRowsToMaps(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 rows, got %d", len(result))
	}
	if result[1]["name"] != "bob" {
		t.Errorf("row 1 name: got %v, want bob", result[1]["name"])
	}
}

// TestPgRowsToMaps_JSONBNotDoubleEncoded は json.RawMessage として保持した
// JSONB データが JSONL (GCS → BigQuery) シリアライズ時に二重エンコードされない
// ことを検証します。
func TestPgRowsToMaps_JSONBNotDoubleEncoded(t *testing.T) {
	jsonData := []byte(`{"faction":"tech","level":5}`)
	rows := newMockRows(
		[]string{"player_id", "deck_snapshot"},
		[][]any{{"p-123", jsonData}},
	)

	result, err := pgRowsToMaps(rows)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	encoded, err := json.Marshal(result[0])
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(encoded, &parsed); err != nil {
		t.Fatalf("json.Unmarshal: %v", err)
	}

	snapshot, ok := parsed["deck_snapshot"].(map[string]interface{})
	if !ok {
		t.Fatalf("deck_snapshot should be a JSON object, got %T: %v", parsed["deck_snapshot"], parsed["deck_snapshot"])
	}
	if snapshot["faction"] != "tech" {
		t.Errorf("deck_snapshot.faction: got %v, want tech", snapshot["faction"])
	}
}

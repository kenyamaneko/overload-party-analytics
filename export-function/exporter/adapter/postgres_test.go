package adapter

import (
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// mockRows は pgRowsToMaps テスト用の pgx.Rows 実装です。
type mockRows struct {
	fields    []pgconn.FieldDescription
	data      [][]any
	index     int
	err       error
	valuesErr error
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
	if m.valuesErr != nil {
		return nil, m.valuesErr
	}
	if m.index < 1 || m.index > len(m.data) {
		return nil, fmt.Errorf("no current row")
	}
	return m.data[m.index-1], nil
}
func (m *mockRows) RawValues() [][]byte { return nil }
func (m *mockRows) Conn() *pgx.Conn     { return nil }

func newMockRows(fields []string, data [][]any) *mockRows {
	fds := make([]pgconn.FieldDescription, len(fields))
	for i, name := range fields {
		fds[i] = pgconn.FieldDescription{Name: name}
	}
	return &mockRows{fields: fds, data: data, index: 0}
}

func TestPgRowsToMaps(t *testing.T) {
	t.Run("クエリ結果行のマップ変換", func(t *testing.T) {
		t.Run("文字列・整数・真偽・NULL を含む行のとき、各カラムの値が保持される", func(t *testing.T) {
			rows := newMockRows(
				[]string{"name", "count", "active", "nullable"},
				[][]any{{"alice", int64(42), true, nil}},
			)

			result, err := pgRowsToMaps(rows)
			require.NoError(t, err)
			require.Len(t, result, 1)

			row := result[0]
			require.Equal(t, "alice", row["name"])
			require.Equal(t, int64(42), row["count"])
			require.Equal(t, true, row["active"])
			require.Nil(t, row["nullable"])
		})

		t.Run("time.Time のカラムのとき、RFC3339Nano 文字列に変換される", func(t *testing.T) {
			ts := time.Date(2025, 6, 15, 10, 30, 0, 123456789, time.UTC)
			rows := newMockRows(
				[]string{"created_at"},
				[][]any{{ts}},
			)

			result, err := pgRowsToMaps(rows)
			require.NoError(t, err)

			got, ok := result[0]["created_at"].(string)
			require.True(t, ok)
			require.Equal(t, "2025-06-15T10:30:00.123456789Z", got)
		})

		t.Run("JSONB カラムが []byte のとき、json.RawMessage として保持される", func(t *testing.T) {
			jsonData := []byte(`{"faction":"tech","cards":[1,2,3]}`)
			rows := newMockRows(
				[]string{"deck_snapshot"},
				[][]any{{jsonData}},
			)

			result, err := pgRowsToMaps(rows)
			require.NoError(t, err)

			raw, ok := result[0]["deck_snapshot"].(json.RawMessage)
			require.True(t, ok)
			require.Equal(t, string(jsonData), string(raw))
		})

		t.Run("JSONB カラムが map のとき、map のまま保持される", func(t *testing.T) {
			mapData := map[string]interface{}{"key": "value"}
			rows := newMockRows(
				[]string{"event_data"},
				[][]any{{mapData}},
			)

			result, err := pgRowsToMaps(rows)
			require.NoError(t, err)

			val, ok := result[0]["event_data"].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, "value", val["key"])
		})

		t.Run("行が無いとき、空の結果になる", func(t *testing.T) {
			rows := newMockRows([]string{"id"}, nil)

			result, err := pgRowsToMaps(rows)
			require.NoError(t, err)
			require.Empty(t, result)
		})

		t.Run("複数行のとき、全行が変換される", func(t *testing.T) {
			rows := newMockRows(
				[]string{"id", "name"},
				[][]any{
					{int64(1), "alice"},
					{int64(2), "bob"},
					{int64(3), "charlie"},
				},
			)

			result, err := pgRowsToMaps(rows)
			require.NoError(t, err)
			require.Len(t, result, 3)
			require.Equal(t, "bob", result[1]["name"])
		})

		t.Run("json.RawMessage の JSONB を JSON 化しても二重エンコードされない", func(t *testing.T) {
			// json.RawMessage として保持した JSONB は JSONL (GCS → BigQuery)
			// シリアライズ時に文字列へ二重エンコードされず、JSON オブジェクトのまま出力される。
			jsonData := []byte(`{"faction":"tech","level":5}`)
			rows := newMockRows(
				[]string{"player_id", "deck_snapshot"},
				[][]any{{"p-123", jsonData}},
			)

			result, err := pgRowsToMaps(rows)
			require.NoError(t, err)

			encoded, err := json.Marshal(result[0])
			require.NoError(t, err)

			var parsed map[string]interface{}
			require.NoError(t, json.Unmarshal(encoded, &parsed))

			snapshot, ok := parsed["deck_snapshot"].(map[string]interface{})
			require.True(t, ok)
			require.Equal(t, "tech", snapshot["faction"])
		})

		t.Run("小数・16bit整数・32bit整数のカラムのとき、値が保持される", func(t *testing.T) {
			rows := newMockRows(
				[]string{"ratio", "small", "medium"},
				[][]any{{float64(1.5), int16(7), int32(1000)}},
			)

			result, err := pgRowsToMaps(rows)
			require.NoError(t, err)

			require.Equal(t, float64(1.5), result[0]["ratio"])
			require.Equal(t, int16(7), result[0]["small"])
			require.Equal(t, int32(1000), result[0]["medium"])
		})

		unsupportedTypeCases := []struct {
			name     string
			value    any
			wantType string
		}{
			{
				name:     "uuid のカラムのとき、変換規則が無いためエラーになり型が示される",
				value:    [16]byte{},
				wantType: "[16]uint8",
			},
			{
				name:     "配列のカラムのとき、変換規則が無いためエラーになり型が示される",
				value:    []string{"a", "b"},
				wantType: "[]string",
			},
			{
				name:     "numeric のカラムのとき、変換規則が無いためエラーになり型が示される",
				value:    struct{ Int int64 }{Int: 1},
				wantType: "struct",
			},
		}
		for _, tt := range unsupportedTypeCases {
			t.Run(tt.name, func(t *testing.T) {
				rows := newMockRows([]string{"tst_col"}, [][]any{{tt.value}})

				result, err := pgRowsToMaps(rows)
				require.Error(t, err)
				require.Contains(t, err.Error(), "unsupported Postgres value type")
				require.Contains(t, err.Error(), tt.wantType)
				require.Contains(t, err.Error(), "tst_col")
				require.Nil(t, result)
			})
		}

		t.Run("行の値の読み出しに失敗したとき、エラーになり行は返らない", func(t *testing.T) {
			rows := newMockRows([]string{"id"}, [][]any{{int64(1)}})
			rows.valuesErr = fmt.Errorf("dummy values error")

			result, err := pgRowsToMaps(rows)
			require.Error(t, err)
			require.Contains(t, err.Error(), "read row values")
			require.Nil(t, result)
		})

		t.Run("行の反復中にエラーが発生したとき、エラーになり行は返らない", func(t *testing.T) {
			rows := newMockRows([]string{"id"}, [][]any{{int64(1)}})
			rows.err = fmt.Errorf("dummy iterate error")

			result, err := pgRowsToMaps(rows)
			require.Error(t, err)
			require.Contains(t, err.Error(), "iterate rows")
			require.Nil(t, result)
		})
	})
}

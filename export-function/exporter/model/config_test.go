package model

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func writeConfigFile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoadConfig(t *testing.T) {
	t.Run("設定ファイルの読み込み", func(t *testing.T) {
		t.Run("必須項目が揃った設定のとき、各テーブルの定義が読み込まれる", func(t *testing.T) {
			path := writeConfigFile(t, `
tables:
  games:
    source_table: games
    bigquery_table: games
    query: "SELECT * FROM games WHERE created_at >= $1 AND created_at < $2"
  players:
    source_table: players
    bigquery_table: players
    query: "SELECT * FROM players WHERE updated_at >= $1 AND updated_at < $2"
`)

			config, err := LoadConfig(path)
			require.NoError(t, err)
			require.Len(t, config.Tables, 2)

			games := config.Tables["games"]
			require.Equal(t, "games", games.SourceTable)
			require.Equal(t, "games", games.BigQueryTable)
			require.NotEmpty(t, games.Query)
		})

		invalidCases := []struct {
			name string
			yaml string
		}{
			{
				name: "source_table が無いとき、エラーになる",
				yaml: `
tables:
  games:
    bigquery_table: games
    query: "SELECT 1"
`,
			},
			{
				name: "bigquery_table が無いとき、エラーになる",
				yaml: `
tables:
  games:
    source_table: games
    query: "SELECT 1"
`,
			},
			{
				name: "query が無いとき、エラーになる",
				yaml: `
tables:
  games:
    source_table: games
    bigquery_table: games
`,
			},
			{
				name: "tables が空のとき、エラーになる",
				yaml: `tables:`,
			},
			{
				name: "YAML として解析できないとき、エラーになる",
				yaml: `{{{invalid yaml`,
			},
		}
		for _, tt := range invalidCases {
			t.Run(tt.name, func(t *testing.T) {
				_, err := LoadConfig(writeConfigFile(t, tt.yaml))
				require.Error(t, err)
			})
		}

		t.Run("設定ファイルが存在しないとき、エラーになる", func(t *testing.T) {
			_, err := LoadConfig("/nonexistent/path/config.yaml")
			require.Error(t, err)
		})

		t.Run("source_table が無いとき、エラーメッセージに該当テーブル名が含まれる", func(t *testing.T) {
			path := writeConfigFile(t, `
tables:
  my_custom_table:
    bigquery_table: bq_table
    query: "SELECT 1"
`)

			_, err := LoadConfig(path)
			require.Error(t, err)
			require.Contains(t, err.Error(), "my_custom_table")
		})
	})
}

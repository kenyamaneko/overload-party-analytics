package model

import (
	"fmt"
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
    natural_key: [game_id]
    query: "SELECT * FROM games WHERE created_at >= $1 AND created_at < $2"
  players:
    source_table: players
    bigquery_table: players
    natural_key: [player_id]
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
			name            string
			yaml            string
			wantErrContains string
		}{
			{
				name: "source_tableが無いとき、エラーになる",
				yaml: `
tables:
  games:
    bigquery_table: games
    query: "SELECT 1"
`,
				wantErrContains: "source_table is required",
			},
			{
				name: "bigquery_tableが無いとき、エラーになる",
				yaml: `
tables:
  games:
    source_table: games
    query: "SELECT 1"
`,
				wantErrContains: "bigquery_table is required",
			},
			{
				name: "queryが無いとき、エラーになる",
				yaml: `
tables:
  games:
    source_table: games
    bigquery_table: games
`,
				wantErrContains: "query is required",
			},
			{
				name: "natural_keyが無いとき、エラーになる",
				yaml: `
tables:
  games:
    source_table: games
    bigquery_table: games
    query: "SELECT 1"
`,
				wantErrContains: "natural_key is required",
			},
			{
				name:            "tablesが空のとき、エラーになる",
				yaml:            `tables:`,
				wantErrContains: "no tables defined in config",
			},
			{
				name:            "YAMLとして解析できないとき、エラーになる",
				yaml:            `{{{invalid yaml`,
				wantErrContains: "parse config file",
			},
		}
		for _, tt := range invalidCases {
			t.Run(tt.name, func(t *testing.T) {
				_, err := LoadConfig(writeConfigFile(t, tt.yaml))
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrContains)
			})
		}

		t.Run("設定ファイルが存在しないとき、エラーになる", func(t *testing.T) {
			_, err := LoadConfig("/nonexistent/path/config.yaml")
			require.Error(t, err)
			require.Contains(t, err.Error(), "read config file")
		})

		t.Run("source_tableが無いとき、エラーメッセージに該当テーブル名が含まれる", func(t *testing.T) {
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

		t.Run("bigquery_tableの識別子検証", func(t *testing.T) {
			invalidIdentifierCases := []struct {
				name          string
				bigqueryTable string
			}{
				{
					name:          "ハイフンを含むとき、エラーになる",
					bigqueryTable: "tst-games",
				},
				{
					name:          "セミコロンと空白を含むとき、エラーになる",
					bigqueryTable: `tst_games; DROP TABLE x`,
				},
				{
					name:          "数字で始まるとき、エラーになる",
					bigqueryTable: "1tst_games",
				},
			}
			for _, tt := range invalidIdentifierCases {
				t.Run(tt.name, func(t *testing.T) {
					path := writeConfigFile(t, fmt.Sprintf(`
tables:
  tst_games:
    source_table: tst_games
    bigquery_table: %q
    query: "SELECT 1"
`, tt.bigqueryTable))

					_, err := LoadConfig(path)
					require.Error(t, err)
					require.Contains(t, err.Error(), tt.bigqueryTable)
					require.Contains(t, err.Error(), "not a valid identifier")
				})
			}

			t.Run("アンダースコア始まりのとき、読み込める", func(t *testing.T) {
				path := writeConfigFile(t, `
tables:
  tst_games:
    source_table: tst_games
    bigquery_table: _tst_games
    natural_key: [tst_id]
    query: "SELECT 1"
`)

				config, err := LoadConfig(path)
				require.NoError(t, err)
				require.Equal(t, "_tst_games", config.Tables["tst_games"].BigQueryTable)
			})
		})

		t.Run("natural_keyの識別子検証", func(t *testing.T) {
			t.Run("カラム名にハイフンを含むとき、エラーになり該当テーブル名とカラム名が示される", func(t *testing.T) {
				path := writeConfigFile(t, `
tables:
  tst_games:
    source_table: tst_games
    bigquery_table: tst_games
    natural_key: [tst-id]
    query: "SELECT 1"
`)

				_, err := LoadConfig(path)
				require.Error(t, err)
				require.Contains(t, err.Error(), "tst_games")
				require.Contains(t, err.Error(), "tst-id")
			})

			t.Run("2カラム指定したとき、両カラムが読み込まれる", func(t *testing.T) {
				path := writeConfigFile(t, `
tables:
  tst_games:
    source_table: tst_games
    bigquery_table: tst_games
    natural_key: [tst_id, tst_updated_at]
    query: "SELECT 1"
`)

				config, err := LoadConfig(path)
				require.NoError(t, err)
				require.Equal(t, []string{"tst_id", "tst_updated_at"}, config.Tables["tst_games"].NaturalKey)
			})
		})
	})
}

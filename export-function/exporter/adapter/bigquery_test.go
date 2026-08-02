package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"export-to-bq/exporter/model"
)

func TestBuildMergeSQL(t *testing.T) {
	t.Run("MERGE 文の生成", func(t *testing.T) {
		tests := []struct {
			name         string
			datasetID    string
			targetTable  string
			stagingTable string
			naturalKey   []string
			columns      []string
		}{
			{
				name:         "自然キーが1個・列が1個のとき、全キーと全列を含む MERGE 文になる",
				datasetID:    "analytics",
				targetTable:  "games",
				stagingTable: "games_staging",
				naturalKey:   []string{"game_id"},
				columns:      []string{"game_id"},
			},
			{
				name:         "自然キーが1個・列が複数のとき、全キーと全列を含む MERGE 文になる",
				datasetID:    "analytics",
				targetTable:  "games",
				stagingTable: "games_staging",
				naturalKey:   []string{"game_id"},
				columns:      []string{"game_id", "score", "created_at"},
			},
			{
				name:         "自然キーが複数・列が1個のとき、全キーと全列を含む MERGE 文になる",
				datasetID:    "analytics",
				targetTable:  "scores",
				stagingTable: "scores_staging",
				naturalKey:   []string{"game_id", "player_id"},
				columns:      []string{"value"},
			},
			{
				name:         "自然キーが複数・列が複数のとき、全キーと全列を含む MERGE 文になる",
				datasetID:    "analytics",
				targetTable:  "scores",
				stagingTable: "scores_staging",
				naturalKey:   []string{"game_id", "player_id"},
				columns:      []string{"game_id", "player_id", "score", "updated_at"},
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := buildMergeSQL(tt.datasetID, tt.targetTable, tt.stagingTable, tt.naturalKey, tt.columns)

				require.Contains(t, got, "WHEN MATCHED THEN UPDATE")
				require.Contains(t, got, "WHEN NOT MATCHED THEN INSERT")
				require.Contains(t, got, tt.targetTable)
				require.Contains(t, got, tt.stagingTable)
				for _, key := range tt.naturalKey {
					require.Contains(t, got, key)
				}
				for _, col := range tt.columns {
					require.Contains(t, got, col)
				}
			})
		}
	})
}

func TestNewBQLoader(t *testing.T) {
	t.Run("BQLoader の生成", func(t *testing.T) {
		t.Run("データセット ID にハイフンを含むとき、生成はエラーになり該当値が示される", func(t *testing.T) {
			_, err := NewBQLoader(context.Background(), "tst-project", "tst-dataset")
			require.Error(t, err)
			require.Contains(t, err.Error(), "tst-dataset")
			require.Contains(t, err.Error(), "not a valid identifier")
		})
	})
}

func TestLoad(t *testing.T) {
	t.Run("BigQuery へのロード", func(t *testing.T) {
		t.Run("ロード先テーブル名にセミコロンを含むとき、ロードはエラーになる", func(t *testing.T) {
			l := &BQLoader{}
			tableConfig := model.TableConfig{BigQueryTable: "tst_games; DROP"}

			_, err := l.Load(context.Background(), tableConfig, "gs://tst-bucket/exports/tst.jsonl", model.DedupModeAppend)
			require.Error(t, err)
			require.Contains(t, err.Error(), "not a valid identifier")
		})

		t.Run("merge ロードで natural_key のカラム名に空白を含むとき、ロードはエラーになる", func(t *testing.T) {
			l := &BQLoader{}
			tableConfig := model.TableConfig{
				BigQueryTable: "tst_games",
				NaturalKey:    []string{"tst id"},
			}

			_, err := l.Load(context.Background(), tableConfig, "gs://tst-bucket/exports/tst.jsonl", model.DedupModeMerge)
			require.Error(t, err)
			require.Contains(t, err.Error(), "tst id")
		})
	})
}

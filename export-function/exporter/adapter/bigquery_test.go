package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"
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

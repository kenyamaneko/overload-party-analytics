package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildMergeSQL は生成された MERGE 文が期待どおりの構造を備えることを検証します。
func TestBuildMergeSQL(t *testing.T) {
	tests := []struct {
		name         string
		datasetID    string
		targetTable  string
		stagingTable string
		naturalKey   []string
		columns      []string
	}{
		{
			name:         "single natural key single column",
			datasetID:    "analytics",
			targetTable:  "games",
			stagingTable: "games_staging",
			naturalKey:   []string{"game_id"},
			columns:      []string{"game_id"},
		},
		{
			name:         "single natural key multiple columns",
			datasetID:    "analytics",
			targetTable:  "games",
			stagingTable: "games_staging",
			naturalKey:   []string{"game_id"},
			columns:      []string{"game_id", "score", "created_at"},
		},
		{
			name:         "multiple natural keys single column",
			datasetID:    "analytics",
			targetTable:  "scores",
			stagingTable: "scores_staging",
			naturalKey:   []string{"game_id", "player_id"},
			columns:      []string{"value"},
		},
		{
			name:         "multiple natural keys multiple columns",
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
}

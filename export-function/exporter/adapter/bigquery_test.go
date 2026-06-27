package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestBuildMergeSQL は naturalKey と columns の件数の境界 (1 件 / 多件) ごとに
// 生成される MERGE 文が期待どおりであることを検証します。
func TestBuildMergeSQL(t *testing.T) {
	tests := []struct {
		name         string
		datasetID    string
		targetTable  string
		stagingTable string
		naturalKey   []string
		columns      []string
		want         string
	}{
		{
			name:         "single natural key single column",
			datasetID:    "analytics",
			targetTable:  "games",
			stagingTable: "games_staging",
			naturalKey:   []string{"game_id"},
			columns:      []string{"game_id"},
			want: "MERGE INTO `analytics.games` AS target " +
				"USING `analytics.games_staging` AS source " +
				"ON target.game_id = source.game_id " +
				"WHEN MATCHED THEN UPDATE SET game_id = source.game_id " +
				"WHEN NOT MATCHED THEN INSERT (game_id) VALUES (source.game_id)",
		},
		{
			name:         "single natural key multiple columns",
			datasetID:    "analytics",
			targetTable:  "games",
			stagingTable: "games_staging",
			naturalKey:   []string{"game_id"},
			columns:      []string{"game_id", "score", "created_at"},
			want: "MERGE INTO `analytics.games` AS target " +
				"USING `analytics.games_staging` AS source " +
				"ON target.game_id = source.game_id " +
				"WHEN MATCHED THEN UPDATE SET game_id = source.game_id, score = source.score, created_at = source.created_at " +
				"WHEN NOT MATCHED THEN INSERT (game_id, score, created_at) VALUES (source.game_id, source.score, source.created_at)",
		},
		{
			name:         "multiple natural keys single column",
			datasetID:    "analytics",
			targetTable:  "scores",
			stagingTable: "scores_staging",
			naturalKey:   []string{"game_id", "player_id"},
			columns:      []string{"value"},
			want: "MERGE INTO `analytics.scores` AS target " +
				"USING `analytics.scores_staging` AS source " +
				"ON target.game_id = source.game_id AND target.player_id = source.player_id " +
				"WHEN MATCHED THEN UPDATE SET value = source.value " +
				"WHEN NOT MATCHED THEN INSERT (value) VALUES (source.value)",
		},
		{
			name:         "multiple natural keys multiple columns",
			datasetID:    "analytics",
			targetTable:  "scores",
			stagingTable: "scores_staging",
			naturalKey:   []string{"game_id", "player_id"},
			columns:      []string{"game_id", "player_id", "score", "updated_at"},
			want: "MERGE INTO `analytics.scores` AS target " +
				"USING `analytics.scores_staging` AS source " +
				"ON target.game_id = source.game_id AND target.player_id = source.player_id " +
				"WHEN MATCHED THEN UPDATE SET game_id = source.game_id, player_id = source.player_id, score = source.score, updated_at = source.updated_at " +
				"WHEN NOT MATCHED THEN INSERT (game_id, player_id, score, updated_at) VALUES (source.game_id, source.player_id, source.score, source.updated_at)",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildMergeSQL(tt.datasetID, tt.targetTable, tt.stagingTable, tt.naturalKey, tt.columns)
			require.Equal(t, tt.want, got)
		})
	}
}

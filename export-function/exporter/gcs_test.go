package exporter

import (
	"testing"
	"time"
)

func TestGcsObjectPath(t *testing.T) {
	tests := []struct {
		name      string
		table     string
		timestamp time.Time
		want      string
	}{
		{
			name:      "standard path",
			table:     "games",
			timestamp: time.Date(2025, 3, 14, 9, 5, 30, 0, time.UTC),
			want:      "exports/20250314/games/090530.jsonl",
		},
		{
			name:      "midnight",
			table:     "game_events",
			timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
			want:      "exports/20250101/game_events/000000.jsonl",
		},
		{
			name:      "end of day",
			table:     "players",
			timestamp: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
			want:      "exports/20251231/players/235959.jsonl",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := gcsObjectPath(tt.table, tt.timestamp)
			if got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

package adapter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestBuildGCSObjectPath(t *testing.T) {
	t.Run("GCS オブジェクトパスの生成", func(t *testing.T) {
		tests := []struct {
			name      string
			table     string
			timestamp time.Time
			want      string
		}{
			{
				name:      "通常の日時のとき、日付とゼロ埋め時刻を含むパスになる",
				table:     "games",
				timestamp: time.Date(2025, 3, 14, 9, 5, 30, 0, time.UTC),
				want:      "exports/20250314/games/090530.000.jsonl",
			},
			{
				name:      "深夜0時のとき、時刻部分が 000000.000 になる",
				table:     "game_events",
				timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
				want:      "exports/20250101/game_events/000000.000.jsonl",
			},
			{
				name:      "1日の終わり(23:59:59)のとき、時刻部分が 235959.000 になる",
				table:     "players",
				timestamp: time.Date(2025, 12, 31, 23, 59, 59, 0, time.UTC),
				want:      "exports/20251231/players/235959.000.jsonl",
			},
			{
				name:      "ミリ秒があるとき、時刻部分が 090530.123 になる",
				table:     "games",
				timestamp: time.Date(2025, 3, 14, 9, 5, 30, 123000000, time.UTC),
				want:      "exports/20250314/games/090530.123.jsonl",
			},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				got := buildGCSObjectPath(tt.table, tt.timestamp)
				require.Equal(t, tt.want, got)
			})
		}
	})
}

func TestParseObjectPath(t *testing.T) {
	t.Run("ステージング URI のバケット検証", func(t *testing.T) {
		t.Run("自バケットの URI のとき、バケット部分を除いたオブジェクトパスが得られる", func(t *testing.T) {
			w := &GCSWriter{bucket: "tst-bucket"}

			got, err := w.parseObjectPath("gs://tst-bucket/exports/tst.jsonl")
			require.NoError(t, err)
			require.Equal(t, "exports/tst.jsonl", got)
		})
	})
}

func TestDelete(t *testing.T) {
	t.Run("ステージングオブジェクトの削除", func(t *testing.T) {
		t.Run("別バケットの URI を削除しようとしたとき、エラーになる", func(t *testing.T) {
			w := &GCSWriter{bucket: "tst-bucket"}

			err := w.Delete(context.Background(), "gs://tst-other/exports/tst.jsonl")
			require.Error(t, err)
			require.Contains(t, err.Error(), "does not belong to bucket")
		})

		t.Run("gs:// で始まらない URI を削除しようとしたとき、エラーになる", func(t *testing.T) {
			w := &GCSWriter{bucket: "tst-bucket"}

			err := w.Delete(context.Background(), "/exports/tst.jsonl")
			require.Error(t, err)
		})
	})
}

//go:build integration

package adapter

import (
	"context"
	"testing"
	"time"

	"cloud.google.com/go/firestore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcfirestore "github.com/testcontainers/testcontainers-go/modules/gcloud/firestore"

	"export-to-bq/exporter/model"
)

const firestoreEmulatorImage = "gcr.io/google.com/cloudsdktool/cloud-sdk:582.0.0-emulators"

func TestFirestoreCheckpointStore(t *testing.T) {
	ctx := context.Background()

	container, err := tcfirestore.Run(ctx, firestoreEmulatorImage)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, container.Terminate(ctx)) })

	t.Setenv("FIRESTORE_EMULATOR_HOST", container.URI())

	rawClient, err := firestore.NewClient(ctx, container.ProjectID())
	require.NoError(t, err)
	t.Cleanup(func() { _ = rawClient.Close() })

	newStore := func(t *testing.T) *FirestoreCheckpointStore {
		t.Helper()
		store, err := NewFirestoreCheckpoint(ctx, container.ProjectID())
		require.NoError(t, err)
		t.Cleanup(func() { _ = store.Close() })
		return store
	}

	t.Run("[結合テスト]チェックポイントの取得", func(t *testing.T) {
		t.Run("指定したテーブル名のチェックポイントがFirestoreに存在しないとき、Getは初回エクスポートを表すゼロ値のチェックポイントを返す", func(t *testing.T) {
			store := newStore(t)

			got, err := store.Get(ctx, "tst_table_missing")

			require.NoError(t, err)
			require.Equal(t, "tst_table_missing", got.Table)
			require.True(t, got.LastExportTime.IsZero())
			require.Zero(t, got.LastRowCount)
			require.True(t, got.UpdatedAt.IsZero())
		})

		t.Run("Updateで保存したチェックポイントを同じテーブル名でGetすると、保存した最終エクスポート時刻と直近の取り込み行数がそのまま読み出せる", func(t *testing.T) {
			store := newStore(t)
			table := "tst_table_roundtrip"
			lastExportTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			require.NoError(t, store.Update(ctx, table, &model.Checkpoint{
				LastExportTime: lastExportTime,
				LastRowCount:   42,
			}))

			got, err := store.Get(ctx, table)

			require.NoError(t, err)
			require.True(t, lastExportTime.Equal(got.LastExportTime))
			require.Equal(t, int64(42), got.LastRowCount)
		})

		t.Run("Firestoreに保存されたドキュメントの内容がチェックポイントとして解釈できない形式のとき、Getはエラーになる。その後、正しい形式でUpdateしてから同じテーブル名でGetすると成功する", func(t *testing.T) {
			store := newStore(t)
			table := "tst_table_corrupt"
			_, err := rawClient.Collection("export_checkpoints").Doc(table).Set(ctx, map[string]interface{}{
				"table":            table,
				"last_export_time": "not-a-timestamp",
				"last_row_count":   1,
				"updated_at":       time.Now(),
			})
			require.NoError(t, err)

			_, err = store.Get(ctx, table)
			require.Error(t, err)
			require.Contains(t, err.Error(), "parse checkpoint")

			require.NoError(t, store.Update(ctx, table, &model.Checkpoint{LastExportTime: time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), LastRowCount: 7}))
			got, err := store.Get(ctx, table)
			require.NoError(t, err)
			require.Equal(t, int64(7), got.LastRowCount)
		})
	})

	t.Run("[結合テスト]チェックポイントの更新", func(t *testing.T) {
		t.Run("Updateを呼ぶと、そのテーブルのチェックポイントの更新日時は呼び出し時点の時刻になる", func(t *testing.T) {
			store := newStore(t)
			table := "tst_table_updated_at"

			// Firestore はタイムスタンプをマイクロ秒精度に丸めるため、下限もマイクロ秒に切り捨てて比較する
			before := time.Now().Truncate(time.Microsecond)
			require.NoError(t, store.Update(ctx, table, &model.Checkpoint{LastExportTime: time.Now(), LastRowCount: 1}))
			after := time.Now()

			got, err := store.Get(ctx, table)
			require.NoError(t, err)
			require.False(t, got.UpdatedAt.Before(before))
			require.False(t, got.UpdatedAt.After(after))
		})

		t.Run("同じテーブルに対してUpdateを複数回呼ぶと、最新の呼び出し内容で上書きされる", func(t *testing.T) {
			store := newStore(t)
			table := "tst_table_overwrite"

			require.NoError(t, store.Update(ctx, table, &model.Checkpoint{LastExportTime: time.Now(), LastRowCount: 10}))
			require.NoError(t, store.Update(ctx, table, &model.Checkpoint{LastExportTime: time.Now(), LastRowCount: 20}))

			got, err := store.Get(ctx, table)
			require.NoError(t, err)
			require.Equal(t, int64(20), got.LastRowCount)
		})
	})
}

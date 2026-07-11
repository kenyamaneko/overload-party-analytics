package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"export-to-bq/exporter/model"
)

// テスト用モック

type stubSource struct {
	rows []map[string]interface{}
}

func (s *stubSource) Query(_ context.Context, _ model.TableConfig, _, _ time.Time) ([]map[string]interface{}, error) {
	return s.rows, nil
}
func (s *stubSource) Close() {}

type stubStaging struct {
	deleted []string
}

func (s *stubStaging) Write(_ context.Context, _ string, _ []map[string]interface{}, _ time.Time) (string, error) {
	return "gs://stub-bucket/exports/stub.jsonl", nil
}
func (s *stubStaging) Delete(_ context.Context, uri string) error {
	s.deleted = append(s.deleted, uri)
	return nil
}
func (s *stubStaging) Close() error { return nil }

type stubWarehouse struct {
	lastMode model.DedupMode
}

func (s *stubWarehouse) Load(_ context.Context, _ model.TableConfig, _ string, mode model.DedupMode) error {
	s.lastMode = mode
	return nil
}
func (s *stubWarehouse) Close() error { return nil }

type stubCheckpoint struct {
	updateErr error
	updated   bool
}

func (s *stubCheckpoint) Get(_ context.Context, table string) (*model.Checkpoint, error) {
	return &model.Checkpoint{Table: table}, nil
}
func (s *stubCheckpoint) Update(_ context.Context, _ string, _ *model.Checkpoint) error {
	s.updated = true
	return s.updateErr
}
func (s *stubCheckpoint) Close() error { return nil }

func newTestConfig() *model.Config {
	return &model.Config{
		Tables: map[string]model.TableConfig{
			"games": {
				SourceTable:   "games",
				BigQueryTable: "games",
				Query:         "SELECT 1 WHERE $1 < $2",
				NaturalKey:    []string{"game_id"},
			},
		},
		DedupMode: model.DedupModeMerge,
	}
}

func TestExport(t *testing.T) {
	t.Run("エクスポート処理", func(t *testing.T) {
		t.Run("checkpoint 更新が失敗したとき、checkpoint 失敗として結果に反映される", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: []map[string]interface{}{{"game_id": "g1"}}}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{updateErr: errors.New("firestore: UNAVAILABLE")}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"games"}, "incremental", "", "")

			require.Len(t, results, 1)
			r := results[0]
			require.False(t, r.IsSuccess)
			require.True(t, r.IsCheckpointFailed)
			require.Contains(t, r.Error, "checkpoint update failed")
		})

		t.Run("full モードのとき、checkpoint を更新しない", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: []map[string]interface{}{{"game_id": "g1"}}}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{updateErr: errors.New("should not be called")}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"games"}, "full", "2024-01-01", "2024-02-01")

			require.Len(t, results, 1)
			require.True(t, results[0].IsSuccess)
			require.False(t, cp.updated)
		})

		t.Run("エクスポート成功後、ステージングファイルが削除される", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: []map[string]interface{}{{"game_id": "g1"}}}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			_ = svc.Export(context.Background(), []string{"games"}, "incremental", "", "")

			require.Len(t, stg.deleted, 1)
		})
	})
}

func TestResolveTimeRange(t *testing.T) {
	t.Run("時間範囲の解決", func(t *testing.T) {
		checkpoint := time.Date(2025, 3, 10, 3, 0, 0, 0, time.UTC)
		now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

		validCases := []struct {
			name       string
			mode       string
			startDate  string
			endDate    string
			checkpoint time.Time
			wantStart  time.Time
			wantEnd    time.Time
		}{
			{
				name:       "incremental モードのとき、checkpoint から現在までの範囲になる",
				mode:       "incremental",
				checkpoint: checkpoint,
				wantStart:  checkpoint,
				wantEnd:    now,
			},
			{
				name:       "incremental モードで日付を指定しても、checkpoint から現在までの範囲になる",
				mode:       "incremental",
				startDate:  "2024-01-01",
				endDate:    "2024-02-01",
				checkpoint: checkpoint,
				wantStart:  checkpoint,
				wantEnd:    now,
			},
			{
				// checkpoint を渡しても full モードは無視して epoch から始まることを確かめる
				name:       "full モードで日付未指定のとき、ゼロ値から現在までの範囲になる",
				mode:       "full",
				checkpoint: checkpoint,
				wantStart:  time.Time{},
				wantEnd:    now,
			},
			{
				name:      "full モードで start_date のみ指定のとき、start は指定日・end は現在になる",
				mode:      "full",
				startDate: "2024-06-01",
				wantStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				wantEnd:   now,
			},
			{
				name:      "full モードで end_date のみ指定のとき、start はゼロ値・end は指定日になる",
				mode:      "full",
				endDate:   "2024-07-01",
				wantStart: time.Time{},
				wantEnd:   time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
			},
			{
				name:      "full モードで両日付を指定のとき、指定した範囲になる",
				mode:      "full",
				startDate: "2024-06-01",
				endDate:   "2024-07-01",
				wantStart: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC),
				wantEnd:   time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC),
			},
		}
		for _, tt := range validCases {
			t.Run(tt.name, func(t *testing.T) {
				start, end, err := resolveTimeRange(tt.mode, tt.startDate, tt.endDate, tt.checkpoint, now)
				require.NoError(t, err)
				require.True(t, tt.wantStart.Equal(start))
				require.True(t, tt.wantEnd.Equal(end))
			})
		}

		invalidCases := []struct {
			name      string
			startDate string
			endDate   string
		}{
			{
				name:      "full モードで start_date が不正なとき、エラーになる",
				startDate: "not-a-date",
			},
			{
				name:    "full モードで end_date が不正なとき、エラーになる",
				endDate: "bad",
			},
		}
		for _, tt := range invalidCases {
			t.Run(tt.name, func(t *testing.T) {
				_, _, err := resolveTimeRange("full", tt.startDate, tt.endDate, time.Time{}, now)
				require.Error(t, err)
			})
		}
	})
}

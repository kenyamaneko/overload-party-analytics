package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"export-to-bq/exporter/model"
)

// テスト用モック

func stagingURIFor(table string) string {
	return fmt.Sprintf("gs://stub-bucket/exports/%s.jsonl", table)
}

type sourceQuery struct {
	table     string
	startTime time.Time
	endTime   time.Time
}

type stubSource struct {
	rows    []map[string]interface{}
	err     error
	queries []sourceQuery
}

func (s *stubSource) Query(_ context.Context, tableConfig model.TableConfig, startTime, endTime time.Time) ([]map[string]interface{}, error) {
	s.queries = append(s.queries, sourceQuery{table: tableConfig.SourceTable, startTime: startTime, endTime: endTime})
	if s.err != nil {
		return nil, s.err
	}
	return s.rows, nil
}
func (s *stubSource) Close() {}

type stagingWrite struct {
	table    string
	rowCount int
}

type stubStaging struct {
	writeErr  error
	deleteErr error
	written   []stagingWrite
	deleted   []string
}

func (s *stubStaging) Write(_ context.Context, table string, rows []map[string]interface{}, _ time.Time) (string, error) {
	s.written = append(s.written, stagingWrite{table: table, rowCount: len(rows)})
	if s.writeErr != nil {
		return "", s.writeErr
	}
	return stagingURIFor(table), nil
}
func (s *stubStaging) Delete(_ context.Context, uri string) error {
	s.deleted = append(s.deleted, uri)
	return s.deleteErr
}
func (s *stubStaging) Close() error { return nil }

type warehouseLoad struct {
	table   string
	gcsPath string
	mode    model.DedupMode
}

type stubWarehouse struct {
	loadErr error
	loaded  []warehouseLoad
}

func (s *stubWarehouse) Load(_ context.Context, tableConfig model.TableConfig, gcsPath string, mode model.DedupMode) error {
	s.loaded = append(s.loaded, warehouseLoad{table: tableConfig.BigQueryTable, gcsPath: gcsPath, mode: mode})
	if s.loadErr != nil {
		return s.loadErr
	}
	return nil
}
func (s *stubWarehouse) Close() error { return nil }

type stubCheckpoint struct {
	stored       *model.Checkpoint
	getErr       error
	updateErr    error
	gets         []string
	updates      []string
	updateCalled bool
}

func (s *stubCheckpoint) Get(_ context.Context, table string) (*model.Checkpoint, error) {
	s.gets = append(s.gets, table)
	if s.getErr != nil {
		return nil, s.getErr
	}
	if s.stored != nil {
		return s.stored, nil
	}
	return &model.Checkpoint{Table: table}, nil
}
func (s *stubCheckpoint) Update(_ context.Context, table string, cp *model.Checkpoint) error {
	s.updateCalled = true
	s.updates = append(s.updates, table)
	if s.updateErr != nil {
		return s.updateErr
	}
	s.stored = cp
	return nil
}
func (s *stubCheckpoint) Close() error { return nil }

func newTestConfig() *model.Config {
	return &model.Config{
		Tables: map[string]model.TableConfig{
			"tst_table_a": {
				SourceTable:   "tst_table_a",
				BigQueryTable: "tst_table_a",
				Query:         "SELECT 1 WHERE $1 < $2",
				NaturalKey:    []string{"tst_id"},
			},
			"tst_table_b": {
				SourceTable:   "tst_table_b",
				BigQueryTable: "tst_table_b",
				Query:         "SELECT 1 WHERE $1 < $2",
				NaturalKey:    []string{"tst_id"},
			},
		},
		DedupMode: model.DedupModeMerge,
	}
}

func TestExport(t *testing.T) {
	t.Run("エクスポート処理", func(t *testing.T) {
		singleRow := []map[string]interface{}{{"tst_col": "v1"}}
		threeRows := []map[string]interface{}{
			{"tst_col": "v1"},
			{"tst_col": "v2"},
			{"tst_col": "v3"},
		}

		t.Run("対象テーブルが0件のとき、結果は0件になる", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: singleRow}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{}, "incremental", "", "")

			require.Len(t, results, 0)
		})

		t.Run("対象テーブル2件が両方成功するとき、結果は入力順に2件とも成功になる", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: singleRow}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"tst_table_a", "tst_table_b"}, "incremental", "", "")

			require.Len(t, results, 2)
			require.Equal(t, "tst_table_a", results[0].Table)
			require.True(t, results[0].IsSuccess)
			require.Equal(t, "tst_table_b", results[1].Table)
			require.True(t, results[1].IsSuccess)
		})

		t.Run("対象2件のうち1件目が設定に無いとき、1件目は失敗・2件目は成功として入力順に並ぶ", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: singleRow}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"tst_missing", "tst_table_a"}, "incremental", "", "")

			require.Len(t, results, 2)
			require.False(t, results[0].IsSuccess)
			require.Contains(t, results[0].Error, "tst_missing")
			require.Contains(t, results[0].Error, "not found in config")
			require.True(t, results[1].IsSuccess)
		})

		t.Run("読み取り結果が0件のとき、成功となり行数0・ファイルパス無しで完了し、下流は呼ばれない", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: []map[string]interface{}{}}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"tst_table_a"}, "incremental", "", "")

			require.Len(t, results, 1)
			r := results[0]
			require.True(t, r.IsSuccess)
			require.Equal(t, int64(0), r.RowsExported)
			require.Empty(t, r.FilePath)
			require.Empty(t, stg.written)
			require.Empty(t, wh.loaded)
			require.Nil(t, cp.stored)
			require.Empty(t, stg.deleted)
		})

		failureStageCases := []struct {
			name                  string
			table                 string
			mode                  string
			startDate             string
			endDate               string
			rows                  []map[string]interface{}
			checkpointGetErr      error
			sourceErr             error
			stagingWriteErr       error
			warehouseLoadErr      error
			checkpointUpdateErr   error
			wantErrorContains     []string
			wantRowsExported      int64
			wantFilePath          string
			wantCheckpointFailed  bool
			wantCheckpointGets    int
			wantSourceQueries     int
			wantStagingWrites     int
			wantWarehouseLoads    int
			wantCheckpointUpdates int
		}{
			{
				name:               "設定に無いテーブルを指定したとき、失敗となり誤り内容にテーブル名が含まれ、checkpoint取得以降は行われない",
				table:              "tst_missing",
				mode:               "incremental",
				rows:               singleRow,
				wantErrorContains:  []string{"tst_missing", "not found in config"},
				wantRowsExported:   0,
				wantFilePath:       "",
				wantCheckpointGets: 0,
				wantSourceQueries:  0,
				wantStagingWrites:  0,
				wantWarehouseLoads: 0,
			},
			{
				name:               "checkpointの取得に失敗したとき、失敗となり行の読み取りは行われない",
				table:              "tst_table_a",
				mode:               "incremental",
				rows:               singleRow,
				checkpointGetErr:   errors.New("dummy get checkpoint error"),
				wantErrorContains:  []string{"get checkpoint"},
				wantRowsExported:   0,
				wantFilePath:       "",
				wantCheckpointGets: 1,
				wantSourceQueries:  0,
				wantStagingWrites:  0,
				wantWarehouseLoads: 0,
			},
			{
				name:               "行の読み取りに失敗したとき、失敗となりステージングへの書き込みは行われない",
				table:              "tst_table_a",
				mode:               "incremental",
				rows:               singleRow,
				sourceErr:          errors.New("dummy query source error"),
				wantErrorContains:  []string{"query source"},
				wantRowsExported:   0,
				wantFilePath:       "",
				wantCheckpointGets: 1,
				wantSourceQueries:  1,
				wantStagingWrites:  0,
				wantWarehouseLoads: 0,
			},
			{
				name:               "ステージングへの書き込みに失敗したとき、失敗となりウェアハウスへのロードは行われない",
				table:              "tst_table_a",
				mode:               "incremental",
				rows:               singleRow,
				stagingWriteErr:    errors.New("dummy write to staging error"),
				wantErrorContains:  []string{"write to staging"},
				wantRowsExported:   0,
				wantFilePath:       "",
				wantCheckpointGets: 1,
				wantSourceQueries:  1,
				wantStagingWrites:  1,
				wantWarehouseLoads: 0,
			},
			{
				name:               "ウェアハウスへのロードに失敗したとき、失敗となりステージングのファイルパスが結果に残り、checkpointは更新されず、ステージングも削除されない",
				table:              "tst_table_a",
				mode:               "incremental",
				rows:               singleRow,
				warehouseLoadErr:   errors.New("dummy load to warehouse error"),
				wantErrorContains:  []string{"load to warehouse"},
				wantRowsExported:   0,
				wantFilePath:       stagingURIFor("tst_table_a"),
				wantCheckpointGets: 1,
				wantSourceQueries:  1,
				wantStagingWrites:  1,
				wantWarehouseLoads: 1,
			},
			{
				name:               "fullモードで開始日が不正な日付のとき、失敗となりエラー内容に開始日の値が含まれ、行の読み取りは行われない",
				table:              "tst_table_a",
				mode:               "full",
				startDate:          "not-a-date",
				rows:               singleRow,
				wantErrorContains:  []string{"not-a-date"},
				wantRowsExported:   0,
				wantFilePath:       "",
				wantCheckpointGets: 1,
				wantSourceQueries:  0,
				wantStagingWrites:  0,
				wantWarehouseLoads: 0,
			},
			{
				name:                  "checkpointの更新に失敗したとき、失敗となるが行数とファイルパスは結果に残る",
				table:                 "tst_table_a",
				mode:                  "incremental",
				rows:                  threeRows,
				checkpointUpdateErr:   errors.New("firestore: UNAVAILABLE"),
				wantErrorContains:     []string{"checkpoint update failed"},
				wantRowsExported:      3,
				wantFilePath:          stagingURIFor("tst_table_a"),
				wantCheckpointFailed:  true,
				wantCheckpointGets:    1,
				wantSourceQueries:     1,
				wantStagingWrites:     1,
				wantWarehouseLoads:    1,
				wantCheckpointUpdates: 1,
			},
		}
		for _, tt := range failureStageCases {
			t.Run(tt.name, func(t *testing.T) {
				cfg := newTestConfig()
				src := &stubSource{rows: tt.rows, err: tt.sourceErr}
				stg := &stubStaging{writeErr: tt.stagingWriteErr}
				wh := &stubWarehouse{loadErr: tt.warehouseLoadErr}
				cp := &stubCheckpoint{getErr: tt.checkpointGetErr, updateErr: tt.checkpointUpdateErr}

				svc := New(cfg, src, stg, wh, cp)
				results := svc.Export(context.Background(), []string{tt.table}, tt.mode, tt.startDate, tt.endDate)

				require.Len(t, results, 1)
				r := results[0]
				require.False(t, r.IsSuccess)
				for _, substr := range tt.wantErrorContains {
					require.Contains(t, r.Error, substr)
				}
				require.Equal(t, tt.wantRowsExported, r.RowsExported)
				require.Equal(t, tt.wantFilePath, r.FilePath)
				require.Equal(t, tt.wantCheckpointFailed, r.IsCheckpointFailed)

				require.Len(t, cp.gets, tt.wantCheckpointGets)
				for _, g := range cp.gets {
					require.Equal(t, tt.table, g)
				}

				require.Len(t, cp.updates, tt.wantCheckpointUpdates)
				for _, u := range cp.updates {
					require.Equal(t, tt.table, u)
				}
				require.Nil(t, cp.stored)

				require.Len(t, src.queries, tt.wantSourceQueries)
				for _, q := range src.queries {
					require.Equal(t, tt.table, q.table)
					// failureStageCases のどのケースも checkpoint をシードしないため、ゼロ値になる。
					require.True(t, q.startTime.IsZero())
				}

				require.Len(t, stg.written, tt.wantStagingWrites)
				for _, w := range stg.written {
					require.Equal(t, tt.table, w.table)
					require.Equal(t, len(tt.rows), w.rowCount)
				}

				require.Len(t, wh.loaded, tt.wantWarehouseLoads)
				for _, l := range wh.loaded {
					require.Equal(t, tt.table, l.table)
					require.Equal(t, stagingURIFor(tt.table), l.gcsPath)
				}

				require.Empty(t, stg.deleted)
			})
		}

		t.Run("ステージングの削除に失敗したとき、エクスポートは成功のままでcheckpointは更新済みになる", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: singleRow}
			stg := &stubStaging{deleteErr: errors.New("dummy delete error")}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"tst_table_a"}, "incremental", "", "")

			require.Len(t, results, 1)
			require.True(t, results[0].IsSuccess)
			require.Equal(t, int64(1), results[0].RowsExported)
			require.NotNil(t, cp.stored)
			require.Equal(t, int64(1), cp.stored.LastRowCount)
		})

		dedupModeCases := []struct {
			name      string
			dedupMode model.DedupMode
			wantMode  model.DedupMode
		}{
			{
				name:      "重複排除モードが未設定のとき、mergeとしてロードされる",
				dedupMode: "",
				wantMode:  model.DedupModeMerge,
			},
			{
				name:      "重複排除モードがappendのとき、appendとしてロードされる",
				dedupMode: model.DedupModeAppend,
				wantMode:  model.DedupModeAppend,
			},
			{
				name:      "重複排除モードがmergeのとき、mergeとしてロードされる",
				dedupMode: model.DedupModeMerge,
				wantMode:  model.DedupModeMerge,
			},
		}
		for _, tt := range dedupModeCases {
			t.Run(tt.name, func(t *testing.T) {
				cfg := newTestConfig()
				cfg.DedupMode = tt.dedupMode
				src := &stubSource{rows: singleRow}
				stg := &stubStaging{}
				wh := &stubWarehouse{}
				cp := &stubCheckpoint{}

				svc := New(cfg, src, stg, wh, cp)
				svc.Export(context.Background(), []string{"tst_table_a"}, "incremental", "", "")

				require.Len(t, wh.loaded, 1)
				require.Equal(t, tt.wantMode, wh.loaded[0].mode)
			})
		}

		t.Run("1行エクスポートしたとき、結果の行数は1になりファイルパスはステージングの書き込み先になる", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: singleRow}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"tst_table_a"}, "incremental", "", "")

			require.Len(t, results, 1)
			require.Equal(t, int64(1), results[0].RowsExported)
			require.Equal(t, stagingURIFor("tst_table_a"), results[0].FilePath)
		})

		t.Run("3行エクスポートしたとき、結果の行数は3になりcheckpointにも行数3と実行時刻が保存される", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: threeRows}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			before := time.Now()
			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"tst_table_a"}, "incremental", "", "")
			after := time.Now()

			require.Len(t, results, 1)
			require.Equal(t, int64(3), results[0].RowsExported)
			require.NotNil(t, cp.stored)
			require.Equal(t, int64(3), cp.stored.LastRowCount)
			require.False(t, cp.stored.LastExportTime.Before(before))
			require.False(t, cp.stored.LastExportTime.After(after))
		})

		t.Run("fullモードのとき、checkpointを更新しない", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: singleRow}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			results := svc.Export(context.Background(), []string{"tst_table_a"}, "full", "2024-01-01", "2024-02-01")

			require.Len(t, results, 1)
			require.True(t, results[0].IsSuccess)
			require.False(t, cp.updateCalled)
		})

		t.Run("エクスポート成功後、ステージングファイルが削除される", func(t *testing.T) {
			cfg := newTestConfig()
			src := &stubSource{rows: singleRow}
			stg := &stubStaging{}
			wh := &stubWarehouse{}
			cp := &stubCheckpoint{}

			svc := New(cfg, src, stg, wh, cp)
			svc.Export(context.Background(), []string{"tst_table_a"}, "incremental", "", "")

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

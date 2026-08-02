package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"export-to-bq/exporter/model"
)

// ErrCheckpointUpdate は BQ ロード成功後の checkpoint 書き込み失敗を示します。
// 呼び出し元は HTTP 500 にエスカレーションする必要があります。
var ErrCheckpointUpdate = errors.New("checkpoint update failed after warehouse load")

// ErrRowCountMismatch はソースからの読み出し件数とウェアハウスへの取り込み件数の不一致を示します。
var ErrRowCountMismatch = errors.New("warehouse row count does not match rows read from source")

// Service はエクスポート処理のインターフェースです。
type Service interface {
	Export(ctx context.Context, tables []string, mode string, startDate, endDate string) []model.ExportResult
	Close()
}

type exporter struct {
	config     *model.Config
	source     SourceReader
	staging    StagingWriter
	warehouse  WarehouseLoader
	checkpoint CheckpointStore
}

// New は新しい Service インスタンスを生成します。
func New(config *model.Config, source SourceReader, staging StagingWriter, warehouse WarehouseLoader, checkpoint CheckpointStore) Service {
	return &exporter{
		config:     config,
		source:     source,
		staging:    staging,
		warehouse:  warehouse,
		checkpoint: checkpoint,
	}
}

// Export は指定テーブルのエクスポート処理を実行します。
func (e *exporter) Export(ctx context.Context, tables []string, mode string, startDate, endDate string) []model.ExportResult {
	results := make([]model.ExportResult, 0, len(tables))

	for _, table := range tables {
		result := model.ExportResult{
			Table:     table,
			StartTime: time.Now(),
		}

		rowCount, filePath, err := e.exportTable(ctx, table, mode, startDate, endDate)
		if err != nil {
			result.IsSuccess = false
			result.Error = err.Error()
			result.RowsExported = rowCount
			result.FilePath = filePath
			if errors.Is(err, ErrCheckpointUpdate) {
				result.IsCheckpointFailed = true
			}
		} else {
			result.IsSuccess = true
			result.RowsExported = rowCount
			result.FilePath = filePath
		}

		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		results = append(results, result)
	}

	return results
}

// exportTable は単一テーブルをエクスポートし、行数とステージングパスを返します。
func (e *exporter) exportTable(ctx context.Context, table string, mode string, startDate, endDate string) (int64, string, error) {
	tableConfig, ok := e.config.Tables[table]
	if !ok {
		return 0, "", fmt.Errorf("table %s not found in config", table)
	}

	checkpoint, err := e.checkpoint.Get(ctx, table)
	if err != nil {
		return 0, "", fmt.Errorf("get checkpoint: %w", err)
	}

	startTime, endTime, err := resolveTimeRange(mode, startDate, endDate, checkpoint.LastExportTime, time.Now())
	if err != nil {
		return 0, "", fmt.Errorf("resolve time range: %w", err)
	}

	slog.Info("exporting table", "table", table, "start_time", startTime, "end_time", endTime)

	rows, err := e.source.Query(ctx, tableConfig, startTime, endTime)
	if err != nil {
		return 0, "", fmt.Errorf("query source: %w", err)
	}

	if len(rows) == 0 {
		slog.Info("no new rows", "table", table)
		return 0, "", nil
	}

	slog.Info("read rows", "rows", len(rows), "table", table)

	stagingURI, err := e.staging.Write(ctx, table, rows, endTime)
	if err != nil {
		return 0, "", fmt.Errorf("write to staging: %w", err)
	}

	slog.Info("staged", "staging_uri", stagingURI)

	dedupMode := e.config.DedupMode
	loadedRows, err := e.warehouse.Load(ctx, tableConfig, stagingURI, dedupMode)
	if err != nil {
		return 0, stagingURI, fmt.Errorf("load to warehouse: %w", err)
	}

	slog.Info("loaded into warehouse", "warehouse_table", tableConfig.BigQueryTable, "dedup", dedupMode, "loaded_rows", loadedRows)

	rowCount := int64(len(rows))
	if loadedRows != rowCount {
		return 0, stagingURI, fmt.Errorf("%w: table=%s: read %d rows, warehouse reported %d", ErrRowCountMismatch, table, rowCount, loadedRows)
	}

	// full mode (backfill) では checkpoint を上書きしない。
	// checkpoint 更新失敗は BQ ロード済みのためハードエラーとして扱う。
	if mode != "full" {
		checkpoint.LastExportTime = endTime
		checkpoint.LastRowCount = rowCount
		if err := e.checkpoint.Update(ctx, table, checkpoint); err != nil {
			return rowCount, stagingURI, fmt.Errorf("%w: table=%s: %v", ErrCheckpointUpdate, table, err)
		}
	}

	// ロード完了後にステージングオブジェクトを削除。
	// 削除失敗はログのみ（GCS lifecycle policy で回収される）。
	if err := e.staging.Delete(ctx, stagingURI); err != nil {
		slog.Warn("staging cleanup failed", "staging_uri", stagingURI, "error", err)
	} else {
		slog.Info("staging object deleted", "staging_uri", stagingURI)
	}

	return rowCount, stagingURI, nil
}

// resolveTimeRange はモードとパラメータからクエリ時間範囲を決定します。
func resolveTimeRange(mode string, startDate, endDate string, checkpointTime, now time.Time) (time.Time, time.Time, error) {
	startTime := checkpointTime
	endTime := now

	if mode == "full" {
		startTime = time.Time{}
		if startDate != "" {
			t, err := time.Parse("2006-01-02", startDate)
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid start_date %q: %w", startDate, err)
			}
			startTime = t
		}
		if endDate != "" {
			t, err := time.Parse("2006-01-02", endDate)
			if err != nil {
				return time.Time{}, time.Time{}, fmt.Errorf("invalid end_date %q: %w", endDate, err)
			}
			endTime = t
		}
	}

	return startTime, endTime, nil
}

// Close は全アダプターをクローズします。
func (e *exporter) Close() {
	e.source.Close()
	if err := e.staging.Close(); err != nil {
		slog.Warn("failed to close staging writer", "error", err)
	}
	if err := e.warehouse.Close(); err != nil {
		slog.Warn("failed to close warehouse loader", "error", err)
	}
	if err := e.checkpoint.Close(); err != nil {
		slog.Warn("failed to close checkpoint store", "error", err)
	}
}

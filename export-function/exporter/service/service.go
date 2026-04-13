package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"export-to-bq/exporter/model"
)

// ErrCheckpointUpdate は BQ ロード成功後の checkpoint 書き込み失敗を示します。
// 呼び出し元は HTTP 500 にエスカレーションする必要があります。
var ErrCheckpointUpdate = errors.New("checkpoint update failed after warehouse load")

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
			result.Success = false
			result.Error = err.Error()
			result.RowsExported = rowCount
			result.FilePath = filePath
			if errors.Is(err, ErrCheckpointUpdate) {
				result.CheckpointFailed = true
			}
			log.Printf("ERROR: failed to export table %s: %v", table, err)
		} else {
			result.Success = true
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
	tableConfig, exists := e.config.Tables[table]
	if !exists {
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

	log.Printf("INFO: Exporting %s from %v to %v", table, startTime, endTime)

	rows, err := e.source.Query(ctx, tableConfig, startTime, endTime)
	if err != nil {
		return 0, "", fmt.Errorf("query source: %w", err)
	}

	if len(rows) == 0 {
		log.Printf("INFO: No new rows for table %s", table)
		return 0, "", nil
	}

	log.Printf("INFO: Read %d rows for table %s", len(rows), table)

	stagingURI, err := e.staging.Write(ctx, table, rows, endTime)
	if err != nil {
		return 0, "", fmt.Errorf("write to staging: %w", err)
	}

	log.Printf("INFO: Staged: %s", stagingURI)

	dedupMode := e.resolveDedupMode(mode)
	if err := e.warehouse.Load(ctx, tableConfig, stagingURI, dedupMode); err != nil {
		return 0, stagingURI, fmt.Errorf("load to warehouse: %w", err)
	}

	log.Printf("INFO: Loaded into warehouse table %s (dedup=%s)", tableConfig.BigQueryTable, dedupMode)

	rowCount := int64(len(rows))

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
		log.Printf("WARN: staging cleanup failed for %s: %v (non-fatal; bucket lifecycle will reap it)", stagingURI, err)
	} else {
		log.Printf("INFO: Staging object deleted: %s", stagingURI)
	}

	return rowCount, stagingURI, nil
}

// resolveDedupMode は設定に基づいて dedup モードを決定します。
func (e *exporter) resolveDedupMode(mode string) model.DedupMode {
	if e.config.DedupMode != "" {
		return e.config.DedupMode
	}
	return model.DedupModeMerge
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
		log.Printf("WARN: failed to close staging writer: %v", err)
	}
	if err := e.warehouse.Close(); err != nil {
		log.Printf("WARN: failed to close warehouse loader: %v", err)
	}
	if err := e.checkpoint.Close(); err != nil {
		log.Printf("WARN: failed to close checkpoint store: %v", err)
	}
}

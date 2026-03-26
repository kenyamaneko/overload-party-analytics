package service

import (
	"context"
	"fmt"
	"log"
	"time"

	"export-to-bq/exporter/model"
)

// Service defines the interface for the export operation.
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

// New creates a new Service instance.
func New(config *model.Config, source SourceReader, staging StagingWriter, warehouse WarehouseLoader, checkpoint CheckpointStore) Service {
	return &exporter{
		config:     config,
		source:     source,
		staging:    staging,
		warehouse:  warehouse,
		checkpoint: checkpoint,
	}
}

// Export executes the export process for the specified tables.
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

// exportTable exports a single table and returns the row count and staging path.
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

	if err := e.warehouse.Load(ctx, tableConfig, stagingURI); err != nil {
		return 0, "", fmt.Errorf("load to warehouse: %w", err)
	}

	log.Printf("INFO: Loaded into warehouse table %s", tableConfig.BigQueryTable)

	rowCount := int64(len(rows))

	// Update checkpoint only in incremental mode.
	// Full mode (backfill) must not overwrite the checkpoint to avoid
	// regressing it to a historical date.
	if mode != "full" {
		checkpoint.LastExportTime = endTime
		checkpoint.LastRowCount = rowCount
		if err := e.checkpoint.Update(ctx, table, checkpoint); err != nil {
			// BQ へのロードは完了済みのため処理自体は成功扱いにする。
			// チェックポイントが古いまま残るため、次回実行で同じ範囲が
			// 再インポートされ重複が発生する可能性がある。
			log.Printf("WARN: checkpoint update failed for %s (BQ load already succeeded, manual checkpoint update may be needed): %v", table, err)
		}
	}

	return rowCount, stagingURI, nil
}

// resolveTimeRange determines the query time range based on mode and parameters.
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

// Close closes all adapters.
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

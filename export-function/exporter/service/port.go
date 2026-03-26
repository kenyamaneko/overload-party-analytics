package service

import (
	"context"
	"time"

	"export-to-bq/exporter/model"
)

// SourceReader reads rows from the source database.
type SourceReader interface {
	Query(ctx context.Context, tableConfig model.TableConfig, startTime, endTime time.Time) ([]map[string]interface{}, error)
	Close()
}

// StagingWriter writes rows to the staging area for warehouse ingestion.
type StagingWriter interface {
	Write(ctx context.Context, table string, rows []map[string]interface{}, timestamp time.Time) (string, error)
	Close() error
}

// WarehouseLoader loads staged data into the data warehouse.
type WarehouseLoader interface {
	Load(ctx context.Context, tableConfig model.TableConfig, stagingURI string) error
	Close() error
}

// CheckpointStore manages export checkpoints.
type CheckpointStore interface {
	Get(ctx context.Context, table string) (*model.Checkpoint, error)
	Update(ctx context.Context, table string, cp *model.Checkpoint) error
	Close() error
}

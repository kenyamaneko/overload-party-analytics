package service

import (
	"context"
	"time"

	"export-to-bq/exporter/model"
)

// SourceReader はソースデータベースから行を読み取ります。
type SourceReader interface {
	Query(ctx context.Context, tableConfig model.TableConfig, startTime, endTime time.Time) ([]map[string]interface{}, error)
	Close()
}

// StagingWriter はウェアハウス取り込み用のステージング領域に行を書き込みます。
type StagingWriter interface {
	Write(ctx context.Context, table string, rows []map[string]interface{}, timestamp time.Time) (string, error)
	// Delete はウェアハウスロード完了後にステージングオブジェクトを削除します。
	Delete(ctx context.Context, uri string) error
	Close() error
}

// WarehouseLoader はステージングデータをデータウェアハウスにロードします。
type WarehouseLoader interface {
	// Load はロードを行い、ウェアハウスに取り込まれた行数を返します。
	Load(ctx context.Context, tableConfig model.TableConfig, stagingURI string, dedupMode model.DedupMode) (int64, error)
	Close() error
}

// CheckpointStore はエクスポートのチェックポイントを管理します。
type CheckpointStore interface {
	Get(ctx context.Context, table string) (*model.Checkpoint, error)
	Update(ctx context.Context, table string, cp *model.Checkpoint) error
	Close() error
}

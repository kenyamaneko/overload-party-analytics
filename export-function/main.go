package exportfunction

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"

	"export-to-bq/exporter/adapter"
	"export-to-bq/exporter/model"
	"export-to-bq/exporter/service"
	"export-to-bq/handler"
)

func init() {
	slog.SetDefault(slog.New(newCloudLoggingHandler()).With("service", "analytics"))
	functions.HTTP("ExportPostgresToBigQuery", handler.New(newExporterService))
}

// newCloudLoggingHandler は Cloud Logging に適合するログハンドラを生成する。
func newCloudLoggingHandler() slog.Handler {
	return slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				a.Key = "severity"
				if level, ok := a.Value.Any().(slog.Level); ok {
					switch {
					case level >= slog.LevelError:
						a.Value = slog.StringValue("ERROR")
					case level >= slog.LevelWarn:
						a.Value = slog.StringValue("WARNING")
					case level >= slog.LevelInfo:
						a.Value = slog.StringValue("INFO")
					default:
						a.Value = slog.StringValue("DEBUG")
					}
				}
			}
			if a.Key == slog.MessageKey {
				a.Key = "message"
			}
			return a
		},
	})
}

func newExporterService(ctx context.Context) (_ service.Service, retErr error) {
	config, err := model.LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	config.InstanceConnectionName = os.Getenv("INSTANCE_CONNECTION_NAME")
	config.DBHost = os.Getenv("DB_HOST")
	config.DBUser = os.Getenv("DB_USER")
	config.DBPassword = os.Getenv("DB_PASSWORD")
	config.DBName = os.Getenv("DB_NAME")
	config.BQProjectID = os.Getenv("BQ_PROJECT_ID")
	config.BQDatasetID = os.Getenv("BQ_DATASET_ID")
	config.GCSBucket = os.Getenv("GCS_BUCKET")

	// DEDUP_MODE: デフォルト "merge" で incremental run を冪等にする。
	// 空 dataset への backfill 時は DEDUP_MODE=append で MERGE をスキップ可能。
	switch mode := os.Getenv("DEDUP_MODE"); mode {
	case "", string(model.DedupModeMerge):
		config.DedupMode = model.DedupModeMerge
	case string(model.DedupModeAppend):
		config.DedupMode = model.DedupModeAppend
	default:
		return nil, fmt.Errorf("invalid DEDUP_MODE %q (want merge or append)", mode)
	}

	if config.DBUser == "" || config.BQProjectID == "" || config.BQDatasetID == "" || config.GCSBucket == "" {
		return nil, fmt.Errorf("missing required environment variables (DB_USER, BQ_PROJECT_ID, BQ_DATASET_ID, GCS_BUCKET)")
	}

	if config.DBName == "" {
		config.DBName = "overload_party"
	}

	source, err := adapter.NewPostgresReader(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create source reader: %w", err)
	}
	defer func() {
		if retErr != nil {
			source.Close()
		}
	}()

	staging, err := adapter.NewGCSWriter(ctx, config.GCSBucket)
	if err != nil {
		return nil, fmt.Errorf("create staging writer: %w", err)
	}
	defer func() {
		if retErr != nil {
			if closeErr := staging.Close(); closeErr != nil {
				slog.Warn("failed to close staging writer during cleanup", "error", closeErr)
			}
		}
	}()

	warehouse, err := adapter.NewBQLoader(ctx, config.BQProjectID, config.BQDatasetID)
	if err != nil {
		return nil, fmt.Errorf("create warehouse loader: %w", err)
	}
	defer func() {
		if retErr != nil {
			if closeErr := warehouse.Close(); closeErr != nil {
				slog.Warn("failed to close warehouse loader during cleanup", "error", closeErr)
			}
		}
	}()

	checkpoint, err := adapter.NewFirestoreCheckpoint(ctx, config.BQProjectID)
	if err != nil {
		return nil, fmt.Errorf("create checkpoint store: %w", err)
	}

	return service.New(config, source, staging, warehouse, checkpoint), nil
}

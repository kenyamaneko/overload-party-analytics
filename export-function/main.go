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

// newCloudLoggingHandler は Cloud Logging が severity として解釈できる形式でログを出力するハンドラを返す。
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

// applyEnvConfig は環境変数から実行時設定を読み込みます。
// 未設定・解釈できない値は起動時に失敗させ、既定値へのフォールバックは行いません。
func applyEnvConfig(config *model.Config) error {
	config.DatabaseConn = os.Getenv("DATABASE_CONN")
	config.BQProjectID = os.Getenv("BQ_PROJECT_ID")
	config.BQDatasetID = os.Getenv("BQ_DATASET_ID")
	config.GCSBucket = os.Getenv("GCS_BUCKET")

	if config.DatabaseConn == "" || config.BQProjectID == "" || config.BQDatasetID == "" || config.GCSBucket == "" {
		return fmt.Errorf("missing required environment variables (DATABASE_CONN, BQ_PROJECT_ID, BQ_DATASET_ID, GCS_BUCKET)")
	}

	rawIAMAuth := os.Getenv("DATABASE_IAM_AUTH_ENABLED")
	switch rawIAMAuth {
	case "true":
		config.DatabaseIAMAuthEnabled = true
	case "false":
		config.DatabaseIAMAuthEnabled = false
	default:
		return fmt.Errorf("DATABASE_IAM_AUTH_ENABLED must be %q or %q, got %q", "true", "false", rawIAMAuth)
	}

	if config.DatabaseIAMAuthEnabled {
		config.CloudSQLConnectionName = os.Getenv("CLOUDSQL_CONNECTION_NAME")
		if config.CloudSQLConnectionName == "" {
			return fmt.Errorf("CLOUDSQL_CONNECTION_NAME is required when DATABASE_IAM_AUTH_ENABLED is true")
		}
	}

	return nil
}

func newExporterService(ctx context.Context) (_ service.Service, retErr error) {
	config, err := model.LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	if err := applyEnvConfig(config); err != nil {
		return nil, err
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

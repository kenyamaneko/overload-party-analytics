package exportfunction

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"

	"export-to-bq/exporter/adapter"
	"export-to-bq/exporter/model"
	"export-to-bq/exporter/service"
	"export-to-bq/handler"
)

func init() {
	functions.HTTP("ExportPostgresToBigQuery", handler.New(newExporterService))
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
				log.Printf("WARN: failed to close staging writer during cleanup: %v", closeErr)
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
				log.Printf("WARN: failed to close warehouse loader during cleanup: %v", closeErr)
			}
		}
	}()

	checkpoint, err := adapter.NewFirestoreCheckpoint(ctx, config.BQProjectID)
	if err != nil {
		return nil, fmt.Errorf("create checkpoint store: %w", err)
	}

	return service.New(config, source, staging, warehouse, checkpoint), nil
}

package exporter

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/firestore"
	"cloud.google.com/go/storage"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Exporter handles the export process from PostgreSQL to BigQuery
type Exporter struct {
	config          *Config
	pgPool          *pgxpool.Pool
	gcsClient       *storage.Client
	bqClient        *bigquery.Client
	firestoreClient *firestore.Client
	checkpointStore *CheckpointStore
}

// ExportResult represents the result of a single table export
type ExportResult struct {
	Table        string        `json:"table"`
	RowsExported int64         `json:"rows_exported"`
	FilePath     string        `json:"file_path,omitempty"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
}

// New creates a new Exporter instance
func New(ctx context.Context) (*Exporter, error) {
	config, err := LoadConfig("config.yaml")
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	// Environment variables
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

	// Create PostgreSQL connection pool
	pgPool, err := newPgPool(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("create pg pool: %w", err)
	}

	// Create GCS client
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		pgPool.Close()
		return nil, fmt.Errorf("create gcs client: %w", err)
	}

	// Create BigQuery client
	bqClient, err := bigquery.NewClient(ctx, config.BQProjectID)
	if err != nil {
		pgPool.Close()
		gcsClient.Close()
		return nil, fmt.Errorf("create bigquery client: %w", err)
	}

	// Create Firestore client
	firestoreClient, err := firestore.NewClient(ctx, config.BQProjectID)
	if err != nil {
		pgPool.Close()
		gcsClient.Close()
		bqClient.Close()
		return nil, fmt.Errorf("create firestore client: %w", err)
	}

	checkpointStore := NewCheckpointStore(firestoreClient)

	return &Exporter{
		config:          config,
		pgPool:          pgPool,
		gcsClient:       gcsClient,
		bqClient:        bqClient,
		firestoreClient: firestoreClient,
		checkpointStore: checkpointStore,
	}, nil
}

// Export executes the export process for the specified tables
func (e *Exporter) Export(ctx context.Context, tables []string, mode string) []ExportResult {
	results := make([]ExportResult, 0, len(tables))

	for _, table := range tables {
		result := ExportResult{
			Table:     table,
			StartTime: time.Now(),
		}

		rowCount, err := e.exportTable(ctx, table, mode)
		if err != nil {
			result.Success = false
			result.Error = err.Error()
			log.Printf("ERROR: failed to export table %s: %v", table, err)
		} else {
			result.Success = true
			result.RowsExported = rowCount
		}

		result.EndTime = time.Now()
		result.Duration = result.EndTime.Sub(result.StartTime)
		results = append(results, result)
	}

	return results
}

// exportTable exports a single table and returns the number of rows exported
func (e *Exporter) exportTable(ctx context.Context, table string, mode string) (int64, error) {
	tableConfig, exists := e.config.Tables[table]
	if !exists {
		return 0, fmt.Errorf("table %s not found in config", table)
	}

	// Get last checkpoint
	checkpoint, err := e.checkpointStore.Get(ctx, table)
	if err != nil {
		return 0, fmt.Errorf("get checkpoint: %w", err)
	}

	// Determine time range
	startTime := checkpoint.LastExportTime
	if mode == "full" {
		startTime = time.Time{}
	}
	endTime := time.Now()

	log.Printf("INFO: Exporting %s from %v to %v", table, startTime, endTime)

	// Read from PostgreSQL
	rows, err := e.queryPostgres(ctx, tableConfig, startTime, endTime)
	if err != nil {
		return 0, fmt.Errorf("query postgres: %w", err)
	}

	if len(rows) == 0 {
		log.Printf("INFO: No new rows for table %s", table)
		return 0, nil
	}

	log.Printf("INFO: Read %d rows from PostgreSQL for table %s", len(rows), table)

	// Write to GCS
	gcsPath, err := e.writeToGCS(ctx, table, rows, endTime)
	if err != nil {
		return 0, fmt.Errorf("write to gcs: %w", err)
	}

	log.Printf("INFO: Wrote to GCS: %s", gcsPath)

	// Load into BigQuery
	if err := e.loadToBigQuery(ctx, tableConfig, gcsPath); err != nil {
		return 0, fmt.Errorf("load to bigquery: %w", err)
	}

	log.Printf("INFO: Loaded into BigQuery table %s", tableConfig.BigQueryTable)

	// Update checkpoint
	rowCount := int64(len(rows))
	checkpoint.LastExportTime = endTime
	checkpoint.LastRowCount = rowCount
	if err := e.checkpointStore.Update(ctx, table, checkpoint); err != nil {
		return 0, fmt.Errorf("update checkpoint: %w", err)
	}

	log.Printf("SUCCESS: Exported %d rows from %s", rowCount, table)
	return rowCount, nil
}

// Close closes all clients
func (e *Exporter) Close() {
	if e.pgPool != nil {
		e.pgPool.Close()
	}
	if e.gcsClient != nil {
		e.gcsClient.Close()
	}
	if e.bqClient != nil {
		e.bqClient.Close()
	}
	if e.firestoreClient != nil {
		e.firestoreClient.Close()
	}
}

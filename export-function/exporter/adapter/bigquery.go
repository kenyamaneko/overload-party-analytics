package adapter

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"

	"export-to-bq/exporter/model"
)

// BQLoader loads data from GCS into BigQuery.
type BQLoader struct {
	client    *bigquery.Client
	datasetID string
}

// NewBQLoader creates a new BQLoader.
func NewBQLoader(ctx context.Context, projectID, datasetID string) (*BQLoader, error) {
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create BigQuery client: %w", err)
	}
	return &BQLoader{client: client, datasetID: datasetID}, nil
}

// Load loads data from a GCS path into a BigQuery table.
func (l *BQLoader) Load(ctx context.Context, tableConfig model.TableConfig, gcsPath string) error {
	dataset := l.client.Dataset(l.datasetID)
	table := dataset.Table(tableConfig.BigQueryTable)

	gcsRef := bigquery.NewGCSReference(gcsPath)
	gcsRef.SourceFormat = bigquery.JSON
	gcsRef.AutoDetect = false
	gcsRef.MaxBadRecords = 0

	loader := table.LoaderFrom(gcsRef)
	loader.WriteDisposition = bigquery.WriteAppend

	job, err := loader.Run(ctx)
	if err != nil {
		return fmt.Errorf("start load job: %w", err)
	}

	status, err := job.Wait(ctx)
	if err != nil {
		return fmt.Errorf("wait for job: %w", err)
	}

	if err := status.Err(); err != nil {
		return fmt.Errorf("load job failed: %w", err)
	}

	return nil
}

// Close closes the underlying BigQuery client.
func (l *BQLoader) Close() error {
	return l.client.Close()
}

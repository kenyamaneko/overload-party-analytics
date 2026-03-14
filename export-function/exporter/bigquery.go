package exporter

import (
	"context"
	"fmt"

	"cloud.google.com/go/bigquery"
)

// loadToBigQuery loads data from GCS into BigQuery
func (e *Exporter) loadToBigQuery(ctx context.Context, tableConfig TableConfig, gcsPath string) error {
	dataset := e.bqClient.Dataset(e.config.BQDatasetID)
	table := dataset.Table(tableConfig.BigQueryTable)

	gcsRef := bigquery.NewGCSReference(gcsPath)
	gcsRef.SourceFormat = bigquery.JSON
	gcsRef.AutoDetect = false
	gcsRef.MaxBadRecords = 0 // Fail on any bad record

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

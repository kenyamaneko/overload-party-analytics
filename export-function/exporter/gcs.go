package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// writeToGCS writes rows to GCS in JSONL format
func (e *Exporter) writeToGCS(ctx context.Context, table string, rows []map[string]interface{}, timestamp time.Time) (string, error) {
	// Build GCS path: exports/YYYYMMDD/table/HHmmss.jsonl
	datePath := timestamp.Format("20060102")
	timePath := timestamp.Format("150405")
	objectPath := fmt.Sprintf("exports/%s/%s/%s.jsonl", datePath, table, timePath)

	bucket := e.gcsClient.Bucket(e.config.GCSBucket)
	obj := bucket.Object(objectPath)
	writer := obj.NewWriter(ctx)
	writer.ContentType = "application/jsonl"

	// Write each row as a JSON line
	encoder := json.NewEncoder(writer)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			writer.Close()
			return "", fmt.Errorf("encode row: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("close writer: %w", err)
	}

	gcsPath := fmt.Sprintf("gs://%s/%s", e.config.GCSBucket, objectPath)
	return gcsPath, nil
}

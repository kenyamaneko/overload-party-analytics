package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"cloud.google.com/go/storage"
)

// GCSWriter writes data to Google Cloud Storage.
type GCSWriter struct {
	client *storage.Client
	bucket string
}

// NewGCSWriter creates a new GCSWriter.
func NewGCSWriter(ctx context.Context, bucket string) (*GCSWriter, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	return &GCSWriter{client: client, bucket: bucket}, nil
}

// Write writes rows to GCS in JSONL format and returns the gs:// path.
func (w *GCSWriter) Write(ctx context.Context, table string, rows []map[string]interface{}, timestamp time.Time) (string, error) {
	objectPath := gcsObjectPath(table, timestamp)

	obj := w.client.Bucket(w.bucket).Object(objectPath)

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	writer := obj.NewWriter(wctx)
	writer.ContentType = "application/jsonl"

	encoder := json.NewEncoder(writer)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			cancel()
			// cancel 済みなので Close は中止を確定させるだけ（エラーは想定内）
			_ = writer.Close()
			return "", fmt.Errorf("encode row: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("upload to GCS: %w", err)
	}

	return fmt.Sprintf("gs://%s/%s", w.bucket, objectPath), nil
}

// Close closes the underlying GCS client.
func (w *GCSWriter) Close() error {
	return w.client.Close()
}

// gcsObjectPath builds the GCS object path: exports/YYYYMMDD/table/HHmmss.SSS.jsonl
func gcsObjectPath(table string, timestamp time.Time) string {
	datePath := timestamp.Format("20060102")
	timePath := timestamp.Format("150405.000")
	return fmt.Sprintf("exports/%s/%s/%s.jsonl", datePath, table, timePath)
}

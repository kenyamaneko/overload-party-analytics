package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/storage"
)

// GCSWriter は Google Cloud Storage にデータを書き込みます。
type GCSWriter struct {
	client *storage.Client
	bucket string
}

// NewGCSWriter は新しい GCSWriter を生成します。
func NewGCSWriter(ctx context.Context, bucket string) (*GCSWriter, error) {
	client, err := storage.NewClient(ctx)
	if err != nil {
		return nil, fmt.Errorf("create GCS client: %w", err)
	}
	return &GCSWriter{client: client, bucket: bucket}, nil
}

// Write は行を JSONL 形式で GCS に書き込み、gs:// パスを返します。
func (w *GCSWriter) Write(ctx context.Context, table string, rows []map[string]interface{}, timestamp time.Time) (string, error) {
	objectPath := buildGCSObjectPath(table, timestamp)

	obj := w.client.Bucket(w.bucket).Object(objectPath)

	wctx, cancel := context.WithCancel(ctx)
	defer cancel()

	writer := obj.NewWriter(wctx)
	writer.ContentType = "application/jsonl"

	encoder := json.NewEncoder(writer)
	for _, row := range rows {
		if err := encoder.Encode(row); err != nil {
			cancel()
			// cancel 済みのため Close のエラーは想定内
			_ = writer.Close()
			return "", fmt.Errorf("encode row: %w", err)
		}
	}

	if err := writer.Close(); err != nil {
		return "", fmt.Errorf("upload to GCS: %w", err)
	}

	return fmt.Sprintf("gs://%s/%s", w.bucket, objectPath), nil
}

// Delete は gs:// URI で指定されたステージングオブジェクトを削除します。
func (w *GCSWriter) Delete(ctx context.Context, uri string) error {
	objectPath, err := w.parseObjectPath(uri)
	if err != nil {
		return err
	}

	if err := w.client.Bucket(w.bucket).Object(objectPath).Delete(ctx); err != nil {
		return fmt.Errorf("delete staging object %s: %w", uri, err)
	}
	return nil
}

// Close は GCS クライアントをクローズします。
func (w *GCSWriter) Close() error {
	return w.client.Close()
}

// parseObjectPath は gs:// URI からオブジェクトパスを抽出し、バケットの一致を検証します。
func (w *GCSWriter) parseObjectPath(uri string) (string, error) {
	prefix := fmt.Sprintf("gs://%s/", w.bucket)
	if !strings.HasPrefix(uri, prefix) {
		return "", fmt.Errorf("uri %q does not belong to bucket %q", uri, w.bucket)
	}
	return strings.TrimPrefix(uri, prefix), nil
}

// buildGCSObjectPath は GCS オブジェクトパス (exports/YYYYMMDD/table/HHmmss.SSS.jsonl) を構築します。
func buildGCSObjectPath(table string, timestamp time.Time) string {
	datePath := timestamp.Format("20060102")
	timePath := timestamp.Format("150405.000")
	return fmt.Sprintf("exports/%s/%s/%s.jsonl", datePath, table, timePath)
}

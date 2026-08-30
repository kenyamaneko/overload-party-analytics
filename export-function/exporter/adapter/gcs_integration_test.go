//go:build integration

package adapter

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"cloud.google.com/go/storage"
	"github.com/fsouza/fake-gcs-server/fakestorage"
	"github.com/stretchr/testify/require"
)

const gcsIntegrationBucket = "tst-gcs-bucket"

// newTestGCSServer は fake-gcs-server を起動し、GCS クライアントのダウンロード経路が
// 参照する Host ヘッダに合わせて publicHost を実アドレスへ設定する。
func newTestGCSServer(t *testing.T) *fakestorage.Server {
	t.Helper()
	srv, err := fakestorage.NewServerWithOptions(fakestorage.Options{Scheme: "http"})
	require.NoError(t, err)
	t.Cleanup(srv.Stop)

	hostport := strings.TrimPrefix(srv.URL(), "http://")
	req, err := http.NewRequest(http.MethodPut, srv.URL()+"/_internal/config",
		strings.NewReader(`{"publicHost":"`+hostport+`"}`))
	require.NoError(t, err)
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.NoError(t, resp.Body.Close())

	srv.CreateBucketWithOpts(fakestorage.CreateBucketOpts{Name: gcsIntegrationBucket})
	return srv
}

func newTestGCSWriter(t *testing.T, srv *fakestorage.Server) *GCSWriter {
	t.Helper()
	t.Setenv("STORAGE_EMULATOR_HOST", srv.URL())
	client, err := storage.NewClient(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	return &GCSWriter{client: client, bucket: gcsIntegrationBucket}
}

func readJSONLObject(t *testing.T, w *GCSWriter, gsPath string) []map[string]interface{} {
	t.Helper()
	objectPath := strings.TrimPrefix(gsPath, fmt.Sprintf("gs://%s/", gcsIntegrationBucket))
	reader, err := w.client.Bucket(gcsIntegrationBucket).Object(objectPath).NewReader(context.Background())
	require.NoError(t, err)
	defer reader.Close()

	var rows []map[string]interface{}
	scanner := bufio.NewScanner(reader)
	for scanner.Scan() {
		var row map[string]interface{}
		require.NoError(t, json.Unmarshal(scanner.Bytes(), &row))
		rows = append(rows, row)
	}
	require.NoError(t, scanner.Err())
	return rows
}

func TestGCSWriterWrite(t *testing.T) {
	ctx := context.Background()

	t.Run("[GCSへの行データ書き込み]", func(t *testing.T) {
		t.Run("行データを渡して書き込みを行うと、戻り値のgs://パスのオブジェクトから、渡した行を渡した順にJSONLとしてデコードした内容が読み出せる", func(t *testing.T) {
			srv := newTestGCSServer(t)
			w := newTestGCSWriter(t, srv)
			rows := []map[string]interface{}{
				{"id": "k1", "value": "one"},
				{"id": "k2", "value": "two"},
			}

			gsPath, err := w.Write(ctx, "games", rows, time.Date(2026, 3, 14, 9, 5, 30, 0, time.UTC))

			require.NoError(t, err)
			got := readJSONLObject(t, w, gsPath)
			require.Len(t, got, 2)
			require.Equal(t, "k1", got[0]["id"])
			require.Equal(t, "one", got[0]["value"])
			require.Equal(t, "k2", got[1]["id"])
			require.Equal(t, "two", got[1]["value"])
		})

		t.Run("バケット名・テーブル名・タイムスタンプを指定して書き込みを行うと、戻り値がgs://<バケット名>/exports/<日付>/<テーブル名>/<時刻>.jsonl形式になる", func(t *testing.T) {
			srv := newTestGCSServer(t)
			w := newTestGCSWriter(t, srv)

			gsPath, err := w.Write(ctx, "games", []map[string]interface{}{{"id": "k1"}}, time.Date(2026, 3, 14, 9, 5, 30, 123000000, time.UTC))

			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("gs://%s/exports/20260314/games/090530.123.jsonl", gcsIntegrationBucket), gsPath)
		})

		t.Run("行データが空のとき、書き込み結果は0行のオブジェクトになる", func(t *testing.T) {
			srv := newTestGCSServer(t)
			w := newTestGCSWriter(t, srv)

			gsPath, err := w.Write(ctx, "games", []map[string]interface{}{}, time.Date(2026, 3, 14, 9, 5, 30, 0, time.UTC))

			require.NoError(t, err)
			require.Empty(t, readJSONLObject(t, w, gsPath))
		})

		t.Run("JSONにエンコードできない値を含む行データを渡すと、エラーメッセージに行のエンコードに失敗した旨が示される", func(t *testing.T) {
			srv := newTestGCSServer(t)
			w := newTestGCSWriter(t, srv)
			rows := []map[string]interface{}{{"id": "k1", "bad": make(chan int)}}

			_, err := w.Write(ctx, "games", rows, time.Date(2026, 3, 14, 9, 5, 30, 0, time.UTC))

			require.Error(t, err)
			require.Contains(t, err.Error(), "encode row")
		})
	})
}

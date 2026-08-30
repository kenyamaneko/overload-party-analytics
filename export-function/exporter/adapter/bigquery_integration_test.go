//go:build integration

package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"cloud.google.com/go/bigquery"
	"cloud.google.com/go/storage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	tcbigquery "github.com/testcontainers/testcontainers-go/modules/gcloud/bigquery"
	"github.com/testcontainers/testcontainers-go/network"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"

	"export-to-bq/exporter/model"
)

const (
	bqIntegrationProjectID  = "tst-analytics"
	bqIntegrationDatasetID  = "tst_dataset"
	bqIntegrationBucket     = "tst-bucket"
	fakeGCSEmulatorImage    = "fsouza/fake-gcs-server:1.55.1"
	bigQueryEmulatorImage   = "ghcr.io/goccy/bigquery-emulator:0.8.1"
	fakeGCSNetworkAlias     = "fake-gcs"
	bigQueryEmulatorTimeout = 30 * time.Second
)

var bqIntegrationSchema = bigquery.Schema{
	{Name: "id", Type: bigquery.StringFieldType},
	{Name: "value", Type: bigquery.StringFieldType},
}

func TestBQLoaderMergeLoad(t *testing.T) {
	ctx := context.Background()

	net, err := network.New(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = net.Remove(ctx) })

	gcsContainer, err := testcontainers.Run(ctx, fakeGCSEmulatorImage,
		testcontainers.WithExposedPorts("4443/tcp"),
		testcontainers.WithCmdArgs("-scheme", "http", "-port", "4443", "-backend", "memory"),
		network.WithNetwork([]string{fakeGCSNetworkAlias}, net),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("4443/tcp")),
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, gcsContainer.Terminate(ctx)) })

	gcsHostEndpoint, err := gcsContainer.PortEndpoint(ctx, "4443/tcp", "http")
	require.NoError(t, err)

	bqContainer, err := tcbigquery.Run(ctx, bigQueryEmulatorImage,
		testcontainers.WithCmdArgs("--project", bqIntegrationProjectID, "--dataset", bqIntegrationDatasetID),
		testcontainers.WithEnv(map[string]string{
			// BigQuery エミュレータの GCS ロードジョブが同一 Docker ネットワーク上の fake-gcs-server を参照できるようにする
			"STORAGE_EMULATOR_HOST": "http://" + fakeGCSNetworkAlias + ":4443",
		}),
		network.WithNetwork([]string{"bq-emulator"}, net),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/discovery/v1/apis/bigquery/v2/rest").WithPort("9050/tcp").WithStartupTimeout(bigQueryEmulatorTimeout),
		),
	)
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, bqContainer.Terminate(ctx)) })

	bqHostEndpoint, err := bqContainer.PortEndpoint(ctx, "9050/tcp", "http")
	require.NoError(t, err)

	storageClient, err := storage.NewClient(ctx, option.WithEndpoint(gcsHostEndpoint+"/storage/v1/"), option.WithoutAuthentication())
	require.NoError(t, err)
	t.Cleanup(func() { _ = storageClient.Close() })
	require.NoError(t, storageClient.Bucket(bqIntegrationBucket).Create(ctx, bqIntegrationProjectID, nil))

	bqClient, err := bigquery.NewClient(ctx, bqIntegrationProjectID, option.WithEndpoint(bqHostEndpoint), option.WithoutAuthentication())
	require.NoError(t, err)
	t.Cleanup(func() { _ = bqClient.Close() })

	loader := &BQLoader{client: bqClient, datasetID: bqIntegrationDatasetID}

	createBQTable := func(t *testing.T, name string) {
		t.Helper()
		require.NoError(t, bqClient.Dataset(bqIntegrationDatasetID).Table(name).Create(ctx, &bigquery.TableMetadata{Schema: bqIntegrationSchema}))
	}

	insertBQRow := func(t *testing.T, table, id, value string) {
		t.Helper()
		q := bqClient.Query(fmt.Sprintf("INSERT INTO `%s.%s.%s` (id, value) VALUES (@id, @value)", bqIntegrationProjectID, bqIntegrationDatasetID, table))
		q.Parameters = []bigquery.QueryParameter{{Name: "id", Value: id}, {Name: "value", Value: value}}
		job, err := q.Run(ctx)
		require.NoError(t, err)
		status, err := job.Wait(ctx)
		require.NoError(t, err)
		require.NoError(t, status.Err())
	}

	uploadJSONL := func(t *testing.T, objectPath string, rows []map[string]interface{}) string {
		t.Helper()
		w := storageClient.Bucket(bqIntegrationBucket).Object(objectPath).NewWriter(ctx)
		enc := json.NewEncoder(w)
		for _, row := range rows {
			require.NoError(t, enc.Encode(row))
		}
		require.NoError(t, w.Close())
		return fmt.Sprintf("gs://%s/%s", bqIntegrationBucket, objectPath)
	}

	readBQRows := func(t *testing.T, table string) map[string]string {
		t.Helper()
		it, err := bqClient.Query(fmt.Sprintf("SELECT id, value FROM `%s.%s.%s`", bqIntegrationProjectID, bqIntegrationDatasetID, table)).Read(ctx)
		require.NoError(t, err)
		got := map[string]string{}
		for {
			var row []bigquery.Value
			rerr := it.Next(&row)
			if rerr == iterator.Done {
				break
			}
			require.NoError(t, rerr)
			got[row[0].(string)] = row[1].(string)
		}
		return got
	}

	countBQRowsByID := func(t *testing.T, table, id string) int64 {
		t.Helper()
		q := bqClient.Query(fmt.Sprintf("SELECT COUNT(*) FROM `%s.%s.%s` WHERE id = @id", bqIntegrationProjectID, bqIntegrationDatasetID, table))
		q.Parameters = []bigquery.QueryParameter{{Name: "id", Value: id}}
		it, err := q.Read(ctx)
		require.NoError(t, err)
		var row []bigquery.Value
		require.NoError(t, it.Next(&row))
		return row[0].(int64)
	}

	bqTableNames := func(t *testing.T) []string {
		t.Helper()
		it := bqClient.Dataset(bqIntegrationDatasetID).Tables(ctx)
		var names []string
		for {
			tbl, terr := it.Next()
			if terr == iterator.Done {
				break
			}
			require.NoError(t, terr)
			names = append(names, tbl.TableID)
		}
		return names
	}

	t.Run("[BigQuery取り込み] MERGE取り込み", func(t *testing.T) {
		t.Run("対象テーブルに一意キーが一致する既存行があるとき、取り込み後にその行はステージングテーブル側の値に更新される", func(t *testing.T) {
			table := "tst_case1"
			createBQTable(t, table)
			insertBQRow(t, table, "k1", "old")
			gcsPath := uploadJSONL(t, "case1/data.jsonl", []map[string]interface{}{{"id": "k1", "value": "new"}})

			_, err := loader.Load(ctx, model.TableConfig{BigQueryTable: table, NaturalKey: []string{"id"}}, gcsPath)
			require.NoError(t, err)

			require.Equal(t, "new", readBQRows(t, table)["k1"])
		})

		t.Run("ステージングテーブル側に対象テーブルに存在しない一意キーを持つ行があるとき、取り込み後にその行が対象テーブルに新規追加される", func(t *testing.T) {
			table := "tst_case2"
			createBQTable(t, table)
			gcsPath := uploadJSONL(t, "case2/data.jsonl", []map[string]interface{}{{"id": "k2", "value": "added"}})

			_, err := loader.Load(ctx, model.TableConfig{BigQueryTable: table, NaturalKey: []string{"id"}}, gcsPath)
			require.NoError(t, err)

			require.Equal(t, "added", readBQRows(t, table)["k2"])
		})

		t.Run("対象テーブルにのみ存在しステージングテーブル側に含まれない行は、取り込み後も対象テーブルに残り続ける", func(t *testing.T) {
			table := "tst_case3"
			createBQTable(t, table)
			insertBQRow(t, table, "k3", "kept")
			gcsPath := uploadJSONL(t, "case3/data.jsonl", []map[string]interface{}{{"id": "other", "value": "x"}})

			_, err := loader.Load(ctx, model.TableConfig{BigQueryTable: table, NaturalKey: []string{"id"}}, gcsPath)
			require.NoError(t, err)

			require.Equal(t, "kept", readBQRows(t, table)["k3"])
		})

		t.Run("同一のGCSパスに対して取り込みを2回連続で実行しても、対象テーブルの該当行は重複しない", func(t *testing.T) {
			table := "tst_case4"
			createBQTable(t, table)
			gcsPath := uploadJSONL(t, "case4/data.jsonl", []map[string]interface{}{{"id": "k4", "value": "v4"}})

			_, err := loader.Load(ctx, model.TableConfig{BigQueryTable: table, NaturalKey: []string{"id"}}, gcsPath)
			require.NoError(t, err)
			_, err = loader.Load(ctx, model.TableConfig{BigQueryTable: table, NaturalKey: []string{"id"}}, gcsPath)
			require.NoError(t, err)

			require.Equal(t, int64(1), countBQRowsByID(t, table, "k4"))
		})

		t.Run("取り込み実行前に前回実行のステージングテーブルが既に残っていても、取り込みはエラーにならず該当行が正しく取り込まれる", func(t *testing.T) {
			table := "tst_case5"
			createBQTable(t, table)
			createBQTable(t, table+stagingTableSuffix)
			gcsPath := uploadJSONL(t, "case5/data.jsonl", []map[string]interface{}{{"id": "k5", "value": "v5"}})

			_, err := loader.Load(ctx, model.TableConfig{BigQueryTable: table, NaturalKey: []string{"id"}}, gcsPath)
			require.NoError(t, err)

			require.Equal(t, "v5", readBQRows(t, table)["k5"])
		})

		t.Run("取り込みが成功したあと、ステージングテーブルは削除され残らない", func(t *testing.T) {
			table := "tst_case6"
			createBQTable(t, table)
			gcsPath := uploadJSONL(t, "case6/data.jsonl", []map[string]interface{}{{"id": "k6", "value": "v6"}})

			_, err := loader.Load(ctx, model.TableConfig{BigQueryTable: table, NaturalKey: []string{"id"}}, gcsPath)
			require.NoError(t, err)

			require.NotContains(t, bqTableNames(t), table+stagingTableSuffix)
		})

		t.Run("対象テーブルが存在しないとき、取り込みはエラーになり、エラーメッセージに対象テーブル名が示される。その後、対象テーブルを作成してから同じ取り込みをやり直すと該当行が正しく取り込まれる", func(t *testing.T) {
			table := "tst_case8"
			tableConfig := model.TableConfig{BigQueryTable: table, NaturalKey: []string{"id"}}
			gcsPath := uploadJSONL(t, "case8/data.jsonl", []map[string]interface{}{{"id": "k8", "value": "v8"}})

			_, err := loader.Load(ctx, tableConfig, gcsPath)
			require.Error(t, err)
			require.Contains(t, err.Error(), "fetch target metadata")
			require.Contains(t, err.Error(), table)

			createBQTable(t, table)
			_, err = loader.Load(ctx, tableConfig, gcsPath)
			require.NoError(t, err)
			require.Equal(t, "v8", readBQRows(t, table)["k8"])
		})
	})
}

package adapter

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strings"

	"cloud.google.com/go/bigquery"
	"google.golang.org/api/googleapi"

	"export-to-bq/exporter/model"
)

// bqSafeIdentifier は MERGE DDL への SQL インジェクション防止のため
// LoadConfig に加えて多重防御として再チェックします。
var bqSafeIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// stagingTableSuffix は MERGE 用ステージングテーブル名のサフィックスです。
const stagingTableSuffix = "_staging"

// BQLoader は GCS から BigQuery にデータをロードします。
type BQLoader struct {
	client    *bigquery.Client
	datasetID string
}

// NewBQLoader は新しい BQLoader を生成します。
func NewBQLoader(ctx context.Context, projectID, datasetID string) (*BQLoader, error) {
	if !bqSafeIdentifier.MatchString(datasetID) {
		return nil, fmt.Errorf("dataset id %q is not a valid identifier", datasetID)
	}
	client, err := bigquery.NewClient(ctx, projectID)
	if err != nil {
		return nil, fmt.Errorf("create BigQuery client: %w", err)
	}
	return &BQLoader{client: client, datasetID: datasetID}, nil
}

// Load は GCS パスからデータをステージング経由で BigQuery テーブルに MERGE し、取り込まれた行数を返します。
func (l *BQLoader) Load(ctx context.Context, tableConfig model.TableConfig, gcsPath string) (int64, error) {
	if !bqSafeIdentifier.MatchString(tableConfig.BigQueryTable) {
		return 0, fmt.Errorf("bigquery_table %q is not a valid identifier", tableConfig.BigQueryTable)
	}

	if len(tableConfig.NaturalKey) == 0 {
		return 0, fmt.Errorf("table %s has no natural_key to merge on", tableConfig.BigQueryTable)
	}

	for _, col := range tableConfig.NaturalKey {
		if !bqSafeIdentifier.MatchString(col) {
			return 0, fmt.Errorf("natural_key column %q is not a valid identifier", col)
		}
	}

	return l.mergeLoad(ctx, tableConfig, gcsPath)
}

// mergeLoad はステージングテーブル経由で MERGE ロードを行い、変更された行数を返します。
// ステージングテーブルは MERGE の成否にかかわらず常に削除されます。
func (l *BQLoader) mergeLoad(ctx context.Context, tableConfig model.TableConfig, gcsPath string) (int64, error) {
	stagingName := tableConfig.BigQueryTable + stagingTableSuffix
	dataset := l.client.Dataset(l.datasetID)
	target := dataset.Table(tableConfig.BigQueryTable)
	staging := dataset.Table(stagingName)

	targetMeta, err := target.Metadata(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch target metadata for %s: %w", tableConfig.BigQueryTable, err)
	}
	if targetMeta.Schema == nil {
		return 0, fmt.Errorf("target table %s has no schema", tableConfig.BigQueryTable)
	}

	// 前回クラッシュ時の残存テーブルを削除してからスキーマを複製
	if err := dropTableIfExists(ctx, staging); err != nil {
		return 0, fmt.Errorf("drop stale staging table %s: %w", stagingName, err)
	}
	if err := staging.Create(ctx, &bigquery.TableMetadata{Schema: targetMeta.Schema}); err != nil {
		return 0, fmt.Errorf("create staging table %s: %w", stagingName, err)
	}
	defer func() {
		if delErr := staging.Delete(ctx); delErr != nil {
			// MERGE は既にコミット（または報告）済みなのでログのみ。
			// 残存テーブルは次回実行時に上書きされる。
			slog.Warn("failed to drop staging table", "staging_table", stagingName, "error", delErr)
		}
	}()

	// GCS JSONL をステージングテーブルにロード
	stagingRef := bigquery.NewGCSReference(gcsPath)
	stagingRef.SourceFormat = bigquery.JSON
	stagingRef.AutoDetect = false
	stagingRef.MaxBadRecords = 0
	stagingRef.Schema = targetMeta.Schema

	stagingLoader := staging.LoaderFrom(stagingRef)
	stagingLoader.WriteDisposition = bigquery.WriteTruncate

	loadJob, err := stagingLoader.Run(ctx)
	if err != nil {
		return 0, fmt.Errorf("start staging load job: %w", err)
	}
	loadStatus, err := loadJob.Wait(ctx)
	if err != nil {
		return 0, fmt.Errorf("wait for staging load job: %w", err)
	}
	if err := loadStatus.Err(); err != nil {
		return 0, fmt.Errorf("staging load job failed: %w", err)
	}

	columns := make([]string, 0, len(targetMeta.Schema))
	for _, f := range targetMeta.Schema {
		if !bqSafeIdentifier.MatchString(f.Name) {
			return 0, fmt.Errorf("target column %q is not a valid identifier", f.Name)
		}
		columns = append(columns, f.Name)
	}

	mergeSQL := buildMergeSQL(l.datasetID, tableConfig.BigQueryTable, stagingName, tableConfig.NaturalKey, columns)

	query := l.client.Query(mergeSQL)
	mergeJob, err := query.Run(ctx)
	if err != nil {
		return 0, fmt.Errorf("start merge job: %w", err)
	}
	mergeStatus, err := mergeJob.Wait(ctx)
	if err != nil {
		return 0, fmt.Errorf("wait for merge job: %w", err)
	}
	if err := mergeStatus.Err(); err != nil {
		return 0, fmt.Errorf("merge job failed: %w", err)
	}

	rowCount, err := mergedRowCount(mergeStatus)
	if err != nil {
		return 0, fmt.Errorf("merge into %s: %w", tableConfig.BigQueryTable, err)
	}

	return rowCount, nil
}

// dropTableIfExists はテーブルを削除します。既に存在しない場合は成功として扱います。
func dropTableIfExists(ctx context.Context, table *bigquery.Table) error {
	err := table.Delete(ctx)
	if err == nil {
		return nil
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == http.StatusNotFound {
		return nil
	}
	return err
}

// mergedRowCount は完了した MERGE ジョブが変更した行数を返します。
func mergedRowCount(status *bigquery.JobStatus) (int64, error) {
	if status.Statistics == nil {
		return 0, errors.New("merge job reported no statistics")
	}
	stats, ok := status.Statistics.Details.(*bigquery.QueryStatistics)
	if !ok {
		return 0, fmt.Errorf("merge job reported %T instead of query statistics", status.Statistics.Details)
	}
	return stats.NumDMLAffectedRows, nil
}

// buildMergeSQL は MERGE ステートメントを構築します。
// 全識別子は bqSafeIdentifier で検証済みです。
func buildMergeSQL(datasetID, targetTable, stagingTable string, naturalKey, columns []string) string {
	onClauses := make([]string, len(naturalKey))
	for i, k := range naturalKey {
		onClauses[i] = fmt.Sprintf("target.%s = source.%s", k, k)
	}

	updateClauses := make([]string, 0, len(columns))
	insertCols := make([]string, 0, len(columns))
	insertVals := make([]string, 0, len(columns))
	for _, c := range columns {
		updateClauses = append(updateClauses, fmt.Sprintf("%s = source.%s", c, c))
		insertCols = append(insertCols, c)
		insertVals = append(insertVals, "source."+c)
	}

	return fmt.Sprintf(
		"MERGE INTO `%s.%s` AS target "+
			"USING `%s.%s` AS source "+
			"ON %s "+
			"WHEN MATCHED THEN UPDATE SET %s "+
			"WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s)",
		datasetID, targetTable,
		datasetID, stagingTable,
		strings.Join(onClauses, " AND "),
		strings.Join(updateClauses, ", "),
		strings.Join(insertCols, ", "),
		strings.Join(insertVals, ", "),
	)
}

// Close は BigQuery クライアントをクローズします。
func (l *BQLoader) Close() error {
	return l.client.Close()
}

package model

import "time"

// ExportResult はテーブル単位のエクスポート結果を表します。
type ExportResult struct {
	Table        string        `json:"table"`
	RowsExported int64         `json:"rows_exported"`
	FilePath     string        `json:"file_path,omitempty"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	IsSuccess    bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
	// IsCheckpointFailed は BQ ロード成功後に checkpoint 書き込みが失敗した場合に true。
	// handler が 206 ではなく 500 を返すために使用する。
	IsCheckpointFailed bool `json:"checkpoint_failed,omitempty"`
}

// Checkpoint はテーブルごとのエクスポート状態を表します。
type Checkpoint struct {
	Table          string
	LastExportTime time.Time
	LastRowCount   int64
	UpdatedAt      time.Time
}

// DedupMode はリラン時の重複行処理方式を制御します。
type DedupMode string

const (
	// DedupModeMerge は NaturalKey で MERGE します。リランでも冪等。
	DedupModeMerge DedupMode = "merge"
	// DedupModeAppend は無条件で行を追加します。空テーブルへの backfill 用。
	DedupModeAppend DedupMode = "append"
)

// Config はエクスポート設定を表します。
type Config struct {
	Tables map[string]TableConfig `yaml:"tables"`

	DBHost                 string
	DBUser                 string
	DBPassword             string
	DBName                 string
	InstanceConnectionName string
	BQProjectID            string
	BQDatasetID            string
	GCSBucket              string
	DedupMode              DedupMode
}

// TableConfig はテーブル単位���エクスポート設定を表します。
type TableConfig struct {
	SourceTable   string   `yaml:"source_table"`
	BigQueryTable string   `yaml:"bigquery_table"`
	Query         string   `yaml:"query"`
	// NaturalKey は BigQuery テーブルの行を一意に識別するカラム群。
	// DedupMode == merge 時にこのキーで MERGE される。
	NaturalKey []string `yaml:"natural_key"`
}

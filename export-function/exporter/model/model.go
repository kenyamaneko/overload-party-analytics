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
	// IsCheckpointFailed はウェアハウスへのロード成功後に checkpoint 書き込みが失敗した場合に true。
	IsCheckpointFailed bool `json:"checkpoint_failed,omitempty"`
}

// Checkpoint はテーブルごとのエクスポート状態を表します。
type Checkpoint struct {
	Table          string
	LastExportTime time.Time
	LastRowCount   int64
	UpdatedAt      time.Time
}

// Config はエクスポート設定を表します。
type Config struct {
	Tables map[string]TableConfig `yaml:"tables"`

	// DatabaseConn は PostgreSQL 接続文字列 (libpq キーワード形式)。
	DatabaseConn string

	// DatabaseIAMAuthEnabled は Cloud SQL への接続方式を切り替えます。
	DatabaseIAMAuthEnabled bool

	// CloudSQLConnectionName は Cloud SQL インスタンスの接続名 (project:region:instance)。
	CloudSQLConnectionName string

	BQProjectID string
	BQDatasetID string
	GCSBucket   string
}

// TableConfig はテーブル単位のエクスポート設定を表します。
type TableConfig struct {
	// SourceTable は取得元の PostgreSQL テーブルをスキーマ修飾名で表します。
	SourceTable   string `yaml:"source_table"`
	BigQueryTable string `yaml:"bigquery_table"`
	Query         string `yaml:"query"`
	// NaturalKey は BigQuery テーブルの行を一意に識別するカラム群です。
	NaturalKey []string `yaml:"natural_key"`
}

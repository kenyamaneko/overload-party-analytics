package model

import "time"

// ExportResult represents the result of a single table export.
type ExportResult struct {
	Table        string        `json:"table"`
	RowsExported int64         `json:"rows_exported"`
	FilePath     string        `json:"file_path,omitempty"`
	StartTime    time.Time     `json:"start_time"`
	EndTime      time.Time     `json:"end_time"`
	Duration     time.Duration `json:"duration"`
	Success      bool          `json:"success"`
	Error        string        `json:"error,omitempty"`
}

// Checkpoint represents the export state for a table.
type Checkpoint struct {
	Table          string
	LastExportTime time.Time
	LastRowCount   int64
	UpdatedAt      time.Time
}

// Config represents the export configuration.
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
}

// TableConfig represents the configuration for a single table.
type TableConfig struct {
	SourceTable   string `yaml:"source_table"`
	BigQueryTable string `yaml:"bigquery_table"`
	Query         string `yaml:"query"`
}

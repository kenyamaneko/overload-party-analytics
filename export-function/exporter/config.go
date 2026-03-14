package exporter

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the export configuration
type Config struct {
	Tables map[string]TableConfig `yaml:"tables"`

	// Runtime configuration (from environment variables)
	DBHost                 string
	DBUser                 string
	DBPassword             string
	DBName                 string
	InstanceConnectionName string
	BQProjectID            string
	BQDatasetID            string
	GCSBucket              string
}

// TableConfig represents the configuration for a single table
type TableConfig struct {
	SourceTable     string `yaml:"source_table"`
	BigQueryTable   string `yaml:"bigquery_table"`
	TimestampColumn string `yaml:"timestamp_column"`
	Query           string `yaml:"query"`
}

// LoadConfig loads the configuration from a YAML file
func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	var config Config
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse config file: %w", err)
	}

	if len(config.Tables) == 0 {
		return nil, fmt.Errorf("no tables defined in config")
	}

	for name, table := range config.Tables {
		if table.SourceTable == "" {
			return nil, fmt.Errorf("table %s: source_table is required", name)
		}
		if table.BigQueryTable == "" {
			return nil, fmt.Errorf("table %s: bigquery_table is required", name)
		}
		if table.Query == "" {
			return nil, fmt.Errorf("table %s: query is required", name)
		}
	}

	return &config, nil
}

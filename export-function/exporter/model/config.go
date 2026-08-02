package model

import (
	"fmt"
	"os"
	"regexp"

	"gopkg.in/yaml.v3"
)

// safeIdentifier は MERGE DDL への SQL インジェクションを防ぐため、
// BigQuery 識別子として安全な文字列パターンを定義します。
var safeIdentifier = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// LoadConfig は YAML ファイルからエクスポート設定を読み込みます。
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
		if !safeIdentifier.MatchString(table.BigQueryTable) {
			return nil, fmt.Errorf("table %s: bigquery_table %q is not a valid identifier", name, table.BigQueryTable)
		}
		if table.Query == "" {
			return nil, fmt.Errorf("table %s: query is required", name)
		}
		// 再実行を冪等にする MERGE のキーになるため、natural_key を必須にする
		if len(table.NaturalKey) == 0 {
			return nil, fmt.Errorf("table %s: natural_key is required", name)
		}
		for _, col := range table.NaturalKey {
			if !safeIdentifier.MatchString(col) {
				return nil, fmt.Errorf("table %s: natural_key column %q is not a valid identifier", name, col)
			}
		}
	}

	return &config, nil
}

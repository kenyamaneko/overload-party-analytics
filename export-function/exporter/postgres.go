package exporter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// queryPostgres executes an incremental query against PostgreSQL
func (e *Exporter) queryPostgres(ctx context.Context, tableConfig TableConfig, startTime, endTime time.Time) ([]map[string]interface{}, error) {
	rows, err := e.pgPool.Query(ctx, tableConfig.Query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return pgRowsToMaps(rows)
}

// pgRowsToMaps converts pgx rows to a slice of maps
func pgRowsToMaps(rows pgx.Rows) ([]map[string]interface{}, error) {
	fieldDescs := rows.FieldDescriptions()
	var result []map[string]interface{}

	for rows.Next() {
		values, err := rows.Values()
		if err != nil {
			return nil, fmt.Errorf("read row values: %w", err)
		}

		rowMap := make(map[string]interface{}, len(fieldDescs))
		for i, fd := range fieldDescs {
			val := values[i]
			switch v := val.(type) {
			case time.Time:
				val = v.Format(time.RFC3339Nano)
			case []byte:
				// JSONB columns: preserve as raw JSON so json.Encode writes
				// an unescaped JSON object instead of a quoted string.
				val = json.RawMessage(v)
			case map[string]interface{}:
				// pgx may also decode JSONB into a map
				val = v
			}
			rowMap[fd.Name] = val
		}
		result = append(result, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return result, nil
}

// newPgPool creates a new pgx connection pool
func newPgPool(ctx context.Context, config *Config) (*pgxpool.Pool, error) {
	var dsn string

	if config.InstanceConnectionName != "" {
		// Cloud SQL via Unix socket (Cloud Functions / Cloud Run)
		dsn = fmt.Sprintf(
			"host=/cloudsql/%s user=%s password=%s dbname=%s sslmode=disable",
			config.InstanceConnectionName, config.DBUser, config.DBPassword, config.DBName,
		)
	} else {
		// Direct TCP connection (local dev with Cloud SQL Auth Proxy)
		host := config.DBHost
		if host == "" {
			host = "localhost"
		}
		dsn = fmt.Sprintf(
			"host=%s user=%s password=%s dbname=%s sslmode=disable",
			host, config.DBUser, config.DBPassword, config.DBName,
		)
	}

	poolConfig, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, fmt.Errorf("parse dsn: %w", err)
	}

	poolConfig.MaxConns = 5

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}

	return pool, nil
}

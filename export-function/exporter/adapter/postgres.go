package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"export-to-bq/exporter/model"
)

// PostgresReader reads data from PostgreSQL.
type PostgresReader struct {
	pool *pgxpool.Pool
}

// NewPostgresReader creates a new PostgresReader.
func NewPostgresReader(ctx context.Context, config *model.Config) (*PostgresReader, error) {
	pool, err := newPgPool(ctx, config)
	if err != nil {
		return nil, err
	}
	return &PostgresReader{pool: pool}, nil
}

// Query executes the table's query with the given time range.
func (r *PostgresReader) Query(ctx context.Context, tableConfig model.TableConfig, startTime, endTime time.Time) ([]map[string]interface{}, error) {
	rows, err := r.pool.Query(ctx, tableConfig.Query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return pgRowsToMaps(rows)
}

// Close closes the connection pool.
func (r *PostgresReader) Close() {
	r.pool.Close()
}

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

func newPgPool(ctx context.Context, config *model.Config) (*pgxpool.Pool, error) {
	var dsn string

	if config.InstanceConnectionName != "" {
		dsn = fmt.Sprintf(
			"host=/cloudsql/%s user=%s password=%s dbname=%s sslmode=disable",
			config.InstanceConnectionName, config.DBUser, config.DBPassword, config.DBName,
		)
	} else {
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

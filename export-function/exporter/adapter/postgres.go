package adapter

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"time"

	"cloud.google.com/go/cloudsqlconn"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"export-to-bq/exporter/model"
)

// maxPoolConns はエクスポートが同時に開く PostgreSQL 接続の上限です。
const maxPoolConns = 5

// PostgresReader は PostgreSQL からデータを読み取ります。
type PostgresReader struct {
	pool        *pgxpool.Pool
	closeDialer func()
}

// NewPostgresReader は新しい PostgresReader を生成します。
func NewPostgresReader(ctx context.Context, config *model.Config) (*PostgresReader, error) {
	pool, closeDialer, err := newPgPool(ctx, config)
	if err != nil {
		return nil, err
	}
	return &PostgresReader{pool: pool, closeDialer: closeDialer}, nil
}

// Query は指定時間範囲でテーブルのクエリを実行します。
func (r *PostgresReader) Query(ctx context.Context, tableConfig model.TableConfig, startTime, endTime time.Time) ([]map[string]interface{}, error) {
	rows, err := r.pool.Query(ctx, tableConfig.Query, startTime, endTime)
	if err != nil {
		return nil, fmt.Errorf("query failed: %w", err)
	}
	defer rows.Close()

	return pgRowsToMaps(rows)
}

// Close はコネクションプールをクローズします。
func (r *PostgresReader) Close() {
	r.pool.Close()
	r.closeDialer()
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
			converted, err := convertPgValue(values[i])
			if err != nil {
				return nil, fmt.Errorf("column %s: %w", fd.Name, err)
			}
			rowMap[fd.Name] = converted
		}
		result = append(result, rowMap)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	return result, nil
}

// convertPgValue は Postgres の値を JSONL 出力に適した表現へ変換します。
// 変換規則を持たない型は BigQuery 側の値が崩れるため、素通りさせずエラーにします。
func convertPgValue(val interface{}) (interface{}, error) {
	switch v := val.(type) {
	case nil:
		return nil, nil
	case string, bool, int16, int32, int64, float32, float64:
		return v, nil
	case time.Time:
		return v.Format(time.RFC3339Nano), nil
	case [16]byte:
		// BigQuery 側の列が STRING のため、pgx が返す uuid の生バイト列を文字列表現にする
		return uuid.UUID(v).String(), nil
	case []byte:
		// JSONB カラム: json.Encode がエスケープなしの JSON オブジェクトを書き出すよう保持
		return json.RawMessage(v), nil
	case map[string]interface{}:
		return v, nil
	default:
		return nil, fmt.Errorf("unsupported Postgres value type %T", v)
	}
}

// newPgPool は設定に応じた接続方式でコネクションプールを構築し、後始末の関数と併せて返します。
func newPgPool(ctx context.Context, config *model.Config) (*pgxpool.Pool, func(), error) {
	poolConfig, err := pgxpool.ParseConfig(config.DatabaseConn)
	if err != nil {
		return nil, nil, fmt.Errorf("parse database conn: %w", err)
	}
	poolConfig.MaxConns = maxPoolConns

	cleanup := func() {}
	if config.DatabaseIAMAuthEnabled {
		dialer, err := cloudsqlconn.NewDialer(ctx,
			cloudsqlconn.WithIAMAuthN(),
			cloudsqlconn.WithDefaultDialOptions(cloudsqlconn.WithPrivateIP()),
		)
		if err != nil {
			return nil, nil, fmt.Errorf("cloudsqlconn new dialer: %w", err)
		}
		connectionName := config.CloudSQLConnectionName
		poolConfig.ConnConfig.DialFunc = func(ctx context.Context, _, _ string) (net.Conn, error) {
			return dialer.Dial(ctx, connectionName)
		}
		cleanup = func() { closeDialer(dialer) }
	}

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		cleanup()
		return nil, nil, fmt.Errorf("create pool: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		cleanup()
		return nil, nil, fmt.Errorf("ping: %w", err)
	}

	return pool, cleanup, nil
}

func closeDialer(dialer *cloudsqlconn.Dialer) {
	// 呼び出し元が終了処理の中で呼ぶため、失敗をログにとどめて処理を続行する。
	if err := dialer.Close(); err != nil {
		slog.Error("cloudsqlconn dialer close failed", "error", err)
	}
}

package exportfunction

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// setValidExporterEnv は初期化が設定検証を通過する環境変数一式を設定します。
func setValidExporterEnv(t *testing.T) {
	t.Helper()
	t.Setenv("INSTANCE_CONNECTION_NAME", "")
	t.Setenv("DB_HOST", "tst-db-host")
	t.Setenv("DB_USER", "tst-user")
	t.Setenv("DB_PASSWORD", "tst-password")
	t.Setenv("DB_NAME", "tst-db")
	t.Setenv("BQ_PROJECT_ID", "tst-project")
	t.Setenv("BQ_DATASET_ID", "tst-dataset")
	t.Setenv("GCS_BUCKET", "tst-bucket")
	t.Setenv("DEDUP_MODE", "merge")
}

func TestNewExporterService(t *testing.T) {
	t.Run("エクスポートサービスの初期化", func(t *testing.T) {
		t.Run("DEDUP_MODEが未対応値のとき、初期化はエラーになり該当値が示される", func(t *testing.T) {
			setValidExporterEnv(t)
			t.Setenv("DEDUP_MODE", "both")

			_, err := newExporterService(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid DEDUP_MODE")
			require.Contains(t, err.Error(), "both")
		})

		missingRequiredCases := []string{
			"DB_USER",
			"DB_PASSWORD",
			"DB_NAME",
			"BQ_PROJECT_ID",
			"BQ_DATASET_ID",
			"GCS_BUCKET",
		}
		for _, envName := range missingRequiredCases {
			t.Run(envName+"が未設定のとき、初期化はエラーになり不足変数群が示される", func(t *testing.T) {
				setValidExporterEnv(t)
				t.Setenv(envName, "")

				_, err := newExporterService(context.Background())
				require.Error(t, err)
				require.Contains(t, err.Error(), "missing required environment variables")
				require.Contains(t, err.Error(), envName)
			})
		}

		t.Run("INSTANCE_CONNECTION_NAMEとDB_HOSTがどちらも未設定のとき、初期化はエラーになり接続先の指定を促す", func(t *testing.T) {
			setValidExporterEnv(t)
			t.Setenv("DB_HOST", "")

			_, err := newExporterService(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), "missing database endpoint")
			require.Contains(t, err.Error(), "INSTANCE_CONNECTION_NAME")
			require.Contains(t, err.Error(), "DB_HOST")
		})
	})
}

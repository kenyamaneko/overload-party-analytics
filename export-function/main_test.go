package exportfunction

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// setValidExporterEnv は初期化が設定検証を通過する環境変数一式を設定します。
func setValidExporterEnv(t *testing.T) {
	t.Helper()
	t.Setenv("DATABASE_CONN", "host=tst-db-host dbname=tst-db user=tst-user sslmode=disable")
	t.Setenv("DATABASE_IAM_AUTH_ENABLED", "false")
	t.Setenv("CLOUDSQL_CONNECTION_NAME", "")
	t.Setenv("BQ_PROJECT_ID", "tst-project")
	t.Setenv("BQ_DATASET_ID", "tst-dataset")
	t.Setenv("GCS_BUCKET", "tst-bucket")
}

func TestNewExporterService(t *testing.T) {
	t.Run("エクスポートサービスの初期化", func(t *testing.T) {
		missingRequiredCases := []string{
			"DATABASE_CONN",
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

		invalidIAMAuthCases := []struct {
			name  string
			value string
		}{
			{name: "未設定のとき", value: ""},
			{name: `"true"/"false" 以外の "yes" のとき`, value: "yes"},
		}
		for _, tt := range invalidIAMAuthCases {
			t.Run("DATABASE_IAM_AUTH_ENABLEDが"+tt.name+"、初期化はエラーになり許容値が示される", func(t *testing.T) {
				setValidExporterEnv(t)
				t.Setenv("DATABASE_IAM_AUTH_ENABLED", tt.value)

				_, err := newExporterService(context.Background())
				require.Error(t, err)
				require.Contains(t, err.Error(), `DATABASE_IAM_AUTH_ENABLED must be "true" or "false"`)
			})
		}

		t.Run("DATABASE_IAM_AUTH_ENABLEDがtrueかつCLOUDSQL_CONNECTION_NAMEが未設定のとき、初期化はエラーになり接続名を要求する", func(t *testing.T) {
			setValidExporterEnv(t)
			t.Setenv("DATABASE_IAM_AUTH_ENABLED", "true")

			_, err := newExporterService(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), "CLOUDSQL_CONNECTION_NAME is required")
		})
	})
}

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
		missingRequiredCases := []struct {
			envName string
			wantMsg string
		}{
			{envName: "DATABASE_CONN", wantMsg: "missing required environment variable (DATABASE_CONN)"},
			{envName: "BQ_PROJECT_ID", wantMsg: "missing required environment variable (BQ_PROJECT_ID)"},
			{envName: "BQ_DATASET_ID", wantMsg: "missing required environment variable (BQ_DATASET_ID)"},
			{envName: "GCS_BUCKET", wantMsg: "missing required environment variable (GCS_BUCKET)"},
		}
		for _, tt := range missingRequiredCases {
			t.Run(tt.envName+`が未設定のとき、エラーメッセージは"`+tt.wantMsg+`"になる`, func(t *testing.T) {
				setValidExporterEnv(t)
				t.Setenv(tt.envName, "")

				_, err := newExporterService(context.Background())
				require.EqualError(t, err, tt.wantMsg)
			})
		}

		t.Run(`DATABASE_CONNとGCS_BUCKETが同時に未設定のとき、エラーメッセージは"missing required environment variable (DATABASE_CONN)"になる`, func(t *testing.T) {
			setValidExporterEnv(t)
			t.Setenv("DATABASE_CONN", "")
			t.Setenv("GCS_BUCKET", "")

			_, err := newExporterService(context.Background())
			require.EqualError(t, err, "missing required environment variable (DATABASE_CONN)")
		})

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

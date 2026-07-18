package exportfunction

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func setAllExporterEnv(t *testing.T, dbUser, dedupMode string) {
	t.Helper()
	t.Setenv("INSTANCE_CONNECTION_NAME", "")
	t.Setenv("DB_HOST", "")
	t.Setenv("DB_USER", dbUser)
	t.Setenv("DB_PASSWORD", "")
	t.Setenv("DB_NAME", "")
	t.Setenv("BQ_PROJECT_ID", "tst-project")
	t.Setenv("BQ_DATASET_ID", "tst-dataset")
	t.Setenv("GCS_BUCKET", "tst-bucket")
	t.Setenv("DEDUP_MODE", dedupMode)
}

func TestNewExporterService(t *testing.T) {
	t.Run("エクスポートサービスの初期化", func(t *testing.T) {
		t.Run("DEDUP_MODEが未対応値のとき、初期化はエラーになり該当値が示される", func(t *testing.T) {
			setAllExporterEnv(t, "tst-user", "both")

			_, err := newExporterService(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), "invalid DEDUP_MODE")
			require.Contains(t, err.Error(), "both")
		})

		t.Run("必須環境変数が未設定のとき、初期化はエラーになり不足変数群が示される", func(t *testing.T) {
			setAllExporterEnv(t, "", "merge")

			_, err := newExporterService(context.Background())
			require.Error(t, err)
			require.Contains(t, err.Error(), "missing required environment variables")
		})
	})
}

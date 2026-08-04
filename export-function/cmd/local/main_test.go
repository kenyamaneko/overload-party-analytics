package main

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRun(t *testing.T) {
	t.Run("ローカル起動", func(t *testing.T) {
		missingRequiredCases := []string{
			"PORT",
			"FUNCTION_TARGET",
		}
		for _, envName := range missingRequiredCases {
			t.Run(envName+"が未設定のとき、起動はエラーになり不足した環境変数名が示される", func(t *testing.T) {
				t.Setenv("PORT", "8080")
				t.Setenv("FUNCTION_TARGET", "ExportPostgresToBigQuery")
				t.Setenv(envName, "")

				err := run()
				require.Error(t, err)
				require.Contains(t, err.Error(), "missing required environment variable")
				require.Contains(t, err.Error(), envName)
			})
		}
	})
}

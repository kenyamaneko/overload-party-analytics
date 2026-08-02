// Package main はエクスポート関数をローカルで HTTP サーバとして起動します。
package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/GoogleCloudPlatform/functions-framework-go/funcframework"

	// エクスポート関数を登録する init() を走らせるため、空インポートする
	_ "export-to-bq"
)

func main() {
	if err := run(); err != nil {
		slog.Error("failed to start local server", "error", err)
		os.Exit(1)
	}
}

// run はエクスポート関数の HTTP サーバを起動します。
func run() error {
	port := os.Getenv("PORT")
	if port == "" {
		return fmt.Errorf("missing required environment variable (PORT)")
	}
	return funcframework.Start(port)
}

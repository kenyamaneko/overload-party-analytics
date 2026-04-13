package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"export-to-bq/exporter/model"
	"export-to-bq/exporter/service"
)

// ExportRequest はエクスポート関数のリクエストペイロードです。
type ExportRequest struct {
	Tables    []string `json:"tables"`
	Mode      string   `json:"mode"`       // "incremental" or "full"
	StartDate string   `json:"start_date"` // "YYYY-MM-DD" (full mode only)
	EndDate   string   `json:"end_date"`   // "YYYY-MM-DD" (full mode only)
}

// ExportResponse はエクスポート関数のレスポンスペイロードです。
type ExportResponse struct {
	Results []model.ExportResult `json:"results"`
	Success bool                 `json:"success"`
	Message string               `json:"message"`
}

// parseExportRequest はリクエストボディをデコード・バリデーションします。
func parseExportRequest(r *http.Request) (*ExportRequest, error) {
	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return nil, fmt.Errorf("invalid request: %w", err)
	}
	if len(req.Tables) == 0 {
		return nil, fmt.Errorf("no tables specified")
	}
	if req.Mode == "" {
		req.Mode = "incremental"
	}
	if req.Mode != "incremental" && req.Mode != "full" {
		return nil, fmt.Errorf("invalid mode %q: must be \"incremental\" or \"full\"", req.Mode)
	}
	return &req, nil
}

// New はファクトリから取得した service.Service に委譲する HTTP ハンドラを生成します。
func New(newExporter func(ctx context.Context) (service.Service, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := context.Background()

		req, err := parseExportRequest(r)
		if err != nil {
			log.Printf("ERROR: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		log.Printf("INFO: Starting export - tables=%v, mode=%s, start_date=%s, end_date=%s", req.Tables, req.Mode, req.StartDate, req.EndDate)

		exp, err := newExporter(ctx)
		if err != nil {
			log.Printf("ERROR: failed to initialize exporter: %v", err)
			http.Error(w, "initialization failed", http.StatusInternalServerError)
			return
		}
		defer exp.Close()

		results := exp.Export(ctx, req.Tables, req.Mode, req.StartDate, req.EndDate)

		success := true
		checkpointFailed := false
		for _, result := range results {
			if !result.Success {
				success = false
				if result.CheckpointFailed {
					checkpointFailed = true
				}
				log.Printf("ERROR: export failed for table %s: %s", result.Table, result.Error)
			} else {
				log.Printf("SUCCESS: exported %d rows from %s in %v", result.RowsExported, result.Table, result.Duration)
			}
		}

		response := ExportResponse{
			Results: results,
			Success: success,
		}

		// checkpoint 書き込み失敗 → 500（BQ ロード済みのため operator に通知必須）
		// その他の部分失敗 → 206
		status := http.StatusOK
		switch {
		case checkpointFailed:
			status = http.StatusInternalServerError
			response.Message = "Checkpoint write failed after warehouse load; re-run requires MERGE dedup to stay idempotent"
		case !success:
			status = http.StatusPartialContent
			response.Message = "Some exports failed, check results for details"
		default:
			response.Message = fmt.Sprintf("Successfully exported %d tables", len(results))
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if err := json.NewEncoder(w).Encode(response); err != nil {
			log.Printf("ERROR: failed to write response: %v", err)
		}
	}
}

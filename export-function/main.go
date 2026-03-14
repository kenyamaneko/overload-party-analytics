package exportfunction

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/GoogleCloudPlatform/functions-framework-go/functions"
	"export-to-bq/exporter"
)

func init() {
	functions.HTTP("ExportPostgresToBigQuery", exportHandler)
}

// ExportRequest defines the request payload for the export function
type ExportRequest struct {
	Tables    []string `json:"tables"`
	Mode      string   `json:"mode"`       // "incremental" or "full"
	StartDate string   `json:"start_date"` // "YYYY-MM-DD" (full mode only)
	EndDate   string   `json:"end_date"`   // "YYYY-MM-DD" (full mode only)
}

// ExportResponse defines the response payload
type ExportResponse struct {
	Results []exporter.ExportResult `json:"results"`
	Success bool                    `json:"success"`
	Message string                  `json:"message"`
}

// parseExportRequest decodes and validates the export request body.
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
	return &req, nil
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	req, err := parseExportRequest(r)
	if err != nil {
		log.Printf("ERROR: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	log.Printf("INFO: Starting export - tables=%v, mode=%s, start_date=%s, end_date=%s", req.Tables, req.Mode, req.StartDate, req.EndDate)

	// Initialize exporter
	exp, err := exporter.New(ctx)
	if err != nil {
		log.Printf("ERROR: failed to initialize exporter: %v", err)
		http.Error(w, "initialization failed", http.StatusInternalServerError)
		return
	}
	defer exp.Close()

	// Run export
	results := exp.Export(ctx, req.Tables, req.Mode, req.StartDate, req.EndDate)

	// Check if any export failed
	success := true
	for _, result := range results {
		if !result.Success {
			success = false
			log.Printf("ERROR: export failed for table %s: %s", result.Table, result.Error)
		} else {
			log.Printf("SUCCESS: exported %d rows from %s in %v", result.RowsExported, result.Table, result.Duration)
		}
	}

	// Build response
	response := ExportResponse{
		Results: results,
		Success: success,
	}

	if success {
		response.Message = fmt.Sprintf("Successfully exported %d tables", len(results))
	} else {
		response.Message = "Some exports failed, check results for details"
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	if success {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusPartialContent)
	}
	json.NewEncoder(w).Encode(response)
}

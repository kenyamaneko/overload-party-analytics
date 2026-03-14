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
	Tables []string `json:"tables"`
	Mode   string   `json:"mode"` // "incremental" or "full"
}

// ExportResponse defines the response payload
type ExportResponse struct {
	Results []exporter.ExportResult `json:"results"`
	Success bool                    `json:"success"`
	Message string                  `json:"message"`
}

func exportHandler(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	// Parse request
	var req ExportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Printf("ERROR: invalid request body: %v", err)
		http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
		return
	}

	// Validate request
	if len(req.Tables) == 0 {
		log.Printf("ERROR: no tables specified")
		http.Error(w, "no tables specified", http.StatusBadRequest)
		return
	}

	if req.Mode == "" {
		req.Mode = "incremental"
	}

	log.Printf("INFO: Starting export - tables=%v, mode=%s", req.Tables, req.Mode)

	// Initialize exporter
	exp, err := exporter.New(ctx)
	if err != nil {
		log.Printf("ERROR: failed to initialize exporter: %v", err)
		http.Error(w, "initialization failed", http.StatusInternalServerError)
		return
	}
	defer exp.Close()

	// Run export
	results := exp.Export(ctx, req.Tables, req.Mode)

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

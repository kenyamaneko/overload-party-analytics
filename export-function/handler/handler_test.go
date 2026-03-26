package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"export-to-bq/exporter/model"
	"export-to-bq/exporter/service"
)

func TestParseExportRequest_Valid(t *testing.T) {
	body := `{"tables": ["games", "players"], "mode": "full", "start_date": "2024-01-01", "end_date": "2024-02-01"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	req, err := parseExportRequest(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(req.Tables) != 2 {
		t.Errorf("tables: got %d, want 2", len(req.Tables))
	}
	if req.Mode != "full" {
		t.Errorf("mode: got %q, want full", req.Mode)
	}
	if req.StartDate != "2024-01-01" {
		t.Errorf("start_date: got %q, want 2024-01-01", req.StartDate)
	}
	if req.EndDate != "2024-02-01" {
		t.Errorf("end_date: got %q, want 2024-02-01", req.EndDate)
	}
}

func TestParseExportRequest_DefaultMode(t *testing.T) {
	body := `{"tables": ["games"]}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	req, err := parseExportRequest(r)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Mode != "incremental" {
		t.Errorf("mode should default to 'incremental', got %q", req.Mode)
	}
}

func TestParseExportRequest_EmptyTables(t *testing.T) {
	body := `{"tables": []}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	_, err := parseExportRequest(r)
	if err == nil {
		t.Fatal("expected error for empty tables")
	}
}

func TestParseExportRequest_MissingTables(t *testing.T) {
	body := `{"mode": "full"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	_, err := parseExportRequest(r)
	if err == nil {
		t.Fatal("expected error for missing tables")
	}
}

func TestParseExportRequest_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{invalid`))

	_, err := parseExportRequest(r)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestParseExportRequest_EmptyBody(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(``))

	_, err := parseExportRequest(r)
	if err == nil {
		t.Fatal("expected error for empty body")
	}
}

func TestParseExportRequest_InvalidMode(t *testing.T) {
	body := `{"tables": ["games"], "mode": "xyz"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	_, err := parseExportRequest(r)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	if !strings.Contains(err.Error(), "xyz") {
		t.Errorf("error should mention the invalid mode, got: %s", err.Error())
	}
}

// mockExporter implements service.Service for testing.
type mockExporter struct {
	results []model.ExportResult
}

func (m *mockExporter) Export(_ context.Context, _ []string, _ string, _, _ string) []model.ExportResult {
	return m.results
}

func (m *mockExporter) Close() {}

func newMockFactory(results []model.ExportResult) func(context.Context) (service.Service, error) {
	return func(_ context.Context) (service.Service, error) {
		return &mockExporter{results: results}, nil
	}
}

func TestHandler_InvalidJSON(t *testing.T) {
	h := New(newMockFactory(nil))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not json`))
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_EmptyTables(t *testing.T) {
	h := New(newMockFactory(nil))
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"tables": []}`))
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandler_AllSuccess(t *testing.T) {
	results := []model.ExportResult{
		{
			Table:        "games",
			RowsExported: 42,
			FilePath:     "gs://bucket/exports/20250311/games/030000.000.jsonl",
			StartTime:    time.Now(),
			EndTime:      time.Now(),
			Duration:     100 * time.Millisecond,
			Success:      true,
		},
		{
			Table:        "players",
			RowsExported: 10,
			FilePath:     "gs://bucket/exports/20250311/players/030000.000.jsonl",
			StartTime:    time.Now(),
			EndTime:      time.Now(),
			Duration:     50 * time.Millisecond,
			Success:      true,
		},
	}

	h := New(newMockFactory(results))
	body := `{"tables": ["games", "players"], "mode": "incremental"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusOK)
	}

	var resp ExportResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !resp.Success {
		t.Error("expected success=true")
	}
	if len(resp.Results) != 2 {
		t.Errorf("results: got %d, want 2", len(resp.Results))
	}
	if resp.Results[0].RowsExported != 42 {
		t.Errorf("rows_exported: got %d, want 42", resp.Results[0].RowsExported)
	}
}

func TestHandler_PartialFailure(t *testing.T) {
	results := []model.ExportResult{
		{
			Table:        "games",
			RowsExported: 42,
			Success:      true,
		},
		{
			Table:   "players",
			Success: false,
			Error:   "query postgres: connection refused",
		},
	}

	h := New(newMockFactory(results))
	body := `{"tables": ["games", "players"], "mode": "incremental"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusPartialContent {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusPartialContent)
	}

	var resp ExportResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Success {
		t.Error("expected success=false for partial failure")
	}
	if resp.Results[1].Error == "" {
		t.Error("expected error message for failed table")
	}
}

func TestHandler_InitFailure(t *testing.T) {
	factory := func(_ context.Context) (service.Service, error) {
		return nil, fmt.Errorf("missing required environment variables")
	}

	h := New(factory)
	body := `{"tables": ["games"]}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	w := httptest.NewRecorder()

	h(w, r)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

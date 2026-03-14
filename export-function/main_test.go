package exportfunction

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

func TestExportHandler_InvalidJSON(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`not json`))
	w := httptest.NewRecorder()

	exportHandler(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestExportHandler_EmptyTables(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"tables": []}`))
	w := httptest.NewRecorder()

	exportHandler(w, r)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want %d", w.Code, http.StatusBadRequest)
	}
}

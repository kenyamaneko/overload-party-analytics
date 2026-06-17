package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"export-to-bq/exporter/model"
)

// テスト用モック

type stubSource struct {
	rows []map[string]interface{}
}

func (s *stubSource) Query(_ context.Context, _ model.TableConfig, _, _ time.Time) ([]map[string]interface{}, error) {
	return s.rows, nil
}
func (s *stubSource) Close() {}

type stubStaging struct {
	deleted []string
}

func (s *stubStaging) Write(_ context.Context, _ string, _ []map[string]interface{}, _ time.Time) (string, error) {
	return "gs://stub-bucket/exports/stub.jsonl", nil
}
func (s *stubStaging) Delete(_ context.Context, uri string) error {
	s.deleted = append(s.deleted, uri)
	return nil
}
func (s *stubStaging) Close() error { return nil }

type stubWarehouse struct {
	lastMode model.DedupMode
}

func (s *stubWarehouse) Load(_ context.Context, _ model.TableConfig, _ string, mode model.DedupMode) error {
	s.lastMode = mode
	return nil
}
func (s *stubWarehouse) Close() error { return nil }

type stubCheckpoint struct {
	updateErr error
	updated   bool
}

func (s *stubCheckpoint) Get(_ context.Context, table string) (*model.Checkpoint, error) {
	return &model.Checkpoint{Table: table}, nil
}
func (s *stubCheckpoint) Update(_ context.Context, _ string, _ *model.Checkpoint) error {
	s.updated = true
	return s.updateErr
}
func (s *stubCheckpoint) Close() error { return nil }

func newTestConfig() *model.Config {
	return &model.Config{
		Tables: map[string]model.TableConfig{
			"games": {
				SourceTable:   "games",
				BigQueryTable: "games",
				Query:         "SELECT 1 WHERE $1 < $2",
				NaturalKey:    []string{"game_id"},
			},
		},
		DedupMode: model.DedupModeMerge,
	}
}

// テスト

func TestExport_CheckpointFailureEscalates(t *testing.T) {
	cfg := newTestConfig()
	src := &stubSource{rows: []map[string]interface{}{{"game_id": "g1"}}}
	stg := &stubStaging{}
	wh := &stubWarehouse{}
	cp := &stubCheckpoint{updateErr: errors.New("firestore: UNAVAILABLE")}

	svc := New(cfg, src, stg, wh, cp)
	results := svc.Export(context.Background(), []string{"games"}, "incremental", "", "")

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.IsSuccess {
		t.Error("expected IsSuccess=false on checkpoint failure")
	}
	if !r.IsCheckpointFailed {
		t.Error("expected IsCheckpointFailed=true")
	}
	if r.Error == "" || !errorContains(r.Error, "checkpoint update failed") {
		t.Errorf("expected error to wrap ErrCheckpointUpdate, got: %q", r.Error)
	}
}

func TestExport_FullModeSkipsCheckpointUpdate(t *testing.T) {
	cfg := newTestConfig()
	src := &stubSource{rows: []map[string]interface{}{{"game_id": "g1"}}}
	stg := &stubStaging{}
	wh := &stubWarehouse{}
	cp := &stubCheckpoint{updateErr: errors.New("should not be called")}

	svc := New(cfg, src, stg, wh, cp)
	results := svc.Export(context.Background(), []string{"games"}, "full", "2024-01-01", "2024-02-01")

	if len(results) != 1 || !results[0].IsSuccess {
		t.Fatalf("expected success in full mode, got %+v", results)
	}
	if cp.updated {
		t.Error("checkpoint.Update should not be called in full (backfill) mode")
	}
}

func TestExport_StagingDeletedAfterSuccess(t *testing.T) {
	cfg := newTestConfig()
	src := &stubSource{rows: []map[string]interface{}{{"game_id": "g1"}}}
	stg := &stubStaging{}
	wh := &stubWarehouse{}
	cp := &stubCheckpoint{}

	svc := New(cfg, src, stg, wh, cp)
	_ = svc.Export(context.Background(), []string{"games"}, "incremental", "", "")

	if len(stg.deleted) != 1 {
		t.Fatalf("expected 1 staging delete, got %d", len(stg.deleted))
	}
}

func errorContains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestResolveTimeRange_Incremental(t *testing.T) {
	checkpoint := time.Date(2025, 3, 10, 3, 0, 0, 0, time.UTC)
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	start, end, err := resolveTimeRange("incremental", "", "", checkpoint, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !start.Equal(checkpoint) {
		t.Errorf("start: got %v, want %v", start, checkpoint)
	}
	if !end.Equal(now) {
		t.Errorf("end: got %v, want %v", end, now)
	}
}

func TestResolveTimeRange_IncrementalIgnoresDateParams(t *testing.T) {
	checkpoint := time.Date(2025, 3, 10, 3, 0, 0, 0, time.UTC)
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	start, end, err := resolveTimeRange("incremental", "2024-01-01", "2024-02-01", checkpoint, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !start.Equal(checkpoint) {
		t.Errorf("start_date should be ignored in incremental mode: got %v, want %v", start, checkpoint)
	}
	if !end.Equal(now) {
		t.Errorf("end_date should be ignored in incremental mode: got %v, want %v", end, now)
	}
}

func TestResolveTimeRange_FullWithoutDates(t *testing.T) {
	checkpoint := time.Date(2025, 3, 10, 3, 0, 0, 0, time.UTC)
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	start, end, err := resolveTimeRange("full", "", "", checkpoint, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !start.IsZero() {
		t.Errorf("full mode without start_date should start from epoch, got %v", start)
	}
	if !end.Equal(now) {
		t.Errorf("end: got %v, want %v", end, now)
	}
}

func TestResolveTimeRange_FullWithStartDate(t *testing.T) {
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	start, end, err := resolveTimeRange("full", "2024-06-01", "", time.Time{}, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	if !start.Equal(want) {
		t.Errorf("start: got %v, want %v", start, want)
	}
	if !end.Equal(now) {
		t.Errorf("end: got %v, want %v", end, now)
	}
}

func TestResolveTimeRange_FullWithEndDate(t *testing.T) {
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	start, end, err := resolveTimeRange("full", "", "2024-07-01", time.Time{}, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !start.IsZero() {
		t.Errorf("start should be epoch, got %v", start)
	}
	wantEnd := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	if !end.Equal(wantEnd) {
		t.Errorf("end: got %v, want %v", end, wantEnd)
	}
}

func TestResolveTimeRange_FullWithBothDates(t *testing.T) {
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	start, end, err := resolveTimeRange("full", "2024-06-01", "2024-07-01", time.Time{}, now)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantStart := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	wantEnd := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	if !start.Equal(wantStart) {
		t.Errorf("start: got %v, want %v", start, wantStart)
	}
	if !end.Equal(wantEnd) {
		t.Errorf("end: got %v, want %v", end, wantEnd)
	}
}

func TestResolveTimeRange_FullWithInvalidStartDate(t *testing.T) {
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	_, _, err := resolveTimeRange("full", "not-a-date", "", time.Time{}, now)

	if err == nil {
		t.Error("expected error for invalid start_date, got nil")
	}
}

func TestResolveTimeRange_FullWithInvalidEndDate(t *testing.T) {
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	_, _, err := resolveTimeRange("full", "", "bad", time.Time{}, now)

	if err == nil {
		t.Error("expected error for invalid end_date, got nil")
	}
}

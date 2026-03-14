package exporter

import (
	"testing"
	"time"
)

func TestResolveTimeRange_Incremental(t *testing.T) {
	checkpoint := time.Date(2025, 3, 10, 3, 0, 0, 0, time.UTC)
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	start, end := resolveTimeRange("incremental", "", "", checkpoint, now)

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

	start, end := resolveTimeRange("incremental", "2024-01-01", "2024-02-01", checkpoint, now)

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

	start, end := resolveTimeRange("full", "", "", checkpoint, now)

	if !start.IsZero() {
		t.Errorf("full mode without start_date should start from epoch, got %v", start)
	}
	if !end.Equal(now) {
		t.Errorf("end: got %v, want %v", end, now)
	}
}

func TestResolveTimeRange_FullWithStartDate(t *testing.T) {
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	start, end := resolveTimeRange("full", "2024-06-01", "", time.Time{}, now)

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

	start, end := resolveTimeRange("full", "", "2024-07-01", time.Time{}, now)

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

	start, end := resolveTimeRange("full", "2024-06-01", "2024-07-01", time.Time{}, now)

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

	start, _ := resolveTimeRange("full", "not-a-date", "", time.Time{}, now)

	if !start.IsZero() {
		t.Errorf("invalid start_date should fall back to epoch, got %v", start)
	}
}

func TestResolveTimeRange_FullWithInvalidEndDate(t *testing.T) {
	now := time.Date(2025, 3, 11, 3, 0, 0, 0, time.UTC)

	_, end := resolveTimeRange("full", "", "bad", time.Time{}, now)

	if !end.Equal(now) {
		t.Errorf("invalid end_date should fall back to now, got %v", end)
	}
}

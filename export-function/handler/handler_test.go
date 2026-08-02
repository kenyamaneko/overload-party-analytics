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

	"github.com/stretchr/testify/require"

	"export-to-bq/exporter/model"
	"export-to-bq/exporter/service"
)

func TestParseExportRequest(t *testing.T) {
	t.Run("エクスポートリクエストのパース", func(t *testing.T) {
		t.Run("全項目を含む JSON のとき、各フィールドがパースされる", func(t *testing.T) {
			body := `{"tables": ["games", "players"], "mode": "full", "start_date": "2024-01-01", "end_date": "2024-02-01"}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

			req, err := parseExportRequest(r)
			require.NoError(t, err)
			require.Len(t, req.Tables, 2)
			require.Equal(t, "full", req.Mode)
			require.Equal(t, "2024-01-01", req.StartDate)
			require.Equal(t, "2024-02-01", req.EndDate)
		})

		t.Run("mode 未指定のとき、incremental が既定になる", func(t *testing.T) {
			body := `{"tables": ["games"]}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

			req, err := parseExportRequest(r)
			require.NoError(t, err)
			require.Equal(t, "incremental", req.Mode)
		})

		invalidCases := []struct {
			name            string
			body            string
			wantErrContains string
		}{
			{name: "tables が空のとき、エラーになる", body: `{"tables": []}`, wantErrContains: "no tables specified"},
			{name: "tables が無いとき、エラーになる", body: `{"mode": "full"}`, wantErrContains: "no tables specified"},
			{name: "JSON として解析できないとき、エラーになる", body: `{invalid`, wantErrContains: "invalid request"},
			{name: "body が空のとき、エラーになる", body: ``, wantErrContains: "invalid request"},
		}
		for _, tt := range invalidCases {
			t.Run(tt.name, func(t *testing.T) {
				r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
				_, err := parseExportRequest(r)
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.wantErrContains)
			})
		}

		t.Run("mode が未定義値のとき、エラーメッセージに該当値が含まれる", func(t *testing.T) {
			body := `{"tables": ["games"], "mode": "xyz"}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

			_, err := parseExportRequest(r)
			require.Error(t, err)
			require.Contains(t, err.Error(), "xyz")
		})
	})
}

// mockExporter はテスト用の service.Service 実装です。
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

func TestHandler(t *testing.T) {
	t.Run("エクスポートハンドラ", func(t *testing.T) {
		badRequestCases := []struct {
			name string
			body string
		}{
			{name: "不正な JSON のとき、400 を返す", body: `not json`},
			{name: "tables が空のとき、400 を返す", body: `{"tables": []}`},
		}
		for _, tt := range badRequestCases {
			t.Run(tt.name, func(t *testing.T) {
				h := New(newMockFactory(nil))
				r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(tt.body))
				w := httptest.NewRecorder()

				h(w, r)

				require.Equal(t, http.StatusBadRequest, w.Code)
			})
		}

		t.Run("全テーブル成功のとき、200 と結果一覧を返す", func(t *testing.T) {
			results := []model.ExportResult{
				{
					Table:        "games",
					RowsExported: 42,
					FilePath:     "gs://bucket/exports/20250311/games/030000.000.jsonl",
					StartTime:    time.Now(),
					EndTime:      time.Now(),
					Duration:     100 * time.Millisecond,
					IsSuccess:    true,
				},
				{
					Table:        "players",
					RowsExported: 10,
					FilePath:     "gs://bucket/exports/20250311/players/030000.000.jsonl",
					StartTime:    time.Now(),
					EndTime:      time.Now(),
					Duration:     50 * time.Millisecond,
					IsSuccess:    true,
				},
			}

			h := New(newMockFactory(results))
			body := `{"tables": ["games", "players"], "mode": "incremental"}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()

			h(w, r)

			require.Equal(t, http.StatusOK, w.Code)

			var resp ExportResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			require.True(t, resp.IsSuccess)
			require.Len(t, resp.Results, 2)
			require.Equal(t, int64(42), resp.Results[0].RowsExported)
		})

		t.Run("2 テーブルのうち 1 テーブルが失敗したとき、500 を返し失敗したテーブルのエラー内容が結果に残る", func(t *testing.T) {
			results := []model.ExportResult{
				{
					Table:        "games",
					RowsExported: 42,
					IsSuccess:    true,
				},
				{
					Table:     "players",
					IsSuccess: false,
					Error:     "query postgres: connection refused",
				},
			}

			h := New(newMockFactory(results))
			body := `{"tables": ["games", "players"], "mode": "incremental"}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()

			h(w, r)

			require.Equal(t, http.StatusInternalServerError, w.Code)

			var resp ExportResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			require.False(t, resp.IsSuccess)
			require.Contains(t, resp.Message, "Some exports failed")
			require.Contains(t, resp.Results[1].Error, "connection refused")
		})

		t.Run("全テーブルが失敗したとき、500 を返す", func(t *testing.T) {
			results := []model.ExportResult{
				{
					Table:     "games",
					IsSuccess: false,
					Error:     "query source: connection refused",
				},
			}

			h := New(newMockFactory(results))
			body := `{"tables": ["games"], "mode": "incremental"}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()

			h(w, r)

			require.Equal(t, http.StatusInternalServerError, w.Code)

			var resp ExportResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			require.False(t, resp.IsSuccess)
			require.Contains(t, resp.Message, "Some exports failed")
		})

		t.Run("checkpoint 書き込み失敗のとき、500 を返し checkpoint の失敗が示される", func(t *testing.T) {
			results := []model.ExportResult{
				{
					Table:              "games",
					RowsExported:       42,
					IsSuccess:          false,
					IsCheckpointFailed: true,
					Error:              "checkpoint update failed after warehouse load: table=games: firestore: UNAVAILABLE",
				},
			}

			h := New(newMockFactory(results))
			body := `{"tables": ["games"], "mode": "incremental"}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()

			h(w, r)

			require.Equal(t, http.StatusInternalServerError, w.Code)

			var resp ExportResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			require.False(t, resp.IsSuccess)
			require.Contains(t, resp.Message, "Checkpoint")
		})

		t.Run("checkpoint 書き込み失敗のテーブルと通常の失敗のテーブルが混在するとき、500 を返し checkpoint の失敗が示される", func(t *testing.T) {
			results := []model.ExportResult{
				{
					Table:              "games",
					IsSuccess:          false,
					IsCheckpointFailed: true,
					Error:              "checkpoint update failed after warehouse load: table=games: firestore: UNAVAILABLE",
				},
				{
					Table:     "players",
					IsSuccess: false,
					Error:     "query postgres: connection refused",
				},
			}

			h := New(newMockFactory(results))
			body := `{"tables": ["games", "players"], "mode": "incremental"}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()

			h(w, r)

			require.Equal(t, http.StatusInternalServerError, w.Code)

			var resp ExportResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			require.False(t, resp.IsSuccess)
			require.Contains(t, resp.Message, "Checkpoint")
			require.Len(t, resp.Results, 2)
		})

		t.Run("エクスポート結果が0件のとき、200と成功レスポンスを返し件数0のメッセージになる", func(t *testing.T) {
			h := New(newMockFactory([]model.ExportResult{}))
			body := `{"tables": ["games"], "mode": "incremental"}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()

			h(w, r)

			require.Equal(t, http.StatusOK, w.Code)

			var resp ExportResponse
			require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
			require.True(t, resp.IsSuccess)
			require.Len(t, resp.Results, 0)
			require.Equal(t, "Successfully exported 0 tables", resp.Message)
		})

		t.Run("サービス初期化に失敗したとき、500 を返す", func(t *testing.T) {
			factory := func(_ context.Context) (service.Service, error) {
				return nil, fmt.Errorf("missing required environment variables")
			}

			h := New(factory)
			body := `{"tables": ["games"]}`
			r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
			w := httptest.NewRecorder()

			h(w, r)

			require.Equal(t, http.StatusInternalServerError, w.Code)
		})
	})
}

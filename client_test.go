package gohoo

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestQuoteSummaryMergesModules(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v10/finance/quoteSummary/AAPL", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"quoteSummary": {
				"result": [
					{"price": {"regularMarketPrice": {"raw": 150.23, "fmt": "150.23"}}},
					{"summaryDetail": {"trailingPE": {"raw": 28.5, "fmt": "28.50"}}}
				],
				"error": null
			}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	data, err := client.QuoteSummary("AAPL", []string{"price", "summaryDetail"})
	if err != nil {
		t.Fatalf("QuoteSummary error: %v", err)
	}
	price, ok := data["price"].(map[string]interface{})
	if !ok {
		t.Fatalf("data[price] missing or wrong type: %v", data["price"])
	}
	if got := Field(price, "regularMarketPrice"); got != 150.23 {
		t.Errorf("regularMarketPrice = %v, want 150.23", got)
	}
	if _, ok := data["summaryDetail"]; !ok {
		t.Error("data[summaryDetail] missing after merge")
	}
}

func TestQuoteSummaryBusinessError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v10/finance/quoteSummary/FAKE", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"quoteSummary": {
				"result": [],
				"error": {"code": "Not Found", "description": "No data found, symbol may be delisted"}
			}
		}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	if _, err := client.QuoteSummary("FAKE", []string{"price"}); err == nil {
		t.Error("QuoteSummary with Yahoo error expected error, got nil")
	} else if !strings.Contains(err.Error(), "No data found") {
		t.Errorf("error = %v, want it to mention Yahoo's description", err)
	}
}

func TestQuoteSummaryEmptyResultIsNotError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v10/finance/quoteSummary/AAPL", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"quoteSummary": {"result": [], "error": null}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	data, err := client.QuoteSummary("AAPL", []string{"price"})
	if err != nil {
		t.Fatalf("QuoteSummary error: %v", err)
	}
	if len(data) != 0 {
		t.Errorf("data = %v, want empty map", data)
	}
}

func TestFundamentalsTimeseries(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/ws/fundamentals-timeseries/v1/finance/timeseries/AAPL", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"timeseries": {"result": [{"meta": {"symbol": ["AAPL"]}}]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	data, err := client.FundamentalsTimeseries("AAPL", []string{"annualTotalRevenue"})
	if err != nil {
		t.Fatalf("FundamentalsTimeseries error: %v", err)
	}
	if _, ok := data["timeseries"]; !ok {
		t.Errorf("data = %v, want key \"timeseries\"", data)
	}
}

func TestChart(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/AAPL", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("range"); got != "1y" {
			t.Errorf("range query = %q, want 1y", got)
		}
		if got := r.URL.Query().Get("interval"); got != "1d" {
			t.Errorf("interval query = %q, want 1d", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(chartFixture))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	data, err := client.Chart("AAPL", "1y", "1d")
	if err != nil {
		t.Fatalf("Chart error: %v", err)
	}
	if _, ok := data["chart"]; !ok {
		t.Errorf("data = %v, want key \"chart\"", data)
	}
}

func TestDoGetHTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/MISSING", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("symbol not found"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	_, err := client.Chart("MISSING", "1y", "1d")
	if err == nil {
		t.Fatal("expected error for HTTP 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") || !strings.Contains(err.Error(), "symbol not found") {
		t.Errorf("error = %v, want it to mention status code and body", err)
	}
}

func TestDoGetAuthedRetriesOn429ThenSucceeds(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/AAPL", func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte("rate limited"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(chartFixture))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	data, err := client.Chart("AAPL", "1y", "1d")
	if err != nil {
		t.Fatalf("Chart error tras reintentos: %v", err)
	}
	if _, ok := data["chart"]; !ok {
		t.Errorf("data = %v, want key \"chart\"", data)
	}
	if got := calls.Load(); got != 3 {
		t.Errorf("llamadas al servidor = %d, want 3 (2 fallidas + 1 exitosa)", got)
	}
}

func TestDoGetAuthedGivesUpAfterMaxAttempts(t *testing.T) {
	var calls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/AAPL", func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("down"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	if _, err := client.Chart("AAPL", "1y", "1d"); err == nil {
		t.Fatal("Chart con 503 persistente: expected error, got nil")
	}
	if got := calls.Load(); got != maxAttempts {
		t.Errorf("llamadas al servidor = %d, want %d (todos los intentos agotados)", got, maxAttempts)
	}
}

func TestQuoteSummaryInvalidJSON(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v10/finance/quoteSummary/AAPL", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte("not json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	if _, err := client.QuoteSummary("AAPL", []string{"price"}); err == nil {
		t.Error("QuoteSummary with invalid JSON expected error, got nil")
	}
}

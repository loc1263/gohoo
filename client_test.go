package gohoo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
)

func TestQuoteSummaryMergesModules(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v10/finance/quoteSummary/AAPL", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
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
	price, ok := data["price"].(map[string]any)
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
		_, _ = w.Write([]byte(`{
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
		_, _ = w.Write([]byte(`{"quoteSummary": {"result": [], "error": null}}`))
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
		_, _ = w.Write([]byte(`{"timeseries": {"result": [{"meta": {"symbol": ["AAPL"]}}]}}`))
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
		_, _ = w.Write([]byte(chartFixture))
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
		_, _ = w.Write([]byte("symbol not found"))
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
			_, _ = w.Write([]byte("rate limited"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(chartFixture))
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
		_, _ = w.Write([]byte("down"))
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
		_, _ = w.Write([]byte("not json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	if _, err := client.QuoteSummary("AAPL", []string{"price"}); err == nil {
		t.Error("QuoteSummary with invalid JSON expected error, got nil")
	}
}

// roundTripFunc adapta una función a http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// redirectDefaultTransport hace que http.DefaultTransport mande todo al
// servidor de prueba mientras dure el test. NewClient no acepta opciones, así
// que es la única forma de probarlo sin tocar Yahoo. No usar con t.Parallel.
func redirectDefaultTransport(t *testing.T, srv *httptest.Server) {
	t.Helper()
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	orig := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r.URL.Scheme, r.URL.Host = base.Scheme, base.Host
		return orig.RoundTrip(r)
	})
	t.Cleanup(func() { http.DefaultTransport = orig })
}

func TestNewClientBootstraps(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotFound) })
	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("crumb-default\n"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	redirectDefaultTransport(t, srv)

	client, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	if got := client.getCrumb(); got != "crumb-default" {
		t.Errorf("crumb = %q, want crumb-default", got)
	}
}

func TestNewClientBootstrapErrors(t *testing.T) {
	cases := []struct {
		name    string
		handler http.HandlerFunc
		want    string
	}{
		{"crumb vacío", func(w http.ResponseWriter, r *http.Request) {}, "respuesta de crumb inválida"},
		{"crumb HTML", func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("<html>consent</html>")) }, "respuesta de crumb inválida"},
		{"HTTP 500", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }, "no se pudo obtener crumb"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			mux := http.NewServeMux()
			mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
			mux.HandleFunc("/v1/test/getcrumb", tc.handler)
			srv := httptest.NewServer(mux)
			defer srv.Close()
			redirectDefaultTransport(t, srv)

			client, err := NewClient()
			if err == nil || client != nil {
				t.Fatalf("NewClient() = %v, %v; want error", client, err)
			}
			if !strings.HasPrefix(err.Error(), "gohoo: ") || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %q, want prefijo gohoo: y %q", err, tc.want)
			}
		})
	}
}

func TestRoundTripInvalidMethod(t *testing.T) {
	c := &Client{http: &http.Client{}}
	if _, err := c.do(context.Background(), "MÉTODO MALO", "http://example.invalid/", nil); err == nil {
		t.Error("método inválido: want error")
	}
}

func TestRoundTripTruncatedBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", "100")
		_, _ = w.Write([]byte("corto"))
	}))
	defer srv.Close()

	c := newTestClient(t, srv)
	_, status, err := c.roundTrip(context.Background(), http.MethodGet, srv.URL, nil)
	if !errors.Is(err, io.ErrUnexpectedEOF) || status != http.StatusOK {
		t.Errorf("status=%d err=%v, want 200 y io.ErrUnexpectedEOF", status, err)
	}
}

func TestReauthAfter401Fails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	mux.HandleFunc("/v10/finance/quoteSummary/AAPL", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newTestClient(t, srv)
	recordSleep(c)
	_, err := c.QuoteSummaryContext(context.Background(), "AAPL", []string{"price"})
	if err == nil || !strings.Contains(err.Error(), "re-autenticación tras HTTP 401 falló") {
		t.Errorf("err = %v, want fallo de re-autenticación", err)
	}
}

func TestQuoteSummaryLazyBootstrapFails(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {})
	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) {}) // crumb vacío
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := newRewritingClient(t, srv, WithLazyBootstrap())
	_, err := c.QuoteSummaryContext(context.Background(), "AAPL", []string{"price"})
	if err == nil || !strings.Contains(err.Error(), "quoteSummary de AAPL") {
		t.Errorf("err = %v, want error de bootstrap de quoteSummary", err)
	}
}

func TestQuoteSummaryHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).QuoteSummaryContext(context.Background(), "NOPE", []string{"price"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestFundamentalsAndChartInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("no es json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	if _, err := c.FundamentalsTimeseriesContext(context.Background(), "AAPL", []string{"annualTotalRevenue"}); err == nil ||
		!strings.Contains(err.Error(), "fundamentals-timeseries") {
		t.Errorf("FundamentalsTimeseries: err = %v, want error de parseo", err)
	}
	if _, err := c.ChartContext(context.Background(), "AAPL", "1y", "1d"); err == nil ||
		!strings.Contains(err.Error(), "no se pudo parsear chart") {
		t.Errorf("Chart: err = %v, want error de parseo", err)
	}
}

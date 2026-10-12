package gohoo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// recordSleep reemplaza la espera del backoff por una que solo anota los
// retardos pedidos (respetando la cancelación del contexto).
func recordSleep(c *Client) *[]time.Duration {
	var mu sync.Mutex
	delays := &[]time.Duration{}
	testCfg(c).sleep = func(ctx context.Context, d time.Duration) error {
		mu.Lock()
		*delays = append(*delays, d)
		mu.Unlock()
		return ctx.Err()
	}
	return delays
}

// testCfg devuelve la configuración del cliente, creándola si es nil (los
// clientes de newTestClient no la tienen).
func testCfg(c *Client) *config {
	if c.cfg == nil {
		c.cfg = &config{}
	}
	return c.cfg
}

// newRewritingClient crea un Client con NewClientWithOptions cuyo transporte
// redirige todo (incluido fc.yahoo.com) al servidor de prueba.
func newRewritingClient(t *testing.T, srv *httptest.Server, opts ...Option) *Client {
	t.Helper()
	base, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	opts = append([]Option{WithHTTPClient(&http.Client{Transport: &rewriteTransport{base: base}})}, opts...)
	c, err := NewClientWithOptions(opts...)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestDefaultRetryDelaysAndMessage(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	delays := recordSleep(client)
	_, err := client.Chart("AAPL", "1y", "1d")
	if err == nil || !strings.HasPrefix(err.Error(), "agotados 4 intentos: HTTP 503 en ") {
		t.Fatalf("error = %v, want prefijo \"agotados 4 intentos: HTTP 503 en \"", err)
	}
	want := []time.Duration{200 * time.Millisecond, 400 * time.Millisecond, 800 * time.Millisecond}
	if got := *delays; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Errorf("retardos = %v, want %v", got, want)
	}
	if calls.Load() != 4 {
		t.Errorf("llamadas = %d, want 4", calls.Load())
	}
	var se *httpStatusError
	if !errors.As(err, &se) || se.StatusCode != 503 {
		t.Errorf("errors.As(*httpStatusError) falló: %v", err)
	}
}

func TestDefaultRenewsCrumbOn401(t *testing.T) {
	var chartCalls, crumbCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) {
		crumbCalls.Add(1)
		_, _ = w.Write([]byte("nuevo-crumb"))
	})
	mux.HandleFunc("/v10/finance/quoteSummary/AAPL", func(w http.ResponseWriter, r *http.Request) {
		if chartCalls.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if got := r.URL.Query().Get("crumb"); got != "nuevo-crumb" {
			t.Errorf("crumb = %q, want nuevo-crumb", got)
		}
		_, _ = w.Write([]byte(`{"quoteSummary":{"result":[{"price":{}}],"error":null}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	recordSleep(client)
	if _, err := client.QuoteSummary("AAPL", []string{"price"}); err != nil {
		t.Fatal(err)
	}
	if crumbCalls.Load() != 1 {
		t.Errorf("getcrumb llamado %d veces, want 1", crumbCalls.Load())
	}
}

// --- Opciones ---

func TestNewClientWithOptionsLazyMakesNoRequests(t *testing.T) {
	var calls atomic.Int32
	var gotUA atomic.Value
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		gotUA.Store(r.UserAgent())
		_, _ = w.Write([]byte(chartFixture))
	}))
	defer srv.Close()

	client, err := NewClientWithOptions(WithLazyBootstrap(), WithBaseURL(srv.URL+"/"), WithUserAgent("vanguard/1.0"))
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatalf("NewClientWithOptions(WithLazyBootstrap) hizo %d requests, want 0", calls.Load())
	}
	if _, err := client.ChartContext(context.Background(), "AAPL", "1y", "1d"); err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 1 {
		t.Errorf("Chart con bootstrap perezoso hizo %d requests, want 1 (sin crumb)", calls.Load())
	}
	if gotUA.Load() != "vanguard/1.0" {
		t.Errorf("User-Agent = %v, want vanguard/1.0", gotUA.Load())
	}
}

func TestLazyBootstrapRunsOnceForQuoteSummary(t *testing.T) {
	var crumbCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {}) // fc.yahoo.com
	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) {
		crumbCalls.Add(1)
		_, _ = w.Write([]byte("crumb-perezoso"))
	})
	mux.HandleFunc("/v10/finance/quoteSummary/AAPL", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("crumb"); got != "crumb-perezoso" {
			t.Errorf("crumb = %q, want crumb-perezoso", got)
		}
		_, _ = w.Write([]byte(`{"quoteSummary":{"result":[{"price":{}}],"error":null}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newRewritingClient(t, srv, WithLazyBootstrap())
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := client.QuoteSummaryContext(context.Background(), "AAPL", []string{"price"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if crumbCalls.Load() != 1 {
		t.Errorf("getcrumb llamado %d veces, want 1", crumbCalls.Load())
	}
}

func TestNewClientWithOptionsEagerBootstrap(t *testing.T) {
	var consent atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		consent.Add(1)
		w.WriteHeader(http.StatusNotFound)
	})
	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("crumb-ok"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newRewritingClient(t, srv)
	if got := client.getCrumb(); got != "crumb-ok" {
		t.Errorf("crumb = %q, want crumb-ok", got)
	}
	if consent.Load() != 1 {
		t.Errorf("visitas a fc.yahoo.com = %d, want 1", consent.Load())
	}
}

func TestWithHTTPClientAddsJarWithoutMutating(t *testing.T) {
	h := &http.Client{Timeout: time.Second}
	client, err := NewClientWithOptions(WithHTTPClient(h), WithLazyBootstrap())
	if err != nil {
		t.Fatal(err)
	}
	if h.Jar != nil {
		t.Error("WithHTTPClient modificó el http.Client del llamador")
	}
	if client.http.Jar == nil || client.http.Timeout != time.Second {
		t.Errorf("http.Client interno = %+v, want Jar y Timeout 1s", client.http)
	}
}

func TestWithBaseURLInvalid(t *testing.T) {
	if _, err := NewClientWithOptions(WithBaseURL("query1.finance.yahoo.com"), WithLazyBootstrap()); err == nil {
		t.Error("base URL sin esquema: want error")
	}
}

// --- Contexto ---

func TestContextCanceledDuringBackoff(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	ctx, cancel := context.WithCancel(context.Background())
	testCfg(client).sleep = func(ctx context.Context, d time.Duration) error {
		cancel()
		return sleepContext(ctx, d)
	}
	_, err := client.ChartContext(ctx, "AAPL", "1y", "1d")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if calls.Load() != 1 {
		t.Errorf("llamadas = %d, want 1 (no debe consumir los reintentos)", calls.Load())
	}
	if !strings.Contains(err.Error(), "HTTP 503") {
		t.Errorf("error = %v, want que mencione el último error", err)
	}
}

func TestContextDeadlineOnHangingServer(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-release:
		case <-r.Context().Done():
		}
	}))
	defer srv.Close()
	defer close(release)

	client := newTestClient(t, srv)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := client.ChartContext(ctx, "AAPL", "1y", "1d")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error = %v, want DeadlineExceeded", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Errorf("tardó %v en volver", elapsed)
	}
}

// --- Errores tipados ---

func TestConsentURLOverride(t *testing.T) {
	c := &Client{}
	if got := c.consentURL(); got != defaultConsentURL {
		t.Errorf("consentURL() = %q, want %q", got, defaultConsentURL)
	}
	testCfg(c).consentURL = "http://consent.test"
	if got := c.consentURL(); got != "http://consent.test" {
		t.Errorf("consentURL() = %q, want http://consent.test", got)
	}
}

func TestSleepContextNonPositive(t *testing.T) {
	if err := sleepContext(context.Background(), 0); err != nil {
		t.Errorf("sleepContext(0) = %v, want nil", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := sleepContext(ctx, -time.Second); !errors.Is(err, context.Canceled) {
		t.Errorf("sleepContext(cancelado, -1s) = %v, want context.Canceled", err)
	}
}

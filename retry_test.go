package gohoo

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitIsDetectableAfterRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "7")
		w.WriteHeader(http.StatusTooManyRequests)
		w.Write([]byte("slow down"))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	recordSleep(client)
	_, err := client.Chart("AAPL", "1y", "1d")
	if !IsRateLimit(err) || !errors.Is(err, ErrRateLimit) {
		t.Fatalf("IsRateLimit(%v) = false", err)
	}
	if !strings.HasPrefix(err.Error(), "agotados 4 intentos:") {
		t.Errorf("error = %v, want prefijo agotados", err)
	}
	var se *StatusError
	if !errors.As(err, &se) || se.RetryAfter != 7*time.Second {
		t.Errorf("StatusError.RetryAfter = %v, want 7s", se)
	}
	if errors.Is(err, ErrNotFound) {
		t.Error("429 no debe ser ErrNotFound")
	}
}

func TestStatusErrorSentinels(t *testing.T) {
	cases := []struct {
		code int
		want error
	}{
		{http.StatusTooManyRequests, ErrRateLimit},
		{http.StatusNotFound, ErrNotFound},
		{http.StatusUnauthorized, ErrUnauthorized},
	}
	for _, c := range cases {
		err := error(&StatusError{StatusCode: c.code})
		for _, s := range []error{ErrRateLimit, ErrNotFound, ErrUnauthorized} {
			if got := errors.Is(err, s); got != (s == c.want) {
				t.Errorf("errors.Is(HTTP %d, %v) = %v", c.code, s, got)
			}
		}
	}
	if errors.Is(&StatusError{StatusCode: 500}, ErrRateLimit) {
		t.Error("HTTP 500 no debe ser ErrRateLimit")
	}
}

func TestPersistent401IsUnauthorized(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/test/getcrumb", func(w http.ResponseWriter, r *http.Request) { w.Write([]byte("c")) })
	mux.HandleFunc("/v8/finance/chart/AAPL", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	recordSleep(client)
	if _, err := client.Chart("AAPL", "1y", "1d"); !errors.Is(err, ErrUnauthorized) {
		t.Errorf("error = %v, want ErrUnauthorized", err)
	}
}

func TestParseRetryAfter(t *testing.T) {
	now := time.Date(2026, 10, 8, 12, 0, 0, 0, time.UTC)
	cases := map[string]time.Duration{
		"":                              0,
		"30":                            30 * time.Second,
		"-1":                            0,
		"basura":                        0,
		"Thu, 08 Oct 2026 12:01:00 GMT": time.Minute,
		"Thu, 08 Oct 2026 11:00:00 GMT": 0,
	}
	for h, want := range cases {
		if got := parseRetryAfter(h, now); got != want {
			t.Errorf("parseRetryAfter(%q) = %v, want %v", h, got, want)
		}
	}
}

// --- Política de reintentos ---

func TestRetryPolicyNoRateLimitRetry(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	testCfg(client).retry = &RetryPolicy{MaxAttempts: 4, RetryRateLimit: false}
	delays := recordSleep(client)
	_, err := client.Chart("AAPL", "1y", "1d")
	if !IsRateLimit(err) {
		t.Fatalf("error = %v, want rate limit", err)
	}
	if calls.Load() != 1 || len(*delays) != 0 {
		t.Errorf("llamadas = %d, esperas = %v; want 1 llamada y ninguna espera", calls.Load(), *delays)
	}
}

func TestRetryPolicySingleAttempt(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	testCfg(client).retry = &RetryPolicy{MaxAttempts: 1}
	recordSleep(client)
	if _, err := client.Chart("AAPL", "1y", "1d"); err == nil || calls.Load() != 1 {
		t.Errorf("err = %v, llamadas = %d; want error y 1 llamada", err, calls.Load())
	}
}

func TestRetryPolicyHonorRetryAfterMaxDelayAndOnRetry(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "5")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()

	var hooks []int
	client := newTestClient(t, srv)
	p := DefaultRetryPolicy()
	p.MaxAttempts = 3
	p.HonorRetryAfter = true
	p.MaxDelay = 3 * time.Second
	p.OnRetry = func(attempt int, err error, d time.Duration) {
		if !IsRateLimit(err) {
			t.Errorf("OnRetry err = %v", err)
		}
		hooks = append(hooks, attempt)
	}
	testCfg(client).retry = &p
	delays := recordSleep(client)
	client.Chart("AAPL", "1y", "1d")

	if got := *delays; len(got) != 2 || got[0] != 3*time.Second || got[1] != 3*time.Second {
		t.Errorf("retardos = %v, want [3s 3s] (Retry-After 5s acotado por MaxDelay)", got)
	}
	if len(hooks) != 2 || hooks[0] != 1 || hooks[1] != 2 {
		t.Errorf("OnRetry attempts = %v, want [1 2]", hooks)
	}
}

func TestRetryPolicyJitterBounds(t *testing.T) {
	p := RetryPolicy{MaxAttempts: 4, BaseDelay: 100 * time.Millisecond, Jitter: 0.5}
	var r jitterRand
	for range 200 {
		d := p.delay(2, nil, r.Float64) // base 200ms
		if d < 200*time.Millisecond || d > 300*time.Millisecond {
			t.Fatalf("delay con jitter = %v, want en [200ms, 300ms]", d)
		}
	}
}

func TestRetryPolicyZeroValuesTakeDefaults(t *testing.T) {
	c := &Client{cfg: &config{retry: &RetryPolicy{}}}
	p := c.retryPolicy()
	if p.MaxAttempts != 4 || p.BaseDelay != 200*time.Millisecond {
		t.Errorf("política = %+v, want MaxAttempts 4 y BaseDelay 200ms", p)
	}
}

// --- ChartRange ---

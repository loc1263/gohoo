package gohoo

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestLimiterAndHookOnEveryAttemptIncludingBootstrap(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/test/getcrumb":
			_, _ = io.WriteString(w, "C1")
		case "/v10/finance/quoteSummary/SQM-B.SN":
			if calls.Add(1) == 1 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			_, _ = io.WriteString(w, `{"quoteSummary":{"result":[{"price":{}}],"error":null}}`)
		default:
			w.WriteHeader(http.StatusNotFound) // fc.yahoo.com
		}
	}))
	defer srv.Close()

	var waits atomic.Int32
	var mu sync.Mutex
	var events []RequestEvent
	c := newRewritingClient(t, srv, WithLazyBootstrap(),
		WithLimiter(LimiterFunc(func(context.Context) error { waits.Add(1); return nil })),
		WithRequestHook(func(ev RequestEvent) { mu.Lock(); events = append(events, ev); mu.Unlock() }))
	recordSleep(c)

	if _, err := c.QuoteSummaryContext(context.Background(), "SQM-B.SN", []string{"price"}); err != nil {
		t.Fatal(err)
	}
	// consent + getcrumb + 2 intentos de quoteSummary
	if waits.Load() != 4 || len(events) != 4 {
		t.Fatalf("waits=%d events=%d", waits.Load(), len(events))
	}
	want := []struct {
		endpoint, symbol string
		attempt, status  int
	}{{"consent", "", 1, 404}, {"getcrumb", "", 1, 200}, {"quoteSummary", "SQM-B.SN", 1, 503}, {"quoteSummary", "SQM-B.SN", 2, 200}}
	for i, w := range want {
		ev := events[i]
		if ev.Endpoint != w.endpoint || ev.Symbol != w.symbol || ev.Attempt != w.attempt || ev.StatusCode != w.status || ev.Method != "GET" {
			t.Errorf("evento %d = %+v", i, ev)
		}
		if strings.Contains(ev.URL, "crumb=") {
			t.Errorf("la URL del evento no debe traer el crumb: %s", ev.URL)
		}
	}
	var se *StatusError
	if !errors.As(events[2].Err, &se) || se.StatusCode != 503 || events[3].Err != nil {
		t.Errorf("errores de los eventos: %v / %v", events[2].Err, events[3].Err)
	}
}

func TestLimiterErrorIsNotRetried(t *testing.T) {
	var reqs atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { reqs.Add(1) }))
	defer srv.Close()
	c := newTestClient(t, srv)
	boom := errors.New("sin cupo")
	testCfg(c).limiter = LimiterFunc(func(context.Context) error { return boom })

	_, err := c.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"})
	if !errors.Is(err, boom) || reqs.Load() != 0 {
		t.Errorf("err=%v requests=%d", err, reqs.Load())
	}
}

func TestRequestEventClassification(t *testing.T) {
	cases := []struct{ url, endpoint, symbol string }{
		{"https://q/v8/finance/chart/%5EVIX?range=1y", "chart", "^VIX"},
		{"https://q/ws/fundamentals-timeseries/v1/finance/timeseries/AAPL?type=x", "fundamentals", "AAPL"},
		{"https://q/v7/finance/quote?symbols=A%2CB&crumb=z", "quote", "A,B"},
		{"https://q/v1/finance/screener?crumb=z", "screener", ""},
		{"https://q/v1/finance/screener/predefined/saved?scrIds=day_gainers", "screener", "day_gainers"},
		{"https://q/v99/otro", "other", ""},
		{"https://fc.yahoo.com", "consent", ""},
	}
	for _, tc := range cases {
		ev := newRequestEvent("GET", tc.url, defaultConsentURL, 1, 200, 0, nil)
		if ev.Endpoint != tc.endpoint || ev.Symbol != tc.symbol || strings.Contains(ev.URL, "crumb") {
			t.Errorf("%s → %+v", tc.url, ev)
		}
	}
}

func TestRateLimitPolicySeparateBudget(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch calls.Add(1) {
		case 1, 2, 3:
			w.Header().Set("Retry-After", "90")
			w.WriteHeader(http.StatusTooManyRequests)
		case 4:
			w.WriteHeader(http.StatusBadGateway)
		default:
			_, _ = w.Write([]byte(chartFixture))
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	delays := recordSleep(c)
	var retries []int
	testCfg(c).retry = &RetryPolicy{
		MaxAttempts: 2, BaseDelay: 100 * time.Millisecond,
		RateLimit: &RateLimitPolicy{MaxRetries: 3, BaseDelay: 5 * time.Second, MaxDelay: 8 * time.Second, HonorRetryAfter: true},
		OnRetry:   func(attempt int, _ error, _ time.Duration) { retries = append(retries, attempt) },
	}
	testCfg(c).retry.RateLimit.HonorRetryAfter = false

	if _, err := c.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"}); err != nil {
		t.Fatal(err)
	}
	// 3 esperas por 429 (5s, 8s tope, 8s) que no consumen MaxAttempts + 1 por el 502 (100ms).
	want := []time.Duration{5 * time.Second, 8 * time.Second, 8 * time.Second, 100 * time.Millisecond}
	if len(*delays) != len(want) {
		t.Fatalf("esperas = %v", *delays)
	}
	for i := range want {
		if (*delays)[i] != want[i] {
			t.Errorf("espera %d = %v, want %v", i, (*delays)[i], want[i])
		}
	}
	if len(retries) != 4 || retries[0] != 1 || retries[3] != 4 {
		t.Errorf("OnRetry = %v", retries)
	}
}

func TestRateLimitPolicyExhaustedAndRetryAfter(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "90")
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	delays := recordSleep(c)
	testCfg(c).retry = &RetryPolicy{RateLimit: &RateLimitPolicy{MaxRetries: 2, BaseDelay: time.Second, MaxDelay: 2 * time.Second, HonorRetryAfter: true}}

	_, err := c.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"})
	if !IsRateLimit(err) || !strings.Contains(err.Error(), "agotados 2 reintentos") {
		t.Errorf("err = %v", err)
	}
	// Retry-After (90 s) no se acota con MaxDelay.
	if len(*delays) != 2 || (*delays)[0] != 90*time.Second || (*delays)[1] != 90*time.Second {
		t.Errorf("esperas = %v", *delays)
	}
}

func TestRateLimitPolicyZeroRetriesReturnsImmediately(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	recordSleep(c)
	testCfg(c).retry = &RetryPolicy{RateLimit: &RateLimitPolicy{}}
	if _, err := c.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"}); !IsRateLimit(err) || calls.Load() != 1 {
		t.Errorf("err=%v llamadas=%d", err, calls.Load())
	}
}

func TestRateLimitPolicyJitter(t *testing.T) {
	p := RateLimitPolicy{BaseDelay: time.Second, Jitter: time.Second}
	if d := p.delay(2, nil, func() float64 { return 0.5 }); d != 2500*time.Millisecond {
		t.Errorf("delay = %v", d)
	}
}

func TestChartBodyRateLimitRetriedOnlyWithRateLimitPolicy(t *testing.T) {
	tooMany := string(readFixture(t, "chart_too_many.json"))
	newSrv := func(calls *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if calls.Add(1) == 1 {
				_, _ = io.WriteString(w, tooMany)
				return
			}
			_, _ = w.Write([]byte(chartFixture))
		}))
	}

	// Por defecto (sin RateLimit) se mantiene el comportamiento de v0.2: error sin reintento.
	var calls atomic.Int32
	srv := newSrv(&calls)
	c := newTestClient(t, srv)
	recordSleep(c)
	if _, err := c.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"}); !IsRateLimit(err) || calls.Load() != 1 {
		t.Errorf("default: err=%v llamadas=%d", err, calls.Load())
	}
	srv.Close()

	var calls2 atomic.Int32
	srv = newSrv(&calls2)
	defer srv.Close()
	c = newTestClient(t, srv)
	recordSleep(c)
	testCfg(c).retry = &RetryPolicy{RateLimit: &RateLimitPolicy{MaxRetries: 1}}
	res, err := c.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"})
	if err != nil || len(res.Bars) == 0 || calls2.Load() != 2 {
		t.Errorf("con RateLimit: err=%v llamadas=%d", err, calls2.Load())
	}
}

func TestRetryPolicyContextOverridesClient(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusTooManyRequests)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	recordSleep(c)
	testCfg(c).retry = &RetryPolicy{RateLimit: &RateLimitPolicy{MaxRetries: 5}}

	ctx := WithRetryPolicyContext(context.Background(), RetryPolicy{MaxAttempts: 1})
	if p, ok := RetryPolicyFromContext(ctx); !ok || p.MaxAttempts != 1 {
		t.Errorf("RetryPolicyFromContext = %+v, %v", p, ok)
	}
	if _, err := c.ChartRange(ctx, "AAPL", ChartParams{Range: "1y"}); !IsRateLimit(err) || calls.Load() != 1 {
		t.Errorf("con contexto: err=%v llamadas=%d", err, calls.Load())
	}
	calls.Store(0)
	if _, err := c.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"}); !IsRateLimit(err) || calls.Load() != 6 {
		t.Errorf("sin contexto: err=%v llamadas=%d", err, calls.Load())
	}
	if _, ok := RetryPolicyFromContext(context.Background()); ok {
		t.Error("un contexto sin política no debe traer una")
	}
}

func TestLimiterErrorMessage(t *testing.T) {
	err := &limiterError{errors.New("sin cupo")}
	if err.Error() != "gohoo: limiter: sin cupo" {
		t.Errorf("Error() = %q", err.Error())
	}
}

func TestRequestEventUnparseableURL(t *testing.T) {
	ev := newRequestEvent(http.MethodGet, "://sin-esquema", defaultConsentURL, 1, 0, 0, nil)
	if ev.Endpoint != "other" || ev.Symbol != "" || ev.URL != "://sin-esquema" {
		t.Errorf("RequestEvent = %+v, want Endpoint other", ev)
	}
}

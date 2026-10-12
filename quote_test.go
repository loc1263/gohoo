package gohoo

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	_ "time/tzdata"
)

func TestQuotesParsesFixture(t *testing.T) {
	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v7/finance/quote" || r.URL.Query().Get("crumb") != "test-crumb" || r.URL.Query().Get("formatted") != "false" {
			t.Errorf("request inesperada: %s", r.URL)
		}
		got = r.URL.Query().Get("symbols")
		_, _ = w.Write(readFixture(t, "quote_mixed.json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	qs, err := c.Quotes(context.Background(), []string{"AAPL", " SQM-B.SN ", "", "CLP=X", "^VIX", "NOEXISTE123", "BTC-USD", "SPY", "AAPL"})
	if err != nil {
		t.Fatal(err)
	}
	if got != "AAPL,SQM-B.SN,CLP=X,^VIX,NOEXISTE123,BTC-USD,SPY" {
		t.Errorf("symbols = %q (sin vacíos ni repetidos, en orden)", got)
	}
	if len(qs) != 6 {
		t.Fatalf("len = %d", len(qs))
	}
	by := map[string]Quote{}
	for _, q := range qs {
		by[q.Symbol] = q
	}
	aapl := by["AAPL"]
	if aapl.QuoteType != "EQUITY" || aapl.Currency != "USD" || aapl.Price != 339.645 || aapl.PreviousClose != 336.64 ||
		aapl.MarketCap != 4956840001536 || aapl.ShortName == "" || aapl.Exchange == "" || aapl.Raw["trailingPE"] == nil {
		t.Errorf("AAPL = %+v", aapl)
	}
	if aapl.Time.IsZero() || aapl.Time.Location().String() != "America/New_York" {
		t.Errorf("AAPL.Time = %v", aapl.Time)
	}
	if q := by["SQM-B.SN"]; q.Currency != "CLP" || q.Price != 62400 {
		t.Errorf("SQM-B.SN = %+v", q)
	}
	if q := by["CLP=X"]; q.QuoteType != "CURRENCY" || q.Price != 975.06 || q.MarketCap != 0 {
		t.Errorf("CLP=X = %+v", q)
	}
	if q := by["^VIX"]; q.QuoteType != "INDEX" || q.Price != 15.21 {
		t.Errorf("^VIX = %+v", q)
	}
}

func TestQuotesBatchesAndPartialFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		syms := strings.Split(r.URL.Query().Get("symbols"), ",")
		if n == 3 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write(readFixture(t, "quote_bad_request.json"))
			return
		}
		if len(syms) != QuoteBatchSize {
			t.Errorf("lote %d con %d símbolos", n, len(syms))
		}
		var b strings.Builder
		b.WriteString(`{"quoteResponse":{"result":[`)
		for i, s := range syms {
			if i > 0 {
				b.WriteString(",")
			}
			fmt.Fprintf(&b, `{"symbol":%q,"regularMarketPrice":1}`, s)
		}
		b.WriteString(`],"error":null}}`)
		_, _ = io.WriteString(w, b.String())
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	syms := make([]string, 2*QuoteBatchSize+5)
	for i := range syms {
		syms[i] = fmt.Sprintf("S%d", i)
	}
	qs, err := c.Quotes(context.Background(), syms)
	if err == nil || !strings.Contains(err.Error(), "Missing required query parameter") {
		t.Errorf("err = %v", err)
	}
	if len(qs) != 2*QuoteBatchSize || calls.Load() != 3 {
		t.Errorf("cotizaciones parciales = %d, llamadas = %d", len(qs), calls.Load())
	}
}

func TestQuotesEmptyInputAndUnknownSymbols(t *testing.T) {
	c := &Client{crumb: "x"} // sin red
	if qs, err := c.Quotes(context.Background(), []string{"", " "}); err != nil || qs != nil {
		t.Errorf("sin símbolos: %v %v", qs, err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(readFixture(t, "quote_empty.json"))
	}))
	defer srv.Close()
	qs, err := newTestClient(t, srv).Quotes(context.Background(), []string{"NOEXISTE1"})
	if err != nil || len(qs) != 0 {
		t.Errorf("desconocidos: %v %v", qs, err)
	}
}

func TestQuotesBodyRateLimitRetriedWithRateLimitPolicy(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			_, _ = io.WriteString(w, `{"finance":{"result":null,"error":{"code":"Too Many Requests","description":"slow down"}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"quoteResponse":{"result":[{"symbol":"AAPL"}],"error":null}}`)
	}))
	defer srv.Close()
	c := newTestClient(t, srv)
	delays := recordSleep(c)
	testCfg(c).retry = &RetryPolicy{RateLimit: &RateLimitPolicy{MaxRetries: 1, BaseDelay: 3 * time.Second}}

	qs, err := c.Quotes(context.Background(), []string{"AAPL"})
	if err != nil || len(qs) != 1 || calls.Load() != 2 || len(*delays) != 1 || (*delays)[0] != 3*time.Second {
		t.Errorf("qs=%v err=%v llamadas=%d esperas=%v", qs, err, calls.Load(), *delays)
	}
}

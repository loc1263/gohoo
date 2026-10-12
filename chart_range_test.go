package gohoo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	_ "time/tzdata" // zonas del exchange deterministas también en Windows
)

func serveFile(t *testing.T, name string) http.HandlerFunc {
	t.Helper()
	body, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(body)
	}
}

// --- Comportamiento por defecto (caracterización de v0.1.0) ---

func TestChartRangeSendsPeriods(t *testing.T) {
	start := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/SPY", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Has("range") {
			t.Errorf("query trae range: %v", q)
		}
		if q.Get("period1") != "1577836800" || q.Get("period2") != "1717200000" {
			t.Errorf("period1/period2 = %s/%s", q.Get("period1"), q.Get("period2"))
		}
		if q.Get("interval") != "1mo" || q.Get("events") != "div,split" {
			t.Errorf("interval/events = %s/%s", q.Get("interval"), q.Get("events"))
		}
		serveFile(t, "chart_monthly_ny.json")(w, r)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	res, err := client.ChartRange(context.Background(), "SPY", ChartParams{Start: start, End: end, Interval: "1mo", Events: "div,split"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Meta.Symbol != "SPY" || res.Meta.Currency != "USD" || res.Meta.InstrumentType != "ETF" {
		t.Errorf("Meta = %+v", res.Meta)
	}
	if res.Meta.Location.String() != "America/New_York" {
		t.Errorf("Location = %v, want America/New_York", res.Meta.Location)
	}
	// 6 barras en el fixture: febrero tiene adjclose null (close 480) y mayo
	// tiene ambos null; las dos se descartan.
	if len(res.Bars) != 4 {
		t.Fatalf("len(Bars) = %d, want 4", len(res.Bars))
	}
	for _, b := range res.Bars {
		if b.Time.Month() == time.February || b.Close == 480 {
			t.Errorf("barra con adjclose null no descartada: %+v", b)
		}
	}
	if got := res.Bars[0].Time; got.Location().String() != "America/New_York" || got.Day() != 1 || got.Hour() != 0 {
		t.Errorf("Bars[0].Time = %v, want 1 de enero 00:00 en Nueva York", got)
	}
	if res.Dividends["2024-03-01"] != 1.5 {
		t.Errorf("Dividends = %v", res.Dividends)
	}
}

func TestChartRangeUsesRange(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/AAPL", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("range") != "5y" || q.Has("period1") || q.Get("interval") != "1d" || q.Get("events") != "div,splits" {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write([]byte(chartFixture))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	if _, err := client.ChartRange(context.Background(), "AAPL", ChartParams{Range: "5y"}); err != nil {
		t.Fatal(err)
	}
}

func TestChartRangeValidationBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer srv.Close()

	client := newTestClient(t, srv)
	t0 := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	bad := []ChartParams{
		{Range: "1y", Start: t0},
		{End: t0},
		{},
		{Start: t0, End: t0},
	}
	for _, p := range bad {
		if _, err := client.ChartRange(context.Background(), "AAPL", p); err == nil {
			t.Errorf("ChartRange(%+v): want error", p)
		}
	}
	if calls.Load() != 0 {
		t.Errorf("la validación hizo %d requests, want 0", calls.Load())
	}
}

func TestChartRangeAdjCloseCriterion(t *testing.T) {
	cases := []struct {
		name string
		body string
		want []float64 // AdjClose de las barras resultantes
	}{
		{"adjclose null con close: se descarta", `{"chart":{"result":[{"meta":{},"timestamp":[1633060800,1635739200],
			"indicators":{"quote":[{"close":[1,3]}],"adjclose":[{"adjclose":[1.5,null]}]}}],"error":null}}`, []float64{1.5}},
		{"sin serie adjclose: usa close", `{"chart":{"result":[{"meta":{},"timestamp":[1633060800,1635739200],
			"indicators":{"quote":[{"close":[10.5,11.5]}]}}],"error":null}}`, []float64{10.5, 11.5}},
		{"adjclose vacío: usa close y descarta close null", `{"chart":{"result":[{"meta":{},"timestamp":[1633060800,1635739200],
			"indicators":{"quote":[{"close":[null,11.5]}],"adjclose":[{"adjclose":[]}]}}],"error":null}}`, []float64{11.5}},
	}
	for _, c := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(c.body))
		}))
		res, err := newTestClient(t, srv).ChartRange(context.Background(), "X", ChartParams{Range: "1y"})
		srv.Close()
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		var got []float64
		for _, b := range res.Bars {
			got = append(got, b.AdjClose)
		}
		if len(got) != len(c.want) {
			t.Errorf("%s: AdjClose = %v, want %v", c.name, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("%s: AdjClose = %v, want %v", c.name, got, c.want)
				break
			}
		}
	}
}

func TestChartRangeNullFieldsAreNaN(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"symbol":"X"},"timestamp":[1700000000,1700086400],
			"indicators":{"quote":[{"open":[null,1],"high":[null,1],"low":[null,1],"close":[null,1],"volume":[null,1]}],
			"adjclose":[{"adjclose":[10,1]}]}}],"error":null}}`))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	res, err := client.ChartRange(context.Background(), "X", ChartParams{Range: "1mo"})
	if err != nil {
		t.Fatal(err)
	}
	b := res.Bars[0]
	if b.AdjClose != 10 || !math.IsNaN(b.Open) || !math.IsNaN(b.Close) || !math.IsNaN(b.Volume) {
		t.Errorf("Bars[0] = %+v, want AdjClose 10 y el resto NaN", b)
	}
	if res.Meta.Location != time.UTC {
		t.Errorf("sin exchangeTimezoneName Location = %v, want UTC", res.Meta.Location)
	}
}

func TestChartRangeErrorsInBody(t *testing.T) {
	cases := []struct {
		file   string
		status int
		want   error
	}{
		{"chart_not_found.json", http.StatusNotFound, ErrNotFound},
		{"chart_not_found.json", http.StatusOK, ErrNotFound},
		{"chart_too_many.json", http.StatusOK, ErrRateLimit},
	}
	for _, c := range cases {
		body, err := os.ReadFile("testdata/" + c.file)
		if err != nil {
			t.Fatal(err)
		}
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(c.status)
			_, _ = w.Write(body)
		}))
		client := newTestClient(t, srv)
		_, err = client.ChartRange(context.Background(), "ZZZZ", ChartParams{Range: "1y"})
		if !errors.Is(err, c.want) {
			t.Errorf("%s con HTTP %d: error = %v, want %v", c.file, c.status, err, c.want)
		}
		srv.Close()
	}
}

func TestChartRangeAndChartSeriesShareEvents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(chartFixture))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	_, divs, splits, err := client.ChartSeries("AAPL", "1y", "1d")
	if err != nil {
		t.Fatal(err)
	}
	res, err := client.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"})
	if err != nil {
		t.Fatal(err)
	}
	if len(divs) != len(res.Dividends) || len(splits) != len(res.Splits) {
		t.Fatalf("eventos distintos: %v/%v vs %v/%v", divs, splits, res.Dividends, res.Splits)
	}
	for k, v := range divs {
		if res.Dividends[k] != v {
			t.Errorf("dividendo %s: %v vs %v", k, v, res.Dividends[k])
		}
	}
	for k, v := range splits {
		if res.Splits[k] != v {
			t.Errorf("split %s: %v vs %v", k, v, res.Splits[k])
		}
	}
}

// --- Monthly ---

func TestConcurrentClientUse(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) <= 8 { // a lo sumo 8 fallos en total: ningún pedido agota 10 intentos
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write([]byte(chartFixture))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	p := DefaultRetryPolicy()
	p.MaxAttempts = 10
	p.Jitter = 0.3
	testCfg(client).retry = &p
	recordSleep(client)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 5 {
				if _, err := client.ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestChartRangeUnparseableBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("no es json"))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"})
	if err == nil || !strings.Contains(err.Error(), "no se pudo parsear chart de AAPL") {
		t.Errorf("err = %v, want error de parseo", err)
	}
}

func TestChartRangeEmptyResultIsNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"chart":{"result":[],"error":null}}`))
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).ChartRange(context.Background(), "AAPL", ChartParams{Range: "1y"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestChartRangeSkipsNullTimestampAndUnknownZone(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"chart":{"result":[{
			"meta":{"symbol":"X","exchangeTimezoneName":"Zona/Inexistente"},
			"timestamp":[null,1700000000],
			"indicators":{"quote":[{"close":[1,2]}]}}]}}`))
	}))
	defer srv.Close()

	res, err := newTestClient(t, srv).ChartRange(context.Background(), "X", ChartParams{Range: "1y"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Bars) != 1 || res.Bars[0].AdjClose != 2 {
		t.Errorf("Bars = %+v, want una barra con AdjClose 2", res.Bars)
	}
	if res.Meta.Location != time.UTC {
		t.Errorf("Location = %v, want UTC para una zona desconocida", res.Meta.Location)
	}
}

func TestChartParamsStartWithoutEndUsesNow(t *testing.T) {
	now := time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC)
	q, err := ChartParams{Start: now.AddDate(0, -1, 0)}.query(now)
	if err != nil {
		t.Fatal(err)
	}
	vals, _ := url.ParseQuery(q)
	if vals.Get("period2") != fmt.Sprint(now.Unix()) {
		t.Errorf("period2 = %s, want %d", vals.Get("period2"), now.Unix())
	}
}

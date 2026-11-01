package gohoo

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDailyBarsAdjustedAtMidnight(t *testing.T) {
	start := time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC)
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/SPY", func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("interval"); got != "1d" {
			t.Errorf("interval = %q, want 1d", got)
		}
		w.Header().Set("Content-Type", "application/json")
		// 14:30 y 14:30 UTC = 09:30 en Nueva York; el segundo cierre ajustado es la mitad del cierre.
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"currency":"USD","symbol":"SPY","exchangeTimezoneName":"America/New_York"},
			"timestamp":[1704209400,1704295800],
			"indicators":{"quote":[{"open":[10,20],"high":[10,20],"low":[10,20],"close":[10,20],"volume":[1,2]}],
			"adjclose":[{"adjclose":[10,10]}]}}],"error":null}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	bars, err := newTestClient(t, srv).DailyBars(context.Background(), "SPY", start, end)
	if err != nil {
		t.Fatalf("DailyBars error: %v", err)
	}
	if len(bars) != 2 {
		t.Fatalf("len(bars) = %d, want 2", len(bars))
	}
	for i, b := range bars {
		if h, m, s := b.Time.Clock(); h != 0 || m != 0 || s != 0 {
			t.Errorf("bars[%d].Time = %v, want midnight", i, b.Time)
		}
		if b.Close != 10 {
			t.Errorf("bars[%d].Close = %v, want 10 (ajustado)", i, b.Close)
		}
	}
}

func TestDailyBarsWithoutBarsIsNotFound(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/SPY", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"chart":{"result":[{"meta":{"symbol":"SPY","exchangeTimezoneName":"America/New_York"},
			"timestamp":[],"indicators":{"quote":[{"open":[],"high":[],"low":[],"close":[],"volume":[]}]}}],"error":null}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	_, err := newTestClient(t, srv).DailyBars(context.Background(), "SPY",
		time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC))
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("err = %v, want errors.Is(ErrNotFound)", err)
	}
}

func TestDailyBarsPropagatesChartRangeError(t *testing.T) {
	// End anterior a Start: ChartRange falla antes de tocar la red.
	_, err := (&Client{}).DailyBars(context.Background(), "SPY",
		time.Date(2024, 1, 5, 0, 0, 0, 0, time.UTC), time.Date(2024, 1, 2, 0, 0, 0, 0, time.UTC))
	if err == nil {
		t.Fatal("expected error for End before Start")
	}
}

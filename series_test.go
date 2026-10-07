package yahoo

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestFirstResult(t *testing.T) {
	chart := map[string]interface{}{
		"chart": map[string]interface{}{
			"result": []interface{}{
				map[string]interface{}{"timestamp": []interface{}{1.0}},
			},
		},
	}
	r, ok := FirstResult(chart)
	if !ok {
		t.Fatal("FirstResult ok = false, want true")
	}
	if _, has := r["timestamp"]; !has {
		t.Error("FirstResult did not return expected result map")
	}

	if _, ok := FirstResult(map[string]interface{}{}); ok {
		t.Error("FirstResult on empty map ok = true, want false")
	}
	empty := map[string]interface{}{"chart": map[string]interface{}{"result": []interface{}{}}}
	if _, ok := FirstResult(empty); ok {
		t.Error("FirstResult with empty result slice ok = true, want false")
	}
}

const chartFixture = `{
  "chart": {
    "result": [
      {
        "timestamp": [1700000000, 1700086400],
        "indicators": {
          "quote": [
            {
              "open":   [100.0, 101.0],
              "high":   [102.0, 103.0],
              "low":    [99.0, 100.0],
              "close":  [101.0, 102.0],
              "volume": [1000000, 1100000]
            }
          ],
          "adjclose": [
            {"adjclose": [100.5, 101.5]}
          ]
        },
        "events": {
          "dividends": {
            "1700000000": {"amount": 0.24, "date": 1700000000}
          },
          "splits": {
            "1700086400": {"date": 1700086400, "numerator": 4, "denominator": 1}
          }
        }
      }
    ],
    "error": null
  }
}`

func TestChartSeries(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/AAPL", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(chartFixture))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	series, dividends, splits, err := client.ChartSeries("AAPL", "1y", "1d")
	if err != nil {
		t.Fatalf("ChartSeries error: %v", err)
	}

	if len(series) != 2 {
		t.Fatalf("len(series) = %d, want 2", len(series))
	}
	wantTime := time.Unix(1700000000, 0).UTC()
	if !series[0].Time.Equal(wantTime) {
		t.Errorf("series[0].Time = %v, want %v", series[0].Time, wantTime)
	}
	if open, _ := AsFloat(series[0].Open); open != 100.0 {
		t.Errorf("series[0].Open = %v, want 100.0", series[0].Open)
	}
	if close_, _ := AsFloat(series[1].Close); close_ != 102.0 {
		t.Errorf("series[1].Close = %v, want 102.0", series[1].Close)
	}
	if adj, _ := AsFloat(series[0].AdjClose); adj != 100.5 {
		t.Errorf("series[0].AdjClose = %v, want 100.5", series[0].AdjClose)
	}

	if got := dividends["2023-11-14"]; got != 0.24 {
		t.Errorf("dividends[2023-11-14] = %v, want 0.24", got)
	}
	if got := splits["2023-11-15"]; got != 4.0 {
		t.Errorf("splits[2023-11-15] = %v, want 4.0", got)
	}
}

func TestChartSeriesEmptyResult(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v8/finance/chart/FAKE", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"chart": {"result": [], "error": null}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := newTestClient(t, srv)
	if _, _, _, err := client.ChartSeries("FAKE", "1y", "1d"); err == nil {
		t.Error("ChartSeries with empty result expected error, got nil")
	}
}

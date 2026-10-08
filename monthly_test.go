package gohoo

import (
	"context"
	"net/http/httptest"
	"testing"
	"time"
)

func monthlyFromFile(t *testing.T, file string) ([]time.Time, []float64) {
	t.Helper()
	srv := httptest.NewServer(serveFile(t, file))
	defer srv.Close()
	res, err := newTestClient(t, srv).ChartRange(context.Background(), "X", ChartParams{Range: "max", Interval: "1mo"})
	if err != nil {
		t.Fatal(err)
	}
	return res.Monthly()
}

func checkMonthly(t *testing.T, dates []time.Time, adj []float64, wantDates []string, wantAdj []float64) {
	t.Helper()
	if len(dates) != len(wantDates) || len(adj) != len(wantAdj) {
		t.Fatalf("Monthly = %v %v, want %v %v", dates, adj, wantDates, wantAdj)
	}
	for i := range dates {
		if got := dates[i].Format(time.RFC3339); got != wantDates[i]+"T00:00:00Z" {
			t.Errorf("dates[%d] = %s, want %s", i, got, wantDates[i])
		}
		if adj[i] != wantAdj[i] {
			t.Errorf("adj[%d] = %v, want %v", i, adj[i], wantAdj[i])
		}
	}
}

func TestMonthlyNewYorkLiveBarWinsAcrossDST(t *testing.T) {
	dates, adj := monthlyFromFile(t, "chart_monthly_ny.json")
	// Abril tiene la barra mensual (505) y la "en vivo" del 15 (503): gana la
	// más reciente. Mayo es todo null y no aparece. El 1 de abril ya es EDT.
	checkMonthly(t, dates, adj,
		[]string{"2024-01-01", "2024-02-01", "2024-03-01", "2024-04-01"},
		[]float64{465, 480, 495, 503})
}

func TestMonthlyTokyoUsesExchangeCalendar(t *testing.T) {
	// Medianoche JST del día 1 cae el día anterior en UTC; con la zona del
	// exchange los meses son marzo y abril, no febrero y marzo.
	dates, adj := monthlyFromFile(t, "chart_monthly_tokyo.json")
	checkMonthly(t, dates, adj, []string{"2024-03-01", "2024-04-01"}, []float64{3000, 3150})
}

func TestMonthlyEmpty(t *testing.T) {
	var r *ChartResult
	if d, a := r.Monthly(); d != nil || a != nil {
		t.Error("Monthly de nil: want nil, nil")
	}
}

// --- Concurrencia ---

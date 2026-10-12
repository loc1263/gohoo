//go:build live

// Pruebas contra Yahoo Finance real: go test -tags live -run Live ./...
// No corren en CI; sirven para detectar cambios en la API de Yahoo.
package gohoo

import (
	"context"
	"math"
	"testing"
	"time"
)

func liveClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLiveScreen(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := liveClient(t)

	res, err := c.Screen(ctx, ScreenParams{
		Query: And(Eq("region", "us"), Gte("intradaymarketcap", 2e11)), Size: 5, SortField: "intradaymarketcap",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Total < 5 || len(res.Quotes) != 5 {
		t.Fatalf("res = %+v", res)
	}
	for i, q := range res.Quotes {
		if q.Symbol == "" || q.MarketCap < 2e11 || (i > 0 && q.MarketCap > res.Quotes[i-1].MarketCap) {
			t.Errorf("quote %d inesperado: %s %.0f", i, q.Symbol, q.MarketCap)
		}
	}

	etf, err := c.Screen(ctx, ScreenParams{Query: Eq("region", "us"), QuoteType: "ETF", Size: 3, SortField: "fundnetassets"})
	if err != nil || len(etf.Quotes) != 3 || etf.Quotes[0].QuoteType != "ETF" {
		t.Fatalf("ETF: %+v, %v", etf, err)
	}

	empty, err := c.Screen(ctx, ScreenParams{Query: Eq("region", "zz")})
	if err != nil || empty.Total != 0 {
		t.Fatalf("región inexistente: %+v, %v", empty, err)
	}

	pre, err := c.ScreenPredefined(ctx, "most_actives", 0, 5)
	if err != nil || len(pre.Quotes) == 0 {
		t.Fatalf("predefinido: %+v, %v", pre, err)
	}
	t.Logf("mega-cap EEUU: %d; primero %s; most_actives: %s", res.Total, res.Quotes[0].Symbol, pre.Quotes[0].Symbol)
}

func TestLiveQuotesAndHooks(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	var events []RequestEvent
	c, err := NewClientWithOptions(WithLazyBootstrap(), WithRequestHook(func(ev RequestEvent) { events = append(events, ev) }))
	if err != nil {
		t.Fatal(err)
	}
	qs, err := c.Quotes(ctx, []string{"AAPL", "SQM-B.SN", "CLP=X", "^VIX", "NOEXISTE123"})
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]Quote{}
	for _, q := range qs {
		by[q.Symbol] = q
	}
	if len(qs) != 4 || by["AAPL"].MarketCap <= 0 || by["SQM-B.SN"].Currency != "CLP" || by["CLP=X"].Price <= 0 || by["^VIX"].Price <= 0 {
		t.Fatalf("quotes = %+v", qs)
	}
	last := events[len(events)-1]
	if last.Endpoint != "quote" || last.StatusCode != 200 {
		t.Errorf("último evento = %+v", last)
	}
	t.Logf("AAPL %.2f %s, CLP=X %.2f, %d requests (con bootstrap)", by["AAPL"].Price, by["AAPL"].Time.Format(time.RFC3339), by["CLP=X"].Price, len(events))
}

func TestLiveInfoTimeseriesAndAdjust(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c := liveClient(t)
	info, err := c.Info(ctx, "AAPL", InfoPercentYield(), InfoWithTrailingPEG())
	if err != nil {
		t.Fatal(err)
	}
	if info["currency"] != "USD" || info["sector"] == nil || info["trailingPegRatio"] == nil {
		t.Fatalf("info: currency=%v sector=%v peg=%v", info["currency"], info["sector"], info["trailingPegRatio"])
	}
	data, err := c.FundamentalsTimeseriesContext(ctx, "AAPL", []string{"quarterlyTotalRevenue"})
	if err != nil || len(TimeseriesPoints(data, "quarterlyTotalRevenue")) < 4 {
		t.Fatal("timeseries", err)
	}
	res, err := c.ChartRange(ctx, "AAPL", ChartParams{Range: "1y", Interval: "1wk"})
	if err != nil {
		t.Fatal(err)
	}
	adj := AtMidnight(res.AutoAdjusted())
	last := adj[len(adj)-1]
	if last.Time.Hour() != 0 || last.Close != res.Bars[len(res.Bars)-1].AdjClose {
		t.Fatalf("ajuste: %+v", last)
	}
	t.Logf("AAPL divY %.2f%% PEG %v; última semana %s", info["dividendYield"], info["trailingPegRatio"], last.Time.Format("2006-01-02 MST"))
}

func TestLiveIndicatorsAndQuant(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	c, err := NewClientWithOptions(WithLazyBootstrap())
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.ChartRange(ctx, "SPY", ChartParams{Range: "1y", Interval: "1d"})
	if err != nil {
		t.Fatal(err)
	}
	bars := res.AutoAdjusted()
	set := ComputeIndicatorsBars(bars)
	last := set[len(set)-1]
	if math.IsNaN(last.RSI14) || last.RSI14 < 0 || last.RSI14 > 100 || math.IsNaN(last.ADX14) || math.IsNaN(last.MACD) {
		t.Fatalf("indicadores: %+v", last)
	}
	closes := Closes(bars)
	vol := Volatility(Returns(closes), 252)
	dd, _, _ := MaxDrawdown(closes)
	if vol <= 0 || vol > 1 || dd > 0 || dd < -0.6 {
		t.Fatalf("vol=%v dd=%v", vol, dd)
	}
	t.Logf("SPY: RSI14 %.1f, ADX14 %.1f, vol anual %.1f%%, max drawdown %.1f%%", last.RSI14, last.ADX14, vol*100, dd*100)
}

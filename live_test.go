//go:build live

// Pruebas contra Yahoo Finance real: go test -tags live -run Live ./...
// No corren en CI; sirven para detectar cambios en la API de Yahoo.
package gohoo

import (
	"context"
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

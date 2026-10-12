package gohoo_test

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"time"
	_ "time/tzdata" // zonas horarias del exchange también en Windows

	"github.com/loc1263/gohoo"
)

// Fetch the current price of a ticker. Requires network access, so it is
// compiled but not run as a test.
func ExampleClient_QuoteSummaryContext() {
	client, err := gohoo.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	data, err := client.QuoteSummaryContext(context.Background(), "AAPL", []string{"price"})
	if err != nil {
		log.Fatal(err)
	}
	price, _ := data["price"].(map[string]any)
	fmt.Println(gohoo.Field(price, "regularMarketPrice"))
}

// Download one year of daily candles plus dividends and splits.
func ExampleClient_ChartSeries() {
	client, err := gohoo.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	series, dividends, splits, err := client.ChartSeries("AAPL", "1y", "1d")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(len(series), len(dividends), len(splits))
}

// List the largest US companies (market cap >= 200 billion USD) with
// Yahoo's screener. Requires network access, so it is compiled but not run.
func ExampleClient_Screen() {
	client, err := gohoo.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	res, err := client.Screen(context.Background(), gohoo.ScreenParams{
		Query:     gohoo.And(gohoo.Eq("region", "us"), gohoo.Gte("intradaymarketcap", 200e9)),
		Size:      50,
		SortField: "intradaymarketcap",
	})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(res.Total)
	for _, q := range res.Quotes {
		fmt.Println(q.Symbol, q.MarketCap, q.Raw["trailingPE"])
	}
}

// Technical indicators with custom periods and basic risk metrics on
// adjusted daily bars. Requires network access, so it is compiled but not run.
func ExampleComputeIndicatorsBars() {
	client, err := gohoo.NewClientWithOptions(gohoo.WithLazyBootstrap())
	if err != nil {
		log.Fatal(err)
	}
	res, err := client.ChartRange(context.Background(), "SPY", gohoo.ChartParams{Range: "2y", Interval: "1d"})
	if err != nil {
		log.Fatal(err)
	}
	bars := res.AutoAdjusted()
	std := gohoo.ComputeIndicatorsBars(bars) // SMA20, RSI14, MACD(12,26,9), ...
	closes := gohoo.Closes(bars)
	golden := gohoo.Crossovers(gohoo.SMA(closes, 50), gohoo.SMA(closes, 200))
	dd, _, _ := gohoo.MaxDrawdown(closes)
	fmt.Println(std[len(std)-1].RSI14, golden[len(golden)-1], gohoo.Volatility(gohoo.Returns(closes), 252), dd)
}

func ExampleField() {
	summary := map[string]any{
		"trailingPE": map[string]any{"raw": 28.5, "fmt": "28.50"},
	}
	fmt.Println(gohoo.Field(summary, "trailingPE"))
	// Output: 28.5
}

func ExampleGetPath() {
	data := map[string]any{
		"summaryDetail": map[string]any{
			"trailingPE": map[string]any{"raw": 28.5},
		},
	}
	fmt.Println(gohoo.RawValue(gohoo.GetPath(data, "summaryDetail", "trailingPE")))
	// Output: 28.5
}

func ExampleEpochToDate() {
	fmt.Println(gohoo.EpochToDate(float64(1700000000)))
	// Output: 2023-11-14
}

func ExampleComputeIndicators() {
	start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	var series []gohoo.OHLCV
	for i := 0; i < 30; i++ {
		p := 100 + float64(i)
		series = append(series, gohoo.OHLCV{
			Time: start.AddDate(0, 0, i),
			Open: p, High: p + 1, Low: p - 1, Close: p, AdjClose: p, Volume: float64(1000),
		})
	}
	ind := gohoo.ComputeIndicators(series)
	fmt.Printf("%.2f\n", ind[len(ind)-1].SMA20)
	// Output: 119.50
}

func ExampleDetectGaps() {
	day := func(d int) gohoo.OHLCV {
		return gohoo.OHLCV{Time: time.Date(2024, 1, d, 0, 0, 0, 0, time.UTC), Close: 1.0}
	}
	series := []gohoo.OHLCV{day(1), day(2), day(3), day(4), day(5), day(25)}
	for _, g := range gohoo.DetectGaps(series, 5) {
		fmt.Println(g.From.Format("2006-01-02"), "->", g.To.Format("2006-01-02"))
	}
	// Output: 2024-01-05 -> 2024-01-25
}

// Download monthly adjusted closes for a date range, one observation per
// calendar month of the exchange. A local server stands in for Yahoo so the
// example runs without network; against Yahoo just drop WithBaseURL.
func ExampleChartResult_Monthly() {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Monthly bars for Feb and Mar 2024 plus the "live" bar of March 15.
		_, _ = w.Write([]byte(`{"chart":{"result":[{
			"meta":{"symbol":"SPY","currency":"USD","exchangeTimezoneName":"America/New_York"},
			"timestamp":[1706763600,1709269200,1710532800],
			"indicators":{"quote":[{"close":[508.1,512.4,509.8]}],"adjclose":[{"adjclose":[500.0,505.0,503.2]}]}
		}],"error":null}}`))
	}))
	defer srv.Close()

	client, err := gohoo.NewClientWithOptions(
		gohoo.WithBaseURL(srv.URL),
		gohoo.WithLazyBootstrap(), // chart doesn't need cookie+crumb
	)
	if err != nil {
		log.Fatal(err)
	}
	res, err := client.ChartRange(context.Background(), "SPY", gohoo.ChartParams{
		Start:    time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
		End:      time.Date(2024, 3, 31, 0, 0, 0, 0, time.UTC),
		Interval: "1mo",
	})
	if err != nil {
		log.Fatal(err)
	}
	dates, adj := res.Monthly()
	for i := range dates {
		fmt.Println(dates[i].Format("2006-01-02"), adj[i])
	}
	// Output:
	// 2024-02-01 500
	// 2024-03-01 503.2
}

// Leave rate-limit handling to the caller: no retries on HTTP 429, and the
// error is recognizable with errors.Is.
func ExampleWithRetryPolicy() {
	client, err := gohoo.NewClientWithOptions(
		gohoo.WithLazyBootstrap(),
		gohoo.WithRetryPolicy(gohoo.RetryPolicy{MaxAttempts: 1, RetryRateLimit: false}),
	)
	if err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	_, err = client.ChartRange(ctx, "AAPL", gohoo.ChartParams{Range: "5y", Interval: "1mo"})
	var se *gohoo.StatusError
	switch {
	case gohoo.IsRateLimit(err):
		if errors.As(err, &se) && se.RetryAfter > 0 {
			fmt.Println("rate limited, retry after", se.RetryAfter)
		}
	case errors.Is(err, gohoo.ErrNotFound):
		fmt.Println("no data")
	case err != nil:
		log.Fatal(err)
	}
}

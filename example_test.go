package gohoo_test

import (
	"fmt"
	"log"
	"time"

	"github.com/loc1263/gohoo"
)

// Fetch the current price of a ticker. Requires network access, so it is
// compiled but not run as a test.
func ExampleClient_QuoteSummary() {
	client, err := gohoo.NewClient()
	if err != nil {
		log.Fatal(err)
	}
	data, err := client.QuoteSummary("AAPL", []string{"price"})
	if err != nil {
		log.Fatal(err)
	}
	price, _ := data["price"].(map[string]interface{})
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

func ExampleField() {
	summary := map[string]interface{}{
		"trailingPE": map[string]interface{}{"raw": 28.5, "fmt": "28.50"},
	}
	fmt.Println(gohoo.Field(summary, "trailingPE"))
	// Output: 28.5
}

func ExampleGetPath() {
	data := map[string]interface{}{
		"summaryDetail": map[string]interface{}{
			"trailingPE": map[string]interface{}{"raw": 28.5},
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

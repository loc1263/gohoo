# gohoo

English | [Español](README.es.md)

A minimalist Go client for Yahoo Finance's unofficial endpoints (`quoteSummary`, `fundamentals-timeseries` and `chart`). Think of it as what [`yfinance`](https://github.com/ranaroussi/yfinance) is for Python, but for Go: no API key, talking directly to `query2.finance.yahoo.com` through `net/http`.

Extracted from [`go-finz-analysis`](https://github.com/loc1263/go-finz-analysis), where it started as an internal package shared by several binaries (stock indicators, stock/ETF/crypto prices). This is the reusable version, with no external dependencies.

Documentation (currently in Spanish): [API.md](API.md) (function/type reference) · [ENDPOINTS.md](ENDPOINTS.md) (what data each Yahoo endpoint returns).

> **Disclaimer:** gohoo is not affiliated with or endorsed by Yahoo. It uses **unofficial, undocumented** endpoints that may change or stop working without notice, and their use is subject to Yahoo's terms of service. Intended for personal and educational projects; the data is not financial advice and should be verified before being used to trade. Provided "as is" (see [LICENSE](LICENSE)).

## Installation

```bash
go get github.com/loc1263/gohoo
```

## Usage

```go
package main

import (
	"fmt"
	"log"

	"github.com/loc1263/gohoo"
)

func main() {
	client, err := gohoo.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	// Raw quoteSummary modules and indicators.
	data, err := client.QuoteSummary("AAPL", []string{"price", "summaryDetail"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Current price:", gohoo.Field(data["price"].(map[string]interface{}), "regularMarketPrice"))

	// Parsed OHLCV history (stocks, ETFs or cryptocurrencies).
	series, dividends, splits, err := client.ChartSeries("AAPL", "1y", "1d")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d candles, %d dividend dates, %d split dates\n", len(series), len(dividends), len(splits))
}
```

## API

- `NewClient() (*Client, error)` — creates an authenticated session (Yahoo cookie + crumb).
- `(*Client) QuoteSummary(ticker string, modules []string) (map[string]interface{}, error)` — `quoteSummary` modules (price, statistics, analyst estimates, etc.), merged into a single map.
- `(*Client) FundamentalsTimeseries(ticker string, types []string) (map[string]interface{}, error)` — annual/quarterly financial statements via `fundamentals-timeseries`.
- `(*Client) Chart(ticker, rang, interval string) (map[string]interface{}, error)` — raw response from the `chart` endpoint (OHLCV + dividends/splits).
- `(*Client) ChartSeries(ticker, rang, interval string) ([]OHLCV, dividends, splits map[string]float64, error)` — `Chart` already parsed into a sorted series, plus dividends/splits keyed by date (`"YYYY-MM-DD"`).
- `ComputeIndicators(series []OHLCV) []IndicatorSet` — SMA, EMA, RSI, MACD, Bollinger Bands, ATR, Stochastic, Williams %R, ADX, VWAP, OBV and derived signals.
- `DetectGaps(series []OHLCV, factor float64) []Gap` — detects unusual gaps in a series.
- Yahoo JSON parsing helpers: `RawValue`, `GetPath`, `Field`, `AsFloat`, `AsInt64`, `AsString`, `EpochToDate`, `EpochToDateTimeNY`, `EpochToMidnight`, `DateStringToMidnight`, `ParseDateOnly`.

Every number Yahoo returns in `quoteSummary`/`fundamentals-timeseries` arrives as `{"raw": x, "fmt": "formatted text"}`; `RawValue`/`Field` always extract `raw`.

## Notes

- `Client` is safe for concurrent use. It retries only on transient failures (network errors, 429, 5xx) with exponential backoff; on a 401 it refreshes the session.
- Don't fire bursts of requests: Yahoo rate-limits by IP.
- No API key or user authentication required: just session cookies plus a public "crumb" token, fetched automatically in `NewClient`.
- `rang` and `interval` take the same values as Yahoo Finance's UI/API: `1d, 5d, 1mo, 3mo, 6mo, 1y, 2y, 5y, 10y, ytd, max` for ranges and `1m, 2m, 5m, 15m, 30m, 60m/1h, 1d, 5d, 1wk, 1mo, 3mo` for intervals. Yahoo limits how much history it returns for intraday intervals.
- Works the same for stocks (`AAPL`), ETFs (`SPY`) and cryptocurrencies (`BTC-USD`) — they all share Yahoo Finance's `chart` endpoint.

## Tests

```bash
go test ./...
```

Tests need no network access: they simulate Yahoo's responses with a local `httptest.Server`.

## License

[MIT](LICENSE)

# gohoo

English | [Español](README.es.md)

A minimalist Go client for Yahoo Finance's unofficial endpoints. Think of it as what [`yfinance`](https://github.com/ranaroussi/yfinance) is for Python, but for Go: no API key, no dependencies outside the standard library.

> **Disclaimer:** gohoo is not affiliated with or endorsed by Yahoo. It uses **unofficial, undocumented** endpoints that may change or stop working without notice, and their use is subject to Yahoo's terms of service. Intended for personal and educational projects; the data is not financial advice and should be verified before being used to trade. Provided "as is" (see [LICENSE](LICENSE)).

## Installation

```bash
go get github.com/loc1263/gohoo
```

Requires Go 1.22 or later.

## Quick start

```go
package main

import (
	"context"
	"fmt"
	"log"

	"github.com/loc1263/gohoo"
)

func main() {
	ctx := context.Background()
	client, err := gohoo.NewClientWithOptions(gohoo.WithLazyBootstrap())
	if err != nil {
		log.Fatal(err)
	}

	// A year of daily candles, adjusted for dividends and splits, and an indicator.
	res, err := client.ChartRange(ctx, "AAPL", gohoo.ChartParams{Range: "1y", Interval: "1d"})
	if err != nil {
		log.Fatal(err)
	}
	bars := res.AutoAdjusted()
	rsi := gohoo.RSI(gohoo.Closes(bars), 14)
	fmt.Printf("%d candles, last RSI14 %.1f\n", len(bars), rsi[len(rsi)-1])

	// Current quotes for several symbols.
	quotes, err := client.Quotes(ctx, []string{"AAPL", "MSFT", "BTC-USD"})
	if err != nil {
		log.Fatal(err)
	}
	for _, q := range quotes {
		fmt.Println(q.Symbol, q.Price, q.Currency)
	}
}
```

## What it does

| I want… | Use |
|---|---|
| Price history (OHLCV) | `ChartRange`, then `AutoAdjusted()` for adjusted prices or `Monthly()` for a monthly series |
| Current prices for many symbols | `Quotes` |
| Company data (`Ticker.info` style) | `Info` |
| Financial statements | `FundamentalsTimeseriesContext` + `TimeseriesPoints` |
| Find instruments by criteria | `Screen`, `ScreenPredefined` |
| Technical indicators | `RSI`, `MACD`, `SMA`, `EMA`, `Bollinger`, `ATR`, `ADX`, `MFI`... or `ComputeIndicatorsBars` for the whole set |
| Risk metrics | `Returns`, `Volatility`, `MaxDrawdown` |
| Any other Yahoo endpoint | `GetJSON`, `PostJSON` |

Stocks (`AAPL`), ETFs (`SPY`), indices (`^VIX`), currencies (`CLP=X`) and crypto (`BTC-USD`) all work the same way.

Errors can be told apart with `errors.Is`: `gohoo.ErrNotFound` (unknown or delisted symbol) and `gohoo.ErrRateLimit` (Yahoo is throttling you).

## Documentation

Currently in Spanish.

- [API.md](API.md): reference for every function and type.
- [ENDPOINTS.md](ENDPOINTS.md): what data each Yahoo endpoint returns.
- [TECHNICAL.md](TECHNICAL.md): how it works (session, retries, rate limits, errors, indicators), compatibility rules, tests and how to publish a release.
- [CHANGELOG.md](CHANGELOG.md): what changed in each version.

## Tests

```bash
go test ./...
```

No network needed: tests use a local fake Yahoo server. See [TECHNICAL.md](TECHNICAL.md) for the tests against the real Yahoo and for the CI.

## License

[MIT](LICENSE)

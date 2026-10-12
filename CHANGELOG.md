# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [0.7.1] - 2026-10-12

No exported signature or behavior changed.

### Changed
- Tests: error paths are now covered (session bootstrap, re-authentication after a 401, invalid JSON, empty chart results, quote/screener error bodies, retry backoff overflow, indicator edge cases) and `NewClient` has its own tests. Coverage goes from 96.2% to 99.9%.
- `.gitattributes` forces LF line endings, so `gofmt` stays clean on Windows checkouts.
- Documentation: deprecated methods are marked as such in the README; `WithLazyBootstrap` lists every method that triggers the session bootstrap; API.md documents the `Query` methods and the `IndicatorSet` fields; ENDPOINTS.md covers all five endpoints and points to the current methods (`ChartRange`, `Info`, `...Context`); fixed doc comments for `Gap` and `EpochToDateTimeNY`.

## [0.7.0] - 2026-10-12

No exported signature or behavior changed (checked with `apidiff` against v0.2.1 and v0.6.0).

### Deprecated
- `Client.QuoteSummary` and `Client.FundamentalsTimeseries`: use their `...Context` variants.
- `Client.Chart`, `Client.ChartContext` and `FirstResult`: use `ChartRange` (or `GetJSON` for the raw response).
They keep working; the `Deprecated:` comment only makes tools such as pkg.go.dev, editors and staticcheck point to the replacement.

### Changed
- The minimum Go version in `go.mod` goes down from 1.26.1 to 1.22.0, the oldest one that builds and passes the tests. Projects on older Go can now use gohoo; nothing changes for the rest.
- Error messages built by gohoo start with `gohoo:`. The retry summary (`agotados N intentos: ...`) and `StatusError` keep their format. Error text is not part of the API: classify errors with `errors.Is`/`errors.As`.
- Documentation: new package overview (`doc.go`) with a guide to which method to use, doc comments on every exported identifier, `any` instead of `interface{}` everywhere (same type), README example updated to the current API.
- CI: `golangci-lint`, `go vet -tags live`, and tests on Linux and Windows with Go 1.22 and the latest Go.

## [0.6.0] - 2026-10-12

All changes are additive: no exported signature or default behavior changed
(checked with `apidiff` against v0.2.1 and v0.5.0; `ComputeIndicators` output is byte-for-byte identical to v0.5.0).

### Added
- `ComputeIndicatorsBars`: the same indicator set as `ComputeIndicators`, computed from typed `[]Bar` (e.g. `ChartRange` results).
- Standalone indicators with configurable periods over `[]float64`: `SMA`, `EMA`, `RSI`, `MACD`, `Bollinger`, `ATR`, `Stochastic`, `WilliamsR`, `ADX`, `OBV`, `VWAP` and `Crossovers` (+1/-1 crossings). A period < 1 returns all NaN; mismatched column lengths return nil.
- Column helpers: `Opens`, `Highs`, `Lows`, `Closes`, `AdjCloses`, `Volumes`.
- Basic risk metrics: `Returns`, `LogReturns`, `Volatility` (annualized sample standard deviation) and `MaxDrawdown` (with peak and trough indices).

### Changed
- Internal only: `ComputeIndicators` delegates to a shared implementation over columns.

## [0.5.0] - 2026-10-12

All changes are additive: no exported signature or default behavior changed
(checked with `apidiff` against v0.2.1 and v0.4.0).

### Added
- `Info` and `FlattenInfo`: a flat `Ticker.info`-style map from `quoteSummary`. Modules are walked in priority order (`InfoModules()`: `summaryDetail` first, because some `defaultKeyStatistics` fields come corrupted for some stocks), the first occurrence of a key wins, numbers are unwrapped and nulls are dropped. Options: `InfoWithModules`, `InfoPercentYield` (dividend yield in percent, falling back to `yield` for ETFs) and `InfoWithTrailingPEG` (adds `trailingPegRatio` from fundamentals-timeseries). With no options values are left as Yahoo sends them.
- `TimeseriesPoints` and `Point` (date, period type, currency, value): typed observations of a fundamentals-timeseries type, newest first. `(*Client).TrailingPEG`.
- `AutoAdjust` and `(*ChartResult).AutoAdjusted()`: the equivalent of yfinance `auto_adjust` (Close = AdjClose, Open/High/Low scaled by the same factor). `Bar.Date()` and `AtMidnight` index daily/weekly bars by date.

## [0.4.0] - 2026-10-12

All changes are additive: no exported signature or default behavior changed
(checked with `apidiff` against v0.2.1 and v0.3.0).

### Added
- `Quotes`: current quotes for many symbols with `/v7/finance/quote`, in batches of `QuoteBatchSize` (100). Typed `Quote` (price, previous close, change, open/high/low, volume, market cap, currency, market state, time in the exchange's zone) plus `Raw`. Unknown symbols are skipped; on a failed batch the previous batches are returned with the error.
- `WithLimiter` (with `Limiter` and `LimiterFunc`): called before every HTTP request, retries and session bootstrap included. An error from `Wait` ends the call without retrying.
- `WithRequestHook` with `RequestEvent` (method, endpoint, symbol, URL without crumb, attempt, status, duration, error): called after every HTTP request.
- `RetryPolicy.RateLimit` (`RateLimitPolicy`): rate limits get their own retry budget, which does not consume `MaxAttempts`, and their own backoff (`min(BaseDelay·2^(n-1), MaxDelay) + U(0, Jitter)`, at least `Retry-After` if `HonorRetryAfter`). With it, rate limits reported in the body with status 200 (`chart.error` / `finance.error` "too many requests") are retried too. With `RateLimit` nil (the default) nothing changes.
- `WithRetryPolicyContext` and `RetryPolicyFromContext`: per-call retry policy that overrides the client's.

## [0.3.0] - 2026-10-12

All changes are additive: no exported signature or default behavior changed
(checked with `apidiff` against v0.2.1).

### Added
- `GetJSON` and `PostJSON`: authenticated requests (cookie + crumb, `RetryPolicy`, crumb refresh on HTTP 401, `context`) to any Yahoo Finance endpoint gohoo does not wrap yet. The current crumb is added on every attempt.
- `Screen`: Yahoo's screener (`POST /v1/finance/screener`) with a query builder (`Eq`, `Gt`, `Lt`, `Gte`, `Lte`, `Btwn`, `IsIn`, `And`, `Or`), paging (`Offset`/`Size`, up to `ScreenMaxSize` = 250) and sorting. Returns a typed `ScreenResult` (`ScreenQuote` with the common fields plus `Raw`). A valid query with no matches returns an empty page, not an error.
- `ScreenPredefined`: Yahoo's predefined screeners (`day_gainers`, `most_actives`, ...); an unknown id is `ErrNotFound`.
- `ErrEmptyScreen` for a screener response without a result block. Screener errors in the body (`finance.error`) are reported with Yahoo's code and description; "too many requests" is `ErrRateLimit`.
- Live tests against Yahoo behind the `live` build tag (`go test -tags live -run Live ./...`) and recorded screener fixtures in `testdata/`.

### Changed
- Internal only: the request/retry engine now supports any HTTP method and a JSON body; `doGet`/`doGetAuthed` delegate to it.

## [0.2.1] - 2026-10-08

### Fixed
- `ChartRange` no longer fills a bar whose `adjclose` is `null` with the unadjusted `close`, which mixed an unadjusted price into an adjusted series. That bar is now dropped. `close` is still used for the whole series when the response has no `adjclose` at all.
- `ChartResult.Monthly` breaks ties between bars with the same timestamp in favor of the later one in `Bars`, as documented ("the most recent bar wins").

## [0.2.0] - 2026-10-08

All changes are additive: no exported signature or default behavior changed
(checked with `apidiff` against v0.1.0, now part of CI).

### Added
- `NewClientWithOptions` with `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithRetryPolicy` and `WithLazyBootstrap`. `NewClient()` delegates to it with no options.
- `context.Context` variants: `QuoteSummaryContext`, `FundamentalsTimeseriesContext`, `ChartContext`, `ChartSeriesContext`. Cancellation also interrupts the retry backoff.
- Typed errors: `StatusError` (with `RetryAfter` from the `Retry-After` header), `ErrRateLimit`, `ErrNotFound`, `ErrUnauthorized` and `IsRateLimit`.
- Configurable `RetryPolicy` (`DefaultRetryPolicy()` keeps the previous behavior): max attempts, base and max delay, jitter, retry on 429, honor `Retry-After`, `OnRetry` hook.
- `ChartRange` with `ChartParams` (relative range or `period1`/`period2`), returning a typed `ChartResult` (`Bar` with `float64` fields in the exchange's time zone, `ChartMeta`, dividends and splits). Yahoo's `chart.error` body is reported as `ErrNotFound`/`ErrRateLimit`.
- `(*ChartResult).Monthly()`: one adjusted close per calendar month of the exchange.

### Changed
- Internal only: `httpStatusError` is now an alias of `StatusError`; the error text is unchanged.

## [0.1.0] - 2026-10-08

### Added
- Yahoo Finance client with cookie + crumb authentication (`NewClient`).
- `QuoteSummary`, `FundamentalsTimeseries`, `Chart` and `ChartSeries` (OHLCV, dividends, splits).
- Automatic retries with exponential backoff for network errors, HTTP 429 and 5xx; session refresh on HTTP 401.
- Technical indicators (`ComputeIndicators`) and gap detection (`DetectGaps`).
- JSON and date parsing helpers.

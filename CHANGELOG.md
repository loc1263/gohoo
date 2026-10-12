# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

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

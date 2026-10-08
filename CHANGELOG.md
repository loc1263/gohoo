# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [0.1.0] - 2026-10-08

### Added
- Yahoo Finance client with cookie + crumb authentication (`NewClient`).
- `QuoteSummary`, `FundamentalsTimeseries`, `Chart` and `ChartSeries` (OHLCV, dividends, splits).
- Automatic retries with exponential backoff for network errors, HTTP 429 and 5xx; session refresh on HTTP 401.
- Technical indicators (`ComputeIndicators`) and gap detection (`DetectGaps`).
- JSON and date parsing helpers.

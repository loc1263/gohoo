# Endpoints de Yahoo Finance

Estos son los tres endpoints no oficiales que usa `gohoo`, qué parámetros aceptan, la forma de su respuesta JSON y qué datos se pueden extraer de cada uno. No son endpoints documentados por Yahoo — todo esto es ingeniería inversa (como la mayoría de las libs tipo `yfinance`), así que la disponibilidad exacta de módulos/campos puede variar según ticker, tipo de activo (acción/ETF/cripto/índice) y puede cambiar sin aviso.

## 1. `quoteSummary` — datos "de ficha" de un ticker

```
GET https://query2.finance.yahoo.com/v10/finance/quoteSummary/{ticker}?modules={m1,m2,...}&crumb={crumb}
```

Expuesto en gohoo como [`Client.QuoteSummary`](API.md#quotesummary). Cada "módulo" pedido es un bloque de datos independiente; Yahoo los devuelve todos fusionados en `quoteSummary.result[0]` (gohoo ya hace ese merge si se pide más de un módulo de golpe). Todos los números vienen envueltos como `{"raw": x, "fmt": "texto"}` — usar `Field`/`RawValue` para desenvolverlos.

Módulos más usados:

| Módulo | Qué contiene |
|---|---|
| `price` | Snapshot de precio actual: `regularMarketPrice`, `regularMarketChange`, `regularMarketChangePercent`, `regularMarketDayHigh/Low`, `regularMarketVolume`, `marketCap`, `currency`, `exchangeName`, `marketState` (`PRE`/`REGULAR`/`POST`/`CLOSED`). |
| `summaryDetail` | Métricas de mercado agregadas: `previousClose`, `open`, `dayHigh/Low`, `fiftyTwoWeekHigh/Low`, `volume`, `averageVolume`, `trailingPE`, `forwardPE`, `dividendYield`, `dividendRate`, `exDividendDate`, `beta`. |
| `defaultKeyStatistics` | Estadísticas clave: `trailingEps`, `forwardEps`, `bookValue`, `priceToBook`, `enterpriseValue`, `profitMargins`, `sharesOutstanding`, `floatShares`, `shortRatio`, `pegRatio`, `52WeekChange`. |
| `financialData` | Salud financiera y guías de analistas: `totalCash`, `totalDebt`, `totalRevenue`, `debtToEquity`, `returnOnEquity`, `freeCashflow`, `operatingCashflow`, `revenueGrowth`, `grossMargins`, `targetMeanPrice`, `recommendationKey` (`buy`/`hold`/`sell`). |
| `earnings` | Historial y próximos earnings: `earningsChart` (EPS actual vs. estimado por trimestre), `financialsChart` (revenue/earnings anual y trimestral). |
| `earningsTrend` | Estimaciones de analistas a futuro por período (`0q`, `+1q`, `0y`, `+1y`): `epsTrend`, `epsRevisions`, `growth`. |
| `earningsHistory` | EPS real vs. estimado de los últimos trimestres reportados, con `surprisePercent`. |
| `recommendationTrend` | Conteo de analistas por rating (`strongBuy`, `buy`, `hold`, `sell`, `strongSell`) para el mes actual y los 3 anteriores. |
| `upgradeDowngradeHistory` | Historial de cambios de rating por firma (`firm`, `toGrade`, `fromGrade`, `action`, fecha). |
| `assetProfile` | Perfil corporativo: `sector`, `industry`, `fullTimeEmployees`, `longBusinessSummary`, dirección, website, y el equipo directivo (`companyOfficers`). |
| `summaryProfile` | Subconjunto de `assetProfile` sin `companyOfficers` (sector, industria, descripción, website). |
| `calendarEvents` | Próximas fechas relevantes: `earningsDate`, `exDividendDate`, `dividendDate`. |
| `balanceSheetHistory` / `balanceSheetHistoryQuarterly` | Balance general anual/trimestral: `totalAssets`, `totalLiab`, `totalStockholderEquity`, `cash`, `inventory`, `longTermDebt`, por cada período (`endDate`). |
| `incomeStatementHistory` / `incomeStatementHistoryQuarterly` | Estado de resultados anual/trimestral: `totalRevenue`, `costOfRevenue`, `grossProfit`, `operatingIncome`, `netIncome`, `ebit`. |
| `cashflowStatementHistory` / `cashflowStatementHistoryQuarterly` | Flujo de caja anual/trimestral: `totalCashFromOperatingActivities`, `capitalExpenditures`, `totalCashflowsFromInvestingActivities`, `dividendsPaid`. |
| `insiderHolders` / `insiderTransactions` | Posiciones y transacciones recientes de insiders (directores/ejecutivos): nombre, cargo, acciones, tipo de transacción, fecha. |
| `institutionOwnership` / `fundOwnership` / `majorHoldersBreakdown` | Quién posee el ticker: instituciones, fondos, % en manos de insiders vs. institucionales. |
| `netSharePurchaseActivity` | Resumen neto de compra/venta de insiders en los últimos 6/12 meses. |
| `esgScores` | Puntajes ESG (ambiental/social/gobernanza) y controversias, cuando Yahoo los tiene para el ticker. |
| `quoteType` | Metadata del instrumento: `quoteType` (`EQUITY`, `ETF`, `CRYPTOCURRENCY`, `INDEX`...), `exchange`, `longName`, `shortName`, `firstTradeDateEpochUtc`. |
| `secFilings` | Últimos filings SEC (10-K, 10-Q, 8-K...) con fecha y link, solo para emisores con reporting SEC. |

Módulos adicionales específicos de ETFs/fondos (no aplican a acciones individuales): `fundProfile`, `topHoldings` (composición/sector weightings), `fundPerformance` (retornos vs. benchmark).

Si se pide un módulo que no existe para ese ticker (p. ej. `esgScores` en una cripto) o un nombre de módulo inválido, Yahoo simplemente omite esa clave del resultado — no es un error.

## 2. `fundamentals-timeseries` — estados financieros históricos

```
GET https://query2.finance.yahoo.com/ws/fundamentals-timeseries/v1/finance/timeseries/{ticker}?symbol={ticker}&type={t1,t2,...}&period1={unix}&period2={unix}
```

Expuesto en gohoo como [`Client.FundamentalsTimeseries`](API.md#fundamentalstimeseries) (siempre pide los últimos 6 años, no configurable). A diferencia de `quoteSummary`, este endpoint entrega **series temporales** en vez de solo el último valor — útil para ver la evolución de una métrica trimestre a trimestre o año a año.

Cada `type` combina un prefijo de frecuencia con un concepto financiero:

- Prefijos: `annual` (anual), `quarterly` (trimestral), `trailing` (TTM, últimos 12 meses — solo aplica a conceptos de flujo como revenue/income/cashflow, no a conceptos de balance como assets).
- Conceptos comunes: `TotalRevenue`, `GrossProfit`, `OperatingIncome`, `NetIncome`, `DilutedEPS`, `BasicEPS`, `EBITDA`, `EBIT`, `OperatingCashFlow`, `FreeCashFlow`, `CapitalExpenditure`, `TotalAssets`, `TotalLiabilitiesNetMinorityInterest`, `StockholdersEquity`, `TotalDebt`, `CashAndCashEquivalents`, `WorkingCapital`.

Ejemplos de `type` completos: `annualTotalRevenue`, `quarterlyNetIncome`, `trailingFreeCashFlow`, `annualTotalAssets`.

La respuesta trae, por cada `type` pedido, un array de puntos `{asOfDate, periodType, reportedValue: {raw, fmt}}` — uno por período disponible dentro de la ventana `period1`/`period2`. `reportedValue` sigue el mismo patrón `{"raw": x, "fmt": "..."}` que `quoteSummary`.

## 3. `chart` — OHLCV histórico + eventos

```
GET https://query2.finance.yahoo.com/v8/finance/chart/{ticker}?range={range}&interval={interval}&events=div,splits
```

Expuesto en gohoo como [`Client.Chart`](API.md#chart) (respuesta cruda) y [`Client.ChartSeries`](API.md#chartseries) (ya parseado a `[]OHLCV` + mapas de dividendos/splits). Es el único de los tres endpoints que no requiere crumb para responder con datos completos, aunque gohoo lo envía igual por consistencia.

Forma de `chart.result[0]`:

- `meta` — metadata del instrumento y de la consulta: `currency`, `symbol`, `exchangeName`, `instrumentType` (`EQUITY`/`ETF`/`CRYPTOCURRENCY`/...), `firstTradeDate`, `regularMarketPrice`, `chartPreviousClose`, `gmtoffset`, `timezone`, `dataGranularity`, `range`, `validRanges` (los rangos válidos que Yahoo realmente soporta para ese ticker/intervalo). gohoo no expone `meta` en `ChartSeries`; hay que leerlo de `Chart` crudo con `GetPath(chart, "chart", "result")` + index 0.
- `timestamp` — array de epochs Unix (UTC), uno por vela.
- `indicators.quote[0]` — arrays paralelos a `timestamp`: `open`, `high`, `low`, `close`, `volume`. Pueden traer `null` en posiciones sin dato (p. ej. mercado cerrado ese día para ese intervalo).
- `indicators.adjclose[0].adjclose` — cierre ajustado por dividendos/splits, paralelo a `timestamp`. Puede faltar el bloque completo para algunos intervalos intradía.
- `events.dividends` — mapa `{epoch: {amount, date}}`, un entry por cada dividendo pagado dentro del rango pedido.
- `events.splits` — mapa `{epoch: {date, numerator, denominator, splitRatio}}`, un entry por cada split/reverse-split dentro del rango.

`ChartSeries` reindexa `dividends`/`splits` por fecha `"YYYY-MM-DD"` (vía `EpochToDate`) en vez de por epoch crudo, para que sean fáciles de cruzar con las fechas de `OHLCV.Time`.

Para criptomonedas e índices, `events` normalmente viene vacío (no hay dividendos/splits que reportar), por lo que los mapas resultantes quedan vacíos — no es un error ni una respuesta incompleta.

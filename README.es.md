# gohoo

[English](README.md) | Español

Cliente Go minimalista para los endpoints no oficiales de Yahoo Finance (`quoteSummary`, `fundamentals-timeseries` y `chart`). Equivalente, para Go, a lo que es [`yfinance`](https://github.com/ranaroussi/yfinance) para Python: sin API key, hablando directo con `query2.finance.yahoo.com` vía `net/http`.

Extraído de [`go-finz-analysis`](https://github.com/loc1263/go-finz-analysis), donde nació como paquete interno usado por varios binarios (indicadores de acciones, precios de acciones/ETFs/criptomonedas). Esta es la versión reutilizable, sin dependencias externas.

Documentación: [API.md](API.md) (referencia de funciones/tipos) · [ENDPOINTS.md](ENDPOINTS.md) (qué datos trae cada endpoint de Yahoo).

> **Aviso:** gohoo no está afiliado ni respaldado por Yahoo. Usa endpoints **no oficiales y no documentados** que pueden cambiar o dejar de funcionar sin aviso, y su uso está sujeto a los términos de servicio de Yahoo. Pensado para proyectos personales y educativos; los datos no deben tomarse como asesoría financiera ni usarse para operar sin verificarlos. Se entrega "tal cual" (ver [LICENSE](LICENSE)).

## Instalación

```bash
go get github.com/loc1263/gohoo
```

Requiere Go 1.22 o superior. Sin dependencias fuera de la biblioteca estándar.

## Uso

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

	// Histórico diario, ajustado por dividendos y splits, e indicadores.
	res, err := client.ChartRange(ctx, "AAPL", gohoo.ChartParams{Range: "1y", Interval: "1d"})
	if err != nil {
		log.Fatal(err)
	}
	bars := res.AutoAdjusted()
	rsi := gohoo.RSI(gohoo.Closes(bars), 14)
	fmt.Printf("%d velas, último RSI14 %.1f\n", len(bars), rsi[len(rsi)-1])

	// Datos de la empresa (estilo Ticker.info) y cotizaciones en lote.
	info, err := client.Info(ctx, "AAPL", gohoo.InfoPercentYield())
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Sector:", info["sector"], "- dividend yield %:", info["dividendYield"])

	quotes, err := client.Quotes(ctx, []string{"AAPL", "MSFT", "BTC-USD"})
	if err != nil {
		log.Fatal(err)
	}
	for _, q := range quotes {
		fmt.Println(q.Symbol, q.Price, q.Currency)
	}
}
```

## API

- `NewClient() (*Client, error)` — crea una sesión autenticada (cookie + crumb de Yahoo).
- `(*Client) QuoteSummary(ticker string, modules []string) (map[string]any, error)` — módulos de `quoteSummary` (precio, estadísticas, estimaciones de analistas, etc.), fusionados en un solo mapa. *Deprecado:* usar `QuoteSummaryContext` o `Info`.
- `(*Client) FundamentalsTimeseries(ticker string, types []string) (map[string]any, error)` — estados financieros anuales/trimestrales vía `fundamentals-timeseries`. *Deprecado:* usar `FundamentalsTimeseriesContext`.
- `(*Client) Chart(ticker, rango, intervalo string) (map[string]any, error)` — respuesta cruda del endpoint `chart` (OHLCV + dividendos/splits). *Deprecado:* usar `ChartRange` (o `GetJSON` para la respuesta cruda).
- `(*Client) ChartSeries(ticker, rango, intervalo string) ([]OHLCV, dividendos, splits map[string]float64, error)` — `Chart` ya parseado a una serie ordenada, más dividendos/splits indexados por fecha (`"YYYY-MM-DD"`).
- `NewClientWithOptions(opts ...Option) (*Client, error)` — cliente configurable: `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithRetryPolicy`, `WithLazyBootstrap` (no hace red hasta que de verdad se necesita el crumb). Sin opciones se comporta exactamente igual que `NewClient()`.
- `QuoteSummaryContext`, `FundamentalsTimeseriesContext`, `ChartContext`, `ChartSeriesContext` — iguales a los anteriores, cancelables con un `context.Context` (también durante la espera entre reintentos). `ChartContext` también está deprecado.
- `(*Client) ChartRange(ctx, ticker string, p ChartParams) (*ChartResult, error)` — `chart` por rango relativo o por fechas (`period1`/`period2`), con barras `float64` en la zona horaria del exchange y los `null` de Yahoo resueltos.
- `(*ChartResult) Monthly() (dates []time.Time, adj []float64)` — un cierre ajustado por mes calendario; dentro de un mes gana la barra más reciente (la mensual frente a la "en vivo").
- `(*Client) Screen(ctx, p ScreenParams) (*ScreenResult, error)` — screener de Yahoo con constructor de queries (`Eq`, `Gt`, `Lt`, `Gte`, `Lte`, `Btwn`, `IsIn`, `And`, `Or`), paginación y orden; `ScreenPredefined(ctx, id, offset, size)` para los screeners guardados de Yahoo (`day_gainers`, `most_actives`, ...).
- `(*Client) GetJSON` / `PostJSON(ctx, path, query[, body])` — petición autenticada (cookie + crumb, reintentos, renovación del crumb) a cualquier endpoint de Yahoo que gohoo todavía no cubre.
- Errores tipados: `*StatusError` (con `RetryAfter`), `ErrRateLimit`, `ErrNotFound`, `ErrUnauthorized`, `IsRateLimit(err)` — funcionan con `errors.Is`/`errors.As` incluso tras agotar los reintentos.
- `RetryPolicy` / `DefaultRetryPolicy()` — intentos, backoff, tope, jitter, si reintentar 429, `Retry-After` y hook `OnRetry`; `RetryPolicy.RateLimit` da a los rate limits su propio presupuesto de reintentos y backoff, y `WithRetryPolicyContext(ctx, p)` cambia la política para una sola llamada.
- `(*Client) Quotes(ctx, símbolos []string) ([]Quote, error)` — cotización actual (precio, variación, market cap, moneda, ...) de muchos símbolos, en lotes de 100 por request.
- `(*Client) Info(ctx, ticker, opts...)` / `FlattenInfo(qs, opts...)` — mapa plano estilo `Ticker.info` a partir de `quoteSummary` (prioridad de módulos, números sin envoltorio; opciones para el dividend yield en porcentaje y `trailingPegRatio`).
- `TimeseriesPoints(data, tipo) []Point` y `(*Client) TrailingPEG` — observaciones tipadas de fundamentals-timeseries, la más reciente primero.
- `AutoAdjust(bars)` / `(*ChartResult) AutoAdjusted()`, `Bar.Date()` y `AtMidnight(bars)` — `auto_adjust` estilo yfinance y barras indexadas por fecha.
- `WithLimiter(l)` / `WithRequestHook(fn)` — un limitador propio antes, y un hook de métricas/logs después, de cada request HTTP (reintentos y bootstrap de sesión incluidos).
- `ComputeIndicators(series []OHLCV) []IndicatorSet` — SMA, EMA, RSI, MACD, Bollinger, ATR, Estocástico, Williams %R, ADX, VWAP, OBV y señales derivadas.
- `ComputeIndicatorsBars(bars []Bar) []IndicatorSet` más indicadores sueltos con períodos configurables (`SMA`, `EMA`, `RSI`, `MACD`, `Bollinger`, `ATR`, `Stochastic`, `WilliamsR`, `ADX`, `OBV`, `VWAP`, `Crossovers`) sobre columnas de `Closes`, `Highs`, `Lows`, `Volumes`...
- Métricas de riesgo: `Returns`, `LogReturns`, `Volatility` (anualizada) y `MaxDrawdown`.
- `DetectGaps(series []OHLCV, factor float64) []Gap` — detecta huecos inusuales en una serie.
- Helpers de parseo de JSON de Yahoo: `RawValue`, `GetPath`, `Field`, `AsFloat`, `AsInt64`, `AsString`, `EpochToDate`, `EpochToDateTimeNY`, `EpochToMidnight`, `DateStringToMidnight`, `ParseDateOnly`.

Todos los números que devuelve Yahoo en `quoteSummary`/`fundamentals-timeseries` llegan como `{"raw": x, "fmt": "texto formateado"}`; `RawValue`/`Field` extraen siempre `raw`.

## Notas

- `Client` es seguro para uso concurrente y reintenta solo ante fallas transitorias (red, 429, 5xx) con backoff; ante 401 renueva la sesión.
- No hagas ráfagas de peticiones: Yahoo aplica rate limit por IP.
- No requiere API key ni autenticación de usuario: solo cookies de sesión + un token "crumb" público, obtenidos automáticamente por `NewClient` (o en el primer uso con `WithLazyBootstrap`).
- `rango` e `intervalo` son los mismos valores que acepta Yahoo Finance en su UI/API: `1d, 5d, 1mo, 3mo, 6mo, 1y, 2y, 5y, 10y, ytd, max` y `1m, 2m, 5m, 15m, 30m, 60m/1h, 1d, 5d, 1wk, 1mo, 3mo` respectivamente. Yahoo limita cuánto histórico entrega para los intervalos intradía.
- Sirve igual para acciones (`AAPL`), ETFs (`SPY`) y criptomonedas (`BTC-USD`) — todas comparten el mismo endpoint `chart` en Yahoo Finance.
- `ChartRange` ubica las barras en la zona horaria del exchange. En Windows sin Go instalado, importá `_ "time/tzdata"` en tu aplicación; si no, la zona cae en UTC. `gohoo` no lo importa para no agrandar los binarios.

### Serie mensual por rango de fechas

```go
client, err := gohoo.NewClientWithOptions(
	gohoo.WithLazyBootstrap(), // chart no necesita cookie+crumb
	gohoo.WithRetryPolicy(gohoo.RetryPolicy{MaxAttempts: 1, RetryRateLimit: false}),
)
if err != nil {
	log.Fatal(err)
}
res, err := client.ChartRange(ctx, "SPY", gohoo.ChartParams{
	Start:    time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
	Interval: "1mo",
})
switch {
case gohoo.IsRateLimit(err):
	// aplicar un cooldown propio
case errors.Is(err, gohoo.ErrNotFound):
	// símbolo inexistente o deslistado
case err != nil:
	log.Fatal(err)
}
dates, adj := res.Monthly()
```

## Tests

```bash
go test ./...
```

Los tests no requieren red: simulan las respuestas de Yahoo con un `httptest.Server` local, algunos con respuestas grabadas de Yahoo en `testdata/`. Las pruebas contra Yahoo real se corren solo a pedido:

```bash
go test -tags live -run Live ./...
```

El CI corre `gofmt`, `go vet`, `golangci-lint`, `apidiff` (sin cambios incompatibles de API entre versiones) y los tests en Linux y Windows con Go 1.22 y la última versión de Go.

## Licencia

[MIT](LICENSE)

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

## Uso

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

	// Indicadores y módulos "en crudo" de quoteSummary.
	data, err := client.QuoteSummary("AAPL", []string{"price", "summaryDetail"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Precio actual:", gohoo.Field(data["price"].(map[string]interface{}), "regularMarketPrice"))

	// Histórico OHLCV ya parseado (acciones, ETFs o criptomonedas).
	series, dividends, splits, err := client.ChartSeries("AAPL", "1y", "1d")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%d velas, %d fechas con dividendo, %d con split\n", len(series), len(dividends), len(splits))
}
```

## API

- `NewClient() (*Client, error)` — crea una sesión autenticada (cookie + crumb de Yahoo).
- `(*Client) QuoteSummary(ticker string, modules []string) (map[string]interface{}, error)` — módulos de `quoteSummary` (precio, estadísticas, estimaciones de analistas, etc.), fusionados en un solo mapa.
- `(*Client) FundamentalsTimeseries(ticker string, types []string) (map[string]interface{}, error)` — estados financieros anuales/trimestrales vía `fundamentals-timeseries`.
- `(*Client) Chart(ticker, rango, intervalo string) (map[string]interface{}, error)` — respuesta cruda del endpoint `chart` (OHLCV + dividendos/splits).
- `(*Client) ChartSeries(ticker, rango, intervalo string) ([]OHLCV, dividendos, splits map[string]float64, error)` — `Chart` ya parseado a una serie ordenada, más dividendos/splits indexados por fecha (`"YYYY-MM-DD"`).
- `NewClientWithOptions(opts ...Option) (*Client, error)` — cliente configurable: `WithHTTPClient`, `WithBaseURL`, `WithUserAgent`, `WithRetryPolicy`, `WithLazyBootstrap` (no hace red hasta que de verdad se necesita el crumb). Sin opciones se comporta exactamente igual que `NewClient()`.
- `QuoteSummaryContext`, `FundamentalsTimeseriesContext`, `ChartContext`, `ChartSeriesContext` — iguales a los anteriores, cancelables con un `context.Context` (también durante la espera entre reintentos).
- `(*Client) ChartRange(ctx, ticker string, p ChartParams) (*ChartResult, error)` — `chart` por rango relativo o por fechas (`period1`/`period2`), con barras `float64` en la zona horaria del exchange y los `null` de Yahoo resueltos.
- `(*ChartResult) Monthly() (dates []time.Time, adj []float64)` — un cierre ajustado por mes calendario; dentro de un mes gana la barra más reciente (la mensual frente a la "en vivo").
- `(*Client) Screen(ctx, p ScreenParams) (*ScreenResult, error)` — screener de Yahoo con constructor de queries (`Eq`, `Gt`, `Lt`, `Gte`, `Lte`, `Btwn`, `IsIn`, `And`, `Or`), paginación y orden; `ScreenPredefined(ctx, id, offset, size)` para los screeners guardados de Yahoo (`day_gainers`, `most_actives`, ...).
- `(*Client) GetJSON` / `PostJSON(ctx, path, query[, body])` — petición autenticada (cookie + crumb, reintentos, renovación del crumb) a cualquier endpoint de Yahoo que gohoo todavía no cubre.
- Errores tipados: `*StatusError` (con `RetryAfter`), `ErrRateLimit`, `ErrNotFound`, `ErrUnauthorized`, `IsRateLimit(err)` — funcionan con `errors.Is`/`errors.As` incluso tras agotar los reintentos.
- `RetryPolicy` / `DefaultRetryPolicy()` — intentos, backoff, tope, jitter, si reintentar 429, `Retry-After` y hook `OnRetry`; `RetryPolicy.RateLimit` da a los rate limits su propio presupuesto de reintentos y backoff, y `WithRetryPolicyContext(ctx, p)` cambia la política para una sola llamada.
- `(*Client) Quotes(ctx, símbolos []string) ([]Quote, error)` — cotización actual (precio, variación, market cap, moneda, ...) de muchos símbolos, en lotes de 100 por request.
- `WithLimiter(l)` / `WithRequestHook(fn)` — un limitador propio antes, y un hook de métricas/logs después, de cada request HTTP (reintentos y bootstrap de sesión incluidos).
- `ComputeIndicators(series []OHLCV) []IndicatorSet` — SMA, EMA, RSI, MACD, Bollinger, ATR, Estocástico, Williams %R, ADX, VWAP, OBV y señales derivadas.
- `DetectGaps(series []OHLCV, factor float64) []Gap` — detecta huecos inusuales en una serie.
- Helpers de parseo de JSON de Yahoo: `RawValue`, `GetPath`, `Field`, `AsFloat`, `AsInt64`, `AsString`, `EpochToDate`, `EpochToDateTimeNY`, `EpochToMidnight`, `DateStringToMidnight`, `ParseDateOnly`.

Todos los números que devuelve Yahoo en `quoteSummary`/`fundamentals-timeseries` llegan como `{"raw": x, "fmt": "texto formateado"}`; `RawValue`/`Field` extraen siempre `raw`.

## Notas

- `Client` es seguro para uso concurrente y reintenta solo ante fallas transitorias (red, 429, 5xx) con backoff; ante 401 renueva la sesión.
- No hagas ráfagas de peticiones: Yahoo aplica rate limit por IP.
- No requiere API key ni autenticación de usuario: solo cookies de sesión + un token "crumb" público, obtenidos automáticamente en `NewClient`.
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

Los tests no requieren red: simulan las respuestas de Yahoo con un `httptest.Server` local.

## Licencia

[MIT](LICENSE)

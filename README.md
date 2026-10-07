# gohoo

Cliente Go minimalista para los endpoints no oficiales de Yahoo Finance (`quoteSummary`, `fundamentals-timeseries` y `chart`). Equivalente, para Go, a lo que es [`yfinance`](https://github.com/ranaroussi/yfinance) para Python: sin API key, hablando directo con `query2.finance.yahoo.com` vía `net/http`.

Extraído de [`go-finz-analysis`](https://github.com/loc1263/go-finz-analysis), donde nació como paquete interno usado por varios binarios (indicadores de acciones, precios de acciones/ETFs/criptomonedas). Esta es la versión reutilizable, sin dependencias externas.

Documentación: [API.md](API.md) (referencia de funciones/tipos) · [ENDPOINTS.md](ENDPOINTS.md) (qué datos trae cada endpoint de Yahoo).

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

	yahoo "github.com/loc1263/gohoo"
)

func main() {
	client, err := yahoo.NewClient()
	if err != nil {
		log.Fatal(err)
	}

	// Indicadores y módulos "en crudo" de quoteSummary.
	data, err := client.QuoteSummary("AAPL", []string{"price", "summaryDetail"})
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Precio actual:", yahoo.Field(data["price"].(map[string]interface{}), "regularMarketPrice"))

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
- Helpers de parseo de JSON de Yahoo: `RawValue`, `GetPath`, `Field`, `AsFloat`, `AsInt64`, `AsString`, `EpochToDate`, `EpochToDateTimeNY`, `EpochToMidnight`, `DateStringToMidnight`, `ParseDateOnly`.

Todos los números que devuelve Yahoo en `quoteSummary`/`fundamentals-timeseries` llegan como `{"raw": x, "fmt": "texto formateado"}`; `RawValue`/`Field` extraen siempre `raw`.

## Notas

- No requiere API key ni autenticación de usuario: solo cookies de sesión + un token "crumb" público, obtenidos automáticamente en `NewClient`.
- `rango` e `intervalo` son los mismos valores que acepta Yahoo Finance en su UI/API: `1d, 5d, 1mo, 3mo, 6mo, 1y, 2y, 5y, 10y, ytd, max` y `1m, 2m, 5m, 15m, 30m, 60m/1h, 1d, 5d, 1wk, 1mo, 3mo` respectivamente. Yahoo limita cuánto histórico entrega para los intervalos intradía.
- Sirve igual para acciones (`AAPL`), ETFs (`SPY`) y criptomonedas (`BTC-USD`) — todas comparten el mismo endpoint `chart` en Yahoo Finance.

## Tests

```bash
go test ./...
```

Los tests no requieren red: simulan las respuestas de Yahoo con un `httptest.Server` local.

## Licencia

[MIT](LICENSE)

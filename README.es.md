# gohoo

[English](README.md) | Español

Cliente Go minimalista para los endpoints no oficiales de Yahoo Finance. Es, para Go, lo que [`yfinance`](https://github.com/ranaroussi/yfinance) es para Python: sin API key y sin dependencias fuera de la librería estándar.

> **Aviso:** gohoo no está afiliado ni respaldado por Yahoo. Usa endpoints **no oficiales y no documentados** que pueden cambiar o dejar de funcionar sin aviso, y su uso está sujeto a los términos de servicio de Yahoo. Pensado para proyectos personales y educativos; los datos no deben tomarse como asesoría financiera ni usarse para operar sin verificarlos. Se entrega "tal cual" (ver [LICENSE](LICENSE)).

## Instalación

```bash
go get github.com/loc1263/gohoo
```

Requiere Go 1.22 o superior.

## Inicio rápido

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

	// Un año de velas diarias, ajustadas por dividendos y splits, y un indicador.
	res, err := client.ChartRange(ctx, "AAPL", gohoo.ChartParams{Range: "1y", Interval: "1d"})
	if err != nil {
		log.Fatal(err)
	}
	bars := res.AutoAdjusted()
	rsi := gohoo.RSI(gohoo.Closes(bars), 14)
	fmt.Printf("%d velas, último RSI14 %.1f\n", len(bars), rsi[len(rsi)-1])

	// Cotización actual de varios símbolos.
	quotes, err := client.Quotes(ctx, []string{"AAPL", "MSFT", "BTC-USD"})
	if err != nil {
		log.Fatal(err)
	}
	for _, q := range quotes {
		fmt.Println(q.Symbol, q.Price, q.Currency)
	}
}
```

## Qué hace

| Quiero… | Uso |
|---|---|
| Historial de precios (OHLCV) | `ChartRange`, y luego `AutoAdjusted()` para precios ajustados o `Monthly()` para una serie mensual |
| Precio actual de muchos símbolos | `Quotes` |
| Datos de la empresa (estilo `Ticker.info`) | `Info` |
| Estados financieros | `FundamentalsTimeseriesContext` + `TimeseriesPoints` |
| Buscar instrumentos por criterios | `Screen`, `ScreenPredefined` |
| Indicadores técnicos | `RSI`, `MACD`, `SMA`, `EMA`, `Bollinger`, `ATR`, `ADX`, `MFI`... o `ComputeIndicatorsBars` para el conjunto completo |
| Métricas de riesgo | `Returns`, `Volatility`, `MaxDrawdown` |
| Cualquier otro endpoint de Yahoo | `GetJSON`, `PostJSON` |

Acciones (`AAPL`), ETFs (`SPY`), índices (`^VIX`), monedas (`CLP=X`) y cripto (`BTC-USD`) funcionan igual.

Los errores se distinguen con `errors.Is`: `gohoo.ErrNotFound` (símbolo inexistente o deslistado) y `gohoo.ErrRateLimit` (Yahoo te está limitando).

## Documentación

- [EXAMPLES.md](EXAMPLES.md): ejemplos de uso de cada parte de la librería.
- [API.md](API.md): referencia de todas las funciones y tipos.
- [ENDPOINTS.md](ENDPOINTS.md): qué datos trae cada endpoint de Yahoo.
- [TECHNICAL.md](TECHNICAL.md): cómo funciona (sesión, reintentos, límites de tasa, errores, indicadores), reglas de compatibilidad, tests y cómo publicar una versión.
- [CHANGELOG.md](CHANGELOG.md): qué cambió en cada versión.

## Tests

```bash
go test ./...
```

No necesitan red: usan un servidor Yahoo falso local. Las pruebas contra Yahoo real y el CI están en [TECHNICAL.md](TECHNICAL.md).

## Licencia

[MIT](LICENSE)

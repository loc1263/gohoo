# gohoo: ejemplos

Ejemplos de uso, de lo más simple a lo más específico. Para el detalle de cada función, ver [API.md](API.md); para cómo funciona por dentro, [TECHNICAL.md](TECHNICAL.md).

Los fragmentos suponen estos imports y las variables `ctx` y `client` del primer ejemplo. Cada fragmento se compila contra la versión actual de la librería. Se ejecutan contra Yahoo real salvo el último ejemplo, que usa un servidor local.

```go
import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/loc1263/gohoo"
)
```

## Índice

1. [Crear el cliente](#1-crear-el-cliente)
2. [Historial de precios](#2-historial-de-precios)
3. [Cotizaciones de varios símbolos](#3-cotizaciones-de-varios-símbolos)
4. [Datos de la empresa](#4-datos-de-la-empresa)
5. [Estados financieros](#5-estados-financieros)
6. [Screener: buscar instrumentos](#6-screener-buscar-instrumentos)
7. [Indicadores técnicos](#7-indicadores-técnicos)
8. [Métricas de riesgo](#8-métricas-de-riesgo)
9. [Tratar los errores](#9-tratar-los-errores)
10. [Muchos símbolos sin que Yahoo te limite](#10-muchos-símbolos-sin-que-yahoo-te-limite)
11. [Otros endpoints y utilidades de parseo](#11-otros-endpoints-y-utilidades-de-parseo)
12. [Probar tu código sin red](#12-probar-tu-código-sin-red)

## 1. Crear el cliente

```go
ctx := context.Background()

// Lo más simple: pide cookie y crumb al crearse.
client, err := gohoo.NewClient()
if err != nil {
	log.Fatal(err)
}
```

Con `WithLazyBootstrap` el constructor no hace red; la sesión se pide solo cuando un endpoint la necesita (`ChartRange` nunca la necesita). Otras opciones:

```go
client, err := gohoo.NewClientWithOptions(
	gohoo.WithLazyBootstrap(),
	gohoo.WithUserAgent("mi-app/1.0"),
	gohoo.WithRetryPolicy(gohoo.RetryPolicy{MaxAttempts: 3, BaseDelay: 300 * time.Millisecond}),
	gohoo.WithLimiter(gohoo.NewIntervalLimiter(40*time.Millisecond)), // ~25 requests por segundo
)
if err != nil {
	log.Fatal(err)
}
_ = client
```

Un solo `Client` por proceso: es seguro entre goroutines.

## 2. Historial de precios

Un año de velas diarias, ajustadas por dividendos y splits:

```go
res, err := client.ChartRange(ctx, "AAPL", gohoo.ChartParams{Range: "1y", Interval: "1d"})
if err != nil {
	log.Fatal(err)
}
for _, b := range res.AutoAdjusted() {
	fmt.Println(b.Time.Format("2006-01-02"), b.Close, b.Volume)
}
fmt.Println(res.Meta.Currency, res.Meta.ExchangeTimezoneName)
```

Por fechas, en vez de un rango relativo (`End` vacío = ahora):

```go
res, err := client.ChartRange(ctx, "SPY", gohoo.ChartParams{
	Start:    time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC),
	End:      time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	Interval: "1wk",
})
if err != nil {
	log.Fatal(err)
}
fmt.Println(len(res.Bars), "velas semanales")
```

Un cierre ajustado por mes calendario:

```go
res, err := client.ChartRange(ctx, "SPY", gohoo.ChartParams{
	Start:    time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
	Interval: "1mo",
})
if err != nil {
	log.Fatal(err)
}
dates, adj := res.Monthly()
for i := range dates {
	fmt.Println(dates[i].Format("2006-01"), adj[i])
}
```

`DailyBars` es el atajo para barras diarias ajustadas, con la hora en la medianoche de cada día (ideal para guardar o comparar por fecha). `end` es exclusivo:

```go
bars, err := client.DailyBars(ctx, "AAPL", time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC))
if err != nil {
	log.Fatal(err) // errors.Is(err, gohoo.ErrNotFound) si Yahoo no devolvió barras
}
fmt.Println(len(bars), bars[0].Date().Format("2006-01-02"))
```

Con dividendos y splits por fecha, usando `ChartSeries`:

```go
series, dividends, splits, err := client.ChartSeries("AAPL", "5y", "1d")
if err != nil {
	log.Fatal(err)
}
fmt.Println(len(series), "velas;", len(dividends), "dividendos;", len(splits), "splits")
for date, amount := range dividends {
	fmt.Println(date, amount) // la fecha es "YYYY-MM-DD"
}
```

## 3. Cotizaciones de varios símbolos

Acciones, ETFs, índices, monedas y cripto en la misma llamada (en lotes de 100 por request):

```go
quotes, err := client.Quotes(ctx, []string{"AAPL", "SPY", "^VIX", "CLP=X", "BTC-USD"})
if err != nil {
	log.Fatal(err)
}
for _, q := range quotes {
	fmt.Printf("%-8s %10.2f %s  (%+.2f%%)\n", q.Symbol, q.Price, q.Currency, q.ChangePercent)
}
```

Los símbolos que Yahoo no conoce simplemente no aparecen en el resultado. Acciones en circulación:

```go
quotes, err := client.Quotes(ctx, []string{"AAPL", "MSFT"})
if err != nil {
	log.Fatal(err)
}
for _, q := range quotes {
	fmt.Println(q.Symbol, q.Shares()) // 0 si Yahoo no lo informa (índices, monedas, muchos ETF)
}
```

## 4. Datos de la empresa

`Info` devuelve un mapa plano, al estilo de `Ticker.info` de yfinance:

```go
info, err := client.Info(ctx, "AAPL")
if err != nil {
	log.Fatal(err)
}
fmt.Println(gohoo.AsString(info["longName"]), "-", gohoo.AsString(info["sector"]))
if pe, ok := gohoo.AsFloat(info["trailingPE"]); ok {
	fmt.Printf("PER: %.1f\n", pe)
}
```

Con opciones: el dividend yield en porcentaje y el PEG (que Yahoo publica aparte):

```go
info, err := client.Info(ctx, "KO", gohoo.InfoPercentYield(), gohoo.InfoWithTrailingPEG())
if err != nil {
	log.Fatal(err)
}
fmt.Println(info["dividendYield"], info["trailingPegRatio"])
```

Si necesitas módulos concretos de `quoteSummary` en crudo:

```go
data, err := client.QuoteSummaryContext(ctx, "AAPL", []string{"price", "summaryDetail"})
if err != nil {
	log.Fatal(err)
}
price, _ := data["price"].(map[string]any)
fmt.Println(gohoo.Field(price, "regularMarketPrice")) // Field saca el "raw" del {"raw": x, "fmt": "..."}
```

## 5. Estados financieros

`FundamentalsTimeseriesContext` baja series anuales y trimestrales; `TimeseriesPoints` las convierte en observaciones tipadas, la más reciente primero:

```go
data, err := client.FundamentalsTimeseriesContext(ctx, "AAPL", []string{"annualTotalRevenue", "quarterlyNetIncome"})
if err != nil {
	log.Fatal(err)
}
for _, p := range gohoo.TimeseriesPoints(data, "annualTotalRevenue") {
	fmt.Println(p.Date, p.PeriodType, p.Currency, p.Value)
}
```

Helpers sobre esas observaciones:

```go
data, err := client.FundamentalsTimeseriesContext(ctx, "AAPL", []string{"quarterlyTotalRevenue", "quarterlyNetIncome"})
if err != nil {
	log.Fatal(err)
}
revenue := gohoo.TimeseriesPoints(data, "quarterlyTotalRevenue")
income := gohoo.TimeseriesPoints(data, "quarterlyNetIncome")

last, ok := gohoo.LatestPoint(revenue) // el trimestre más reciente
if !ok {
	log.Fatal("sin datos")
}
// El resultado del mismo cierre, para calcular un margen.
if ni, ok := gohoo.PointOn(income, last.Date); ok {
	fmt.Printf("margen neto del trimestre %s: %.1f%%\n", last.Date, ni.Value/last.Value*100)
}
// Ingresos de los últimos 12 meses (4 trimestres).
if ttm, ok := gohoo.SumLast(revenue, 4); ok {
	fmt.Printf("ingresos 12M: %.0f\n", ttm)
}
```

`FilterPeriodType` separa por tipo de período (`"3M"`, `"12M"`...) cuando una serie los mezcla, y `TrailingPEG` baja el PEG directamente:

```go
data, err := client.FundamentalsTimeseriesContext(ctx, "AAPL", []string{"annualNetIncome"})
if err != nil {
	log.Fatal(err)
}
annual := gohoo.FilterPeriodType(gohoo.TimeseriesPoints(data, "annualNetIncome"), "12M")
fmt.Println(len(annual), "años")

peg, ok, err := client.TrailingPEG(ctx, "AAPL")
if err != nil {
	log.Fatal(err)
}
fmt.Println(peg, ok) // ok es false si Yahoo no tiene el dato
```

## 6. Screener: buscar instrumentos

Las 50 mayores empresas de EE. UU. por capitalización:

```go
res, err := client.Screen(ctx, gohoo.ScreenParams{
	Query:     gohoo.And(gohoo.Eq("region", "us"), gohoo.Gte("intradaymarketcap", 200e9)),
	Size:      50,
	SortField: "intradaymarketcap",
})
if err != nil {
	log.Fatal(err)
}
fmt.Println(res.Total, "empresas en total;", res.Count, "en esta página")
for _, q := range res.Quotes {
	fmt.Println(q.Symbol, q.ShortName, q.MarketCap)
}
```

Condiciones disponibles: `Eq`, `Gt`, `Lt`, `Gte`, `Lte`, `Btwn`, `IsIn`, `And` y `Or`. Con paginación y ETFs:

```go
q := gohoo.And(gohoo.Eq("region", "us"), gohoo.Eq("fundfamilyname", "Vanguard"))
for offset := 0; ; offset += 100 {
	res, err := client.Screen(ctx, gohoo.ScreenParams{
		Query:     q,
		QuoteType: "ETF",
		Offset:    offset,
		Size:      100,
		SortField: "fundnetassets", // para ETF no vale "intradaymarketcap"
	})
	if err != nil {
		log.Fatal(err)
	}
	for _, e := range res.Quotes {
		fmt.Println(e.Symbol, e.ShortName)
	}
	if offset+res.Count >= res.Total || res.Count == 0 {
		break
	}
}
```

Los screeners guardados de Yahoo:

```go
res, err := client.ScreenPredefined(ctx, "day_gainers", 0, 25)
if err != nil {
	log.Fatal(err) // un id inexistente cumple errors.Is(err, gohoo.ErrNotFound)
}
for _, q := range res.Quotes {
	fmt.Println(q.Symbol, q.ShortName)
}
```

## 7. Indicadores técnicos

El conjunto estándar (SMA20, EMA12/26, RSI14, MACD, Bollinger, ATR, estocástico, Williams %R, ADX, VWAP, OBV):

```go
res, err := client.ChartRange(ctx, "SPY", gohoo.ChartParams{Range: "2y", Interval: "1d"})
if err != nil {
	log.Fatal(err)
}
set := gohoo.ComputeIndicatorsBars(res.AutoAdjusted())
last := set[len(set)-1]
fmt.Printf("RSI14 %.1f  MACD %.2f  histograma %.2f\n", last.RSI14, last.MACD, last.MACDHist)
```

O cada indicador con el período que quieras, sobre columnas de las barras. Los valores sin historia suficiente son `NaN`:

```go
res, err := client.ChartRange(ctx, "SPY", gohoo.ChartParams{Range: "2y", Interval: "1d"})
if err != nil {
	log.Fatal(err)
}
bars := res.AutoAdjusted()
closes := gohoo.Closes(bars)

rsi7 := gohoo.RSI(closes, 7)
line, signal, hist := gohoo.MACD(closes, 12, 26, 9)
upper, middle, lower := gohoo.Bollinger(closes, 20, 2)
mfi := gohoo.MFI(gohoo.Highs(bars), gohoo.Lows(bars), closes, gohoo.Volumes(bars), 14)
n := len(closes) - 1
fmt.Println(rsi7[n], line[n], signal[n], hist[n], upper[n], middle[n], lower[n], mfi[n])

// Cruces de medias móviles: +1 cruce al alza (golden cross), -1 a la baja (death cross).
cross := gohoo.Crossovers(gohoo.SMA(closes, 50), gohoo.SMA(closes, 200))
for i, c := range cross {
	if c != 0 {
		fmt.Println(bars[i].Time.Format("2006-01-02"), c)
	}
}
```

Si los volúmenes de Yahoo traen huecos, `MFISkipNaN` ignora esas barras en vez de dejar toda la ventana en `NaN`:

```go
res, err := client.ChartRange(ctx, "AAPL", gohoo.ChartParams{Range: "1y", Interval: "1d"})
if err != nil {
	log.Fatal(err)
}
b := res.Bars
mfi := gohoo.MFISkipNaN(gohoo.Highs(b), gohoo.Lows(b), gohoo.Closes(b), gohoo.Volumes(b), 14)
fmt.Printf("%.1f\n", mfi[len(mfi)-1])
```

## 8. Métricas de riesgo

```go
res, err := client.ChartRange(ctx, "SPY", gohoo.ChartParams{Range: "5y", Interval: "1d"})
if err != nil {
	log.Fatal(err)
}
closes := gohoo.AdjCloses(res.Bars)

returns := gohoo.Returns(closes) // retornos simples; el primero es NaN
vol := gohoo.Volatility(returns, 252) // anualizada (252 para retornos diarios)
dd, peak, trough := gohoo.MaxDrawdown(closes)
fmt.Printf("volatilidad %.1f%%  máxima caída %.1f%% (de %s a %s)\n",
	vol*100, dd*100, res.Bars[peak].Time.Format("2006-01-02"), res.Bars[trough].Time.Format("2006-01-02"))
```

## 9. Tratar los errores

Los errores se distinguen con `errors.Is` / `errors.As`; el texto no es parte de la API.

```go
res, err := client.ChartRange(ctx, "SIMBOLOQUENOEXISTE", gohoo.ChartParams{Range: "1mo"})
switch {
case errors.Is(err, gohoo.ErrNotFound):
	fmt.Println("símbolo inexistente, deslistado o sin datos")
case gohoo.IsRateLimit(err):
	fmt.Println("Yahoo te está limitando: espera antes de seguir")
case err != nil:
	var se *gohoo.StatusError
	if errors.As(err, &se) {
		fmt.Println("HTTP", se.StatusCode, "reintentar en", se.RetryAfter)
	}
	log.Fatal(err)
default:
	fmt.Println(len(res.Bars), "velas")
}
```

Lo mismo vale para `Info`, `Quotes` y el resto: un símbolo inexistente cumple `errors.Is(err, gohoo.ErrNotFound)`.

## 10. Muchos símbolos sin que Yahoo te limite

`ParallelMap` baja muchos símbolos con un máximo de goroutines y devuelve los resultados en el mismo orden. El ritmo de las requests lo fija el limitador del cliente, no el número de goroutines:

```go
limited, err := gohoo.NewClientWithOptions(
	gohoo.WithLazyBootstrap(),
	gohoo.WithLimiter(gohoo.NewIntervalLimiter(50*time.Millisecond)), // 20 requests por segundo en total
)
if err != nil {
	log.Fatal(err)
}

tickers := []string{"AAPL", "MSFT", "GOOGL", "AMZN", "NVDA", "XXXXXX"}
results := gohoo.ParallelMap(ctx, tickers, 4, func(ctx context.Context, t string) (float64, error) {
	bars, err := limited.DailyBars(ctx, t, time.Now().AddDate(0, -1, 0), time.Now())
	if err != nil {
		return 0, err
	}
	return bars[len(bars)-1].Close, nil
})
for i, r := range results {
	if r.Err != nil {
		fmt.Println(tickers[i], "error:", r.Err)
		continue
	}
	fmt.Println(tickers[i], r.Value)
}
```

Para medir qué tan seguido te limita Yahoo, `WithRequestHook` recibe cada request ya terminada. Se llama desde la goroutine de la request, así que debe ser rápido y seguro entre goroutines:

```go
var throttled atomic.Int64 // import "sync/atomic"
hooked, err := gohoo.NewClientWithOptions(
	gohoo.WithLazyBootstrap(),
	gohoo.WithRequestHook(func(ev gohoo.RequestEvent) {
		if ev.StatusCode == 429 {
			throttled.Add(1)
		}
		log.Printf("%s %s %s intento %d: %d en %s", ev.Method, ev.Endpoint, ev.Symbol, ev.Attempt, ev.StatusCode, ev.Duration)
	}),
)
if err != nil {
	log.Fatal(err)
}
_ = hooked
```

La política de reintentos también se puede cambiar para una sola llamada, con el mismo cliente. Por ejemplo, sin esperas largas dentro de un request web:

```go
quick := gohoo.WithRetryPolicyContext(ctx, gohoo.RetryPolicy{MaxAttempts: 1})
if _, err := client.Quotes(quick, []string{"AAPL"}); err != nil {
	log.Println("falló rápido:", err)
}
```

## 11. Otros endpoints y utilidades de parseo

Cualquier endpoint de Yahoo que gohoo no envuelva, con la misma sesión, reintentos y renovación del crumb:

```go
body, err := client.GetJSON(ctx, "/v1/finance/trending/US", nil)
if err != nil {
	log.Fatal(err)
}
fmt.Println(len(body), "bytes de JSON")
```

Ayudas para recorrer JSON de Yahoo:

```go
summary := map[string]any{
	"summaryDetail": map[string]any{
		"trailingPE": map[string]any{"raw": 28.5, "fmt": "28.50"},
	},
}
pe := gohoo.RawValue(gohoo.GetPath(summary, "summaryDetail", "trailingPE"))
v, _ := gohoo.AsFloat(pe)
fmt.Println(v)                                       // 28.5
fmt.Println(gohoo.EpochToDate(float64(1700000000))) // 2023-11-14
```

## 12. Probar tu código sin red

`WithBaseURL` apunta el cliente a un servidor local, para que tus tests no dependan de Yahoo:

```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	_, _ = w.Write([]byte(`{"chart":{"result":[{
		"meta":{"symbol":"TEST","currency":"USD","exchangeTimezoneName":"America/New_York"},
		"timestamp":[1704205800,1704292200],
		"indicators":{"quote":[{"open":[10,11],"high":[11,12],"low":[9,10],"close":[10.5,11.5],"volume":[100,200]}],
		"adjclose":[{"adjclose":[10.5,11.5]}]}}],"error":null}}`))
}))
defer srv.Close()

test, err := gohoo.NewClientWithOptions(gohoo.WithBaseURL(srv.URL), gohoo.WithLazyBootstrap())
if err != nil {
	log.Fatal(err)
}
res, err := test.ChartRange(ctx, "TEST", gohoo.ChartParams{Range: "5d"})
if err != nil {
	log.Fatal(err)
}
fmt.Println(len(res.Bars), res.Bars[1].Close) // 2 11.5
```

Necesita los imports `net/http` y `net/http/httptest`. La librería usa este mismo método en sus propios tests.

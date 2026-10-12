# API de gohoo

Referencia completa del paquete `gohoo` (módulo `github.com/loc1263/gohoo`). Para una introducción rápida ver [README.md](README.md). Para qué significa cada módulo/campo que devuelve Yahoo en cada endpoint, ver [ENDPOINTS.md](ENDPOINTS.md).

```go
import "github.com/loc1263/gohoo"
```

## Índice

- [Client](#client)
  - [NewClient](#newclient)
  - [QuoteSummary](#quotesummary)
  - [FundamentalsTimeseries](#fundamentalstimeseries)
  - [Chart](#chart)
  - [ChartSeries](#chartseries)
  - [NewClientWithOptions](#newclientwithoptions)
  - [Variantes con context](#variantes-con-context)
  - [ChartRange](#chartrange)
  - [ChartResult.Monthly](#chartresultmonthly)
  - [GetJSON y PostJSON](#getjson-y-postjson)
  - [Screen y ScreenPredefined](#screen-y-screenpredefined)
  - [Quotes](#quotes)
  - [WithLimiter y WithRequestHook](#withlimiter-y-withrequesthook)
  - [Info y FlattenInfo](#info-y-flatteninfo)
  - [TimeseriesPoints y TrailingPEG](#timeseriespoints-y-trailingpeg)
  - [AutoAdjust, Bar.Date y AtMidnight](#autoadjust-bardate-y-atmidnight)
- [Indicadores técnicos](#indicadores-técnicos)
  - [Sobre barras tipadas y con períodos configurables](#sobre-barras-tipadas-y-con-períodos-configurables)
  - [Métricas de riesgo](#métricas-de-riesgo)
- [Detección de huecos](#detección-de-huecos)
- [Tipos](#tipos)
  - [OHLCV](#ohlcv)
  - [ChartParams, ChartResult, ChartMeta, Bar](#chartparams-chartresult-chartmeta-bar)
  - [RetryPolicy](#retrypolicy)
- [Helpers de parseo JSON](#helpers-de-parseo-json)
- [Helpers de fecha/hora](#helpers-de-fechahora)
- [Manejo de errores](#manejo-de-errores)
- [Límites y particularidades de Yahoo Finance](#límites-y-particularidades-de-yahoo-finance)

---

## Client

```go
type Client struct {
    // campos no exportados: http.Client con cookie jar + crumb de sesión
}
```

`Client` encapsula la sesión HTTP (cookies + token "crumb") necesaria para hablar con `query2.finance.yahoo.com`. Es seguro reutilizar la misma instancia para múltiples llamadas y tickers, también desde varias goroutines; no hace falta crear un `Client` por símbolo.

### NewClient

```go
func NewClient() (*Client, error)
```

Crea un `Client` autenticado:

1. Hace un `GET` a `https://fc.yahoo.com` para fijar cookies de consentimiento/sesión (se ignora cualquier error, incluyendo 404).
2. Hace un `GET` a `https://query2.finance.yahoo.com/v1/test/getcrumb` para obtener el token "crumb".

Devuelve error si no se pudo construir el cookie jar o si la respuesta del crumb viene vacía o es HTML (p. ej. una página de error/consentimiento en vez del token).

```go
client, err := gohoo.NewClient()
if err != nil {
    log.Fatal(err)
}
```

### QuoteSummary

```go
func (c *Client) QuoteSummary(ticker string, modules []string) (map[string]interface{}, error)
```

Llama a `v10/finance/quoteSummary/{ticker}` con los módulos pedidos (join por `,`) y el crumb de la sesión. Los módulos de `quoteSummary.result` se fusionan en un único `map[string]interface{}` (si Yahoo devuelve más de un resultado, las claves del último sobrescriben a las anteriores).

Errores:
- Error de red/HTTP de `doGet`.
- JSON inválido al parsear la respuesta.
- `quoteSummary.error` no nulo en la respuesta de Yahoo (ticker inexistente, módulo inválido, etc.).

Si `result` viene vacío, devuelve un mapa vacío (no es un error).

Módulos comunes de Yahoo: `price`, `summaryDetail`, `defaultKeyStatistics`, `financialData`, `earnings`, `earningsTrend`, `recommendationTrend`, `assetProfile`, `balanceSheetHistory`, `incomeStatementHistory`, `cashflowStatementHistory`, entre otros definidos por la API no oficial de Yahoo.

```go
data, err := client.QuoteSummary("AAPL", []string{"price", "summaryDetail"})
if err != nil {
    log.Fatal(err)
}
price := gohoo.Field(data["price"].(map[string]interface{}), "regularMarketPrice")
```

### FundamentalsTimeseries

```go
func (c *Client) FundamentalsTimeseries(ticker string, types []string) (map[string]interface{}, error)
```

Llama a `ws/fundamentals-timeseries/v1/finance/timeseries/{ticker}` pidiendo los `type` indicados, con una ventana fija de `period1`/`period2` de **6 años hacia atrás desde ahora** hasta hoy (no configurable). Devuelve el JSON crudo de Yahoo sin aplanar, ya que su forma varía según los `type` pedidos.

Tipos típicos: `annualTotalRevenue`, `quarterlyTotalRevenue`, `annualNetIncome`, `quarterlyNetIncome`, `annualFreeCashFlow`, `trailingFreeCashFlow`, etc. (prefijo `annual`/`quarterly`/`trailing` + el nombre del concepto financiero).

```go
data, err := client.FundamentalsTimeseries("AAPL", []string{"annualTotalRevenue", "quarterlyNetIncome"})
```

### Chart

```go
func (c *Client) Chart(ticker, rango, intervalo string) (map[string]interface{}, error)
```

Llama a `v8/finance/chart/{ticker}` con `range`, `interval` y `events=div,splits`, devolviendo el JSON crudo de Yahoo (OHLCV + metadata + eventos). Para la mayoría de los usos conviene `ChartSeries`, que ya parsea esta respuesta.

Valores válidos de `rango`: `1d, 5d, 1mo, 3mo, 6mo, 1y, 2y, 5y, 10y, ytd, max`.
Valores válidos de `intervalo`: `1m, 2m, 5m, 15m, 30m, 60m/1h, 1d, 5d, 1wk, 1mo, 3mo`.

### ChartSeries

```go
func (c *Client) ChartSeries(ticker, rango, intervalo string) ([]OHLCV, map[string]float64, map[string]float64, error)
```

Llama a `Chart` y parsea la respuesta a:

1. `[]OHLCV` — velas ordenadas cronológicamente (orden ascendente, tal como las entrega Yahoo).
2. `dividends map[string]float64` — monto del dividendo indexado por fecha `"YYYY-MM-DD"`.
3. `splits map[string]float64` — ratio del split (`numerator/denominator`) indexado por fecha `"YYYY-MM-DD"`.

Para criptomonedas (`BTC-USD`, etc.) los mapas de dividendos/splits normalmente quedan vacíos, ya que Yahoo no reporta esos eventos para ese tipo de activo.

Errores: cualquier error de `Chart`, o `"respuesta de chart vacía o inválida para %s"` si falta `chart.result[0]`, `timestamp`, `indicators` o `indicators.quote[0]`.

```go
series, dividends, splits, err := client.ChartSeries("AAPL", "1y", "1d")
if err != nil {
    log.Fatal(err)
}
last := series[len(series)-1]
fmt.Println(last.Time, last.Close)
```

### NewClientWithOptions

```go
func NewClientWithOptions(opts ...Option) (*Client, error)

func WithHTTPClient(h *http.Client) Option
func WithBaseURL(u string) Option
func WithUserAgent(ua string) Option
func WithRetryPolicy(p RetryPolicy) Option
func WithLazyBootstrap() Option
```

Constructor configurable. Sin opciones es idéntico a `NewClient()` (que de hecho delega en él).

| Opción | Efecto | Por defecto |
| --- | --- | --- |
| `WithHTTPClient(h)` | Usa `h` para todas las requests. Si `h` no trae `Jar`, se usa una copia de `h` con un cookie jar nuevo (el crumb depende de las cookies); `h` no se modifica. | `http.Client` con timeout de 20 s |
| `WithBaseURL(u)` | Host de `chart`, `quoteSummary`, `fundamentals-timeseries` y `getcrumb` (p. ej. `https://query1.finance.yahoo.com` o un `httptest.Server`). Error si `u` no tiene esquema y host. | `https://query2.finance.yahoo.com` |
| `WithUserAgent(ua)` | Header `User-Agent` de todas las requests. | Chrome/Windows de escritorio |
| `WithRetryPolicy(p)` | Política de reintentos (ver [RetryPolicy](#retrypolicy)). | `DefaultRetryPolicy()` |
| `WithLazyBootstrap()` | El constructor no hace red; cookie+crumb se piden la primera vez que hacen falta (`QuoteSummary`) o tras un 401. `Chart*` nunca los exige. | Bootstrap inmediato |

```go
client, err := gohoo.NewClientWithOptions(
    gohoo.WithLazyBootstrap(),
    gohoo.WithBaseURL("https://query1.finance.yahoo.com"),
    gohoo.WithRetryPolicy(gohoo.RetryPolicy{MaxAttempts: 1, RetryRateLimit: false}),
)
```

### Variantes con context

```go
func (c *Client) QuoteSummaryContext(ctx context.Context, ticker string, modules []string) (map[string]interface{}, error)
func (c *Client) FundamentalsTimeseriesContext(ctx context.Context, ticker string, types []string) (map[string]interface{}, error)
func (c *Client) ChartContext(ctx context.Context, ticker, rango, intervalo string) (map[string]interface{}, error)
func (c *Client) ChartSeriesContext(ctx context.Context, ticker, rango, intervalo string) ([]OHLCV, map[string]float64, map[string]float64, error)
```

Igual que los métodos sin sufijo (que delegan en estas con `context.Background()`), pero la request y la espera entre reintentos se cortan cuando `ctx` termina: el método vuelve de inmediato, sin consumir los intentos restantes, con un error que cumple `errors.Is(err, context.Canceled)` (o `context.DeadlineExceeded`) y que además envuelve el último error HTTP.

### ChartRange

```go
func (c *Client) ChartRange(ctx context.Context, ticker string, p ChartParams) (*ChartResult, error)
```

Llama a `v8/finance/chart/{ticker}` por rango relativo (`p.Range`) o por fechas (`p.Start`/`p.End` → `period1`/`period2`) y devuelve un resultado tipado ([ChartResult](#chartparams-chartresult-chartmeta-bar)) con `float64` en lugar de `interface{}` y las barras en la zona horaria del exchange.

Validación (antes de tocar la red): `Range` y `Start` son excluyentes; hace falta uno de los dos; `End` requiere `Start` y debe ser posterior. `End` cero = ahora; `Interval` vacío = `1d`; `Events` vacío = `div,splits`.

A diferencia de `Chart`, un `{"chart":{"error":{...}}}` de Yahoo (con status 200 o 404) se devuelve como error: `errors.Is(err, ErrNotFound)` para símbolos sin datos y `errors.Is(err, ErrRateLimit)` para "Too Many Requests".

```go
res, err := client.ChartRange(ctx, "SPY", gohoo.ChartParams{
    Start:    time.Date(2015, 1, 1, 0, 0, 0, 0, time.UTC),
    Interval: "1mo",
})
```

### ChartResult.Monthly

```go
func (r *ChartResult) Monthly() (dates []time.Time, adj []float64)
```

Una observación por mes calendario, en orden cronológico: la fecha es el día 1 a medianoche **UTC** del mes en que cae la barra **según la zona del exchange**, y el valor es su `AdjClose`. Si hay dos barras en el mismo mes (la mensual y la "en vivo" del mes en curso), gana la más reciente (a igual hora, la que aparece después en `Bars`). Es una función pura sobre `Bars` (no hace red).

Usar la zona del exchange importa: la barra mensual de marzo de Tokio tiene timestamp `2024-02-29 15:00 UTC` (medianoche JST); en UTC caería en febrero.

> **Windows:** si la máquina no tiene Go instalado, `time.LoadLocation` no encuentra la base de zonas y `Meta.Location` queda en UTC. La aplicación debe importar `_ "time/tzdata"`; `gohoo` no lo hace para no sumar ~450 KB a todos los binarios.

### GetJSON y PostJSON

```go
func (c *Client) GetJSON(ctx context.Context, path string, q url.Values) ([]byte, error)
func (c *Client) PostJSON(ctx context.Context, path string, q url.Values, body any) ([]byte, error)
```

Petición autenticada a cualquier endpoint de Yahoo Finance que gohoo todavía no cubre; devuelve el cuerpo tal cual. `path` es relativo al host del cliente (p. ej. `/v7/finance/quote`; ver `WithBaseURL`) o una URL absoluta, y `q` se une a la query que traiga `path`. El crumb vigente se agrega como `crumb` en cada intento (reemplaza cualquier `crumb` de `q`). Usa la misma sesión, `RetryPolicy`, cancelación y errores tipados que el resto de los métodos; ante un HTTP 401 renueva el crumb y reintenta. `PostJSON` serializa `body` con `encoding/json` (`Content-Type: application/json`); con `body == nil` no manda cuerpo.

```go
raw, err := client.GetJSON(ctx, "/v7/finance/quote", url.Values{"symbols": {"AAPL,MSFT"}})
```

### Screen y ScreenPredefined

```go
func (c *Client) Screen(ctx context.Context, p ScreenParams) (*ScreenResult, error)
func (c *Client) ScreenPredefined(ctx context.Context, id string, offset, size int) (*ScreenResult, error)

func Eq(field string, v any) Query
func Gt(field string, v float64) Query // también Lt, Gte, Lte
func Btwn(field string, lo, hi float64) Query
func IsIn(field string, vs ...any) Query // Or de Eq
func And(qs ...Query) Query
func Or(qs ...Query) Query

type ScreenParams struct {
    Query     Query  // obligatoria
    QuoteType string // "EQUITY" (por defecto), "ETF", "MUTUALFUND"...
    Offset    int
    Size      int    // 0 = 25; máximo ScreenMaxSize (250)
    SortField string // "" = "ticker"
    SortAsc   bool   // por defecto descendente
}

type ScreenResult struct {
    Start, Count, Total int
    Quotes              []ScreenQuote
}

type ScreenQuote struct {
    Symbol, ShortName, LongName, QuoteType string
    Exchange, FullExchangeName, Region     string
    Currency                               string
    MarketCap, Price                       float64 // Price = regularMarketPrice
    Raw                                    map[string]any
}
```

`Screen` consulta el screener de Yahoo (ver [ENDPOINTS.md](ENDPOINTS.md)); `ScreenPredefined`, uno de los screeners guardados de Yahoo (`day_gainers`, `day_losers`, `most_actives`, `undervalued_large_caps`, `aggressive_small_caps`, ...). Los números de `Raw` ya vienen sin el envoltorio `{raw, fmt}`.

- Validación antes de la red: `Query` no vacía, `0 <= Size <= 250`, `Offset >= 0`, `id` no vacío.
- Una query válida sin coincidencias devuelve un `ScreenResult` con `Total` 0, **sin error**.
- `SortField` debe existir para el `QuoteType`: `intradaymarketcap` vale para `EQUITY` pero no para `ETF` (ahí, p. ej., `fundnetassets`).
- Un error de Yahoo en el cuerpo (`finance.error`, p. ej. operador o campo inválido) se devuelve con su código y descripción; un id predefinido inexistente cumple `errors.Is(err, ErrNotFound)` y "too many requests", `errors.Is(err, ErrRateLimit)`. Una respuesta sin bloque de resultados es `ErrEmptyScreen`.
- Para paginar: repetir con `Offset += Size` mientras `Offset < Total`.

```go
res, err := client.Screen(ctx, gohoo.ScreenParams{
    Query:     gohoo.And(gohoo.Eq("region", "us"), gohoo.Gte("intradaymarketcap", 200e9)),
    Size:      100,
    SortField: "intradaymarketcap",
})
for _, q := range res.Quotes {
    fmt.Println(q.Symbol, q.MarketCap, q.Raw["trailingPE"])
}
```

### Quotes

```go
const QuoteBatchSize = 100

func (c *Client) Quotes(ctx context.Context, symbols []string) ([]Quote, error)

type Quote struct {
    Symbol, ShortName, LongName, QuoteType string // QuoteType: EQUITY, ETF, CURRENCY, INDEX, CRYPTOCURRENCY...
    Exchange, FullExchangeName, Currency    string
    MarketState                             string // REGULAR, PRE, POST, CLOSED...
    Price, PreviousClose, Change            float64 // regularMarket*
    ChangePercent                           float64 // en porcentaje
    Open, DayHigh, DayLow, Volume           float64
    MarketCap                               float64 // en Currency; 0 en índices, monedas y la mayoría de los ETF
    Time                                    time.Time // regularMarketTime en la zona del exchange
    Raw                                     map[string]any
}
```

Cotización actual de muchos símbolos con pocas requests: llama a `/v7/finance/quote` en lotes de `QuoteBatchSize` (ver [ENDPOINTS.md](ENDPOINTS.md)). Sirve para acciones, ETFs, índices (`^VIX`), monedas (`CLP=X`) y cripto (`BTC-USD`).

- Los símbolos vacíos y repetidos se descartan; los que Yahoo no conoce no aparecen en el resultado (no es un error).
- El orden es el de Yahoo: para buscar por símbolo, indexar por `Quote.Symbol`.
- Si un lote falla, devuelve las cotizaciones de los lotes anteriores junto con el error.
- `MarketCap` viene en la moneda de cotización (p. ej. CLP para `SQM-B.SN`), igual que `price.marketCap` de `quoteSummary`.

```go
qs, err := client.Quotes(ctx, []string{"AAPL", "SQM-B.SN", "CLP=X", "^VIX"})
for _, q := range qs {
    fmt.Println(q.Symbol, q.Price, q.Currency, q.MarketCap)
}
```

### WithLimiter y WithRequestHook

```go
type Limiter interface{ Wait(ctx context.Context) error } // golang.org/x/time/rate.Limiter lo cumple
type LimiterFunc func(ctx context.Context) error

func WithLimiter(l Limiter) Option
func WithRequestHook(fn func(RequestEvent)) Option

type RequestEvent struct {
    Method     string
    Endpoint   string // chart, quoteSummary, fundamentals, quote, screener, getcrumb, consent, other
    Symbol     string // ticker de la URL (en quote, la lista; en un screener predefinido, su id)
    URL        string // sin el crumb
    Attempt    int    // 1 = primer intento de la llamada
    StatusCode int    // 0 si no hubo respuesta
    Duration   time.Duration
    Err        error  // nil si respondió 200; *StatusError si no
}
```

Ganchos para aplicar una política de tráfico propia **dentro** del cliente, sobre cada request HTTP (incluidos los reintentos y el bootstrap de cookie+crumb):

- `WithLimiter`: `Wait` se llama antes de cada request; sirve para espaciar las requests de todo un proceso (un mismo `Limiter` puede compartirse entre clientes). Si devuelve error, la llamada termina con ese error sin reintentar.
- `WithRequestHook`: `fn` se llama después de cada request con su resultado, para métricas o logs (p. ej. contar los 429). Se llama desde la goroutine de la request: debe ser seguro para uso concurrente y rápido.

```go
var n429 atomic.Int64
client, err := gohoo.NewClientWithOptions(
    gohoo.WithLimiter(rate.NewLimiter(rate.Every(350*time.Millisecond), 1)),
    gohoo.WithRequestHook(func(ev gohoo.RequestEvent) {
        if ev.StatusCode == http.StatusTooManyRequests {
            n429.Add(1)
        }
    }),
)
```

### Info y FlattenInfo

```go
func (c *Client) Info(ctx context.Context, ticker string, opts ...InfoOption) (map[string]any, error)
func FlattenInfo(qs map[string]any, opts ...InfoOption) map[string]any
func InfoModules() []string // summaryDetail, price, defaultKeyStatistics, financialData, assetProfile, quoteType

func InfoWithModules(modules ...string) InfoOption // módulos a pedir/aplanar y su prioridad
func InfoPercentYield() InfoOption                 // dividendYield en %, con "yield" de respaldo (ETF)
func InfoWithTrailingPEG() InfoOption              // agrega trailingPegRatio (una request más; solo Info)
```

Un mapa plano con los datos del ticker, al estilo de `Ticker.info` de yfinance. `FlattenInfo` es la parte pura (sobre una respuesta de `QuoteSummary`); `Info` pide `quoteSummary` y aplana.

- Los módulos se recorren en orden de prioridad y **la primera aparición de cada clave gana**. `summaryDetail` va primero porque algunos campos de `defaultKeyStatistics` vienen corruptos en ciertas acciones (p. ej. `forwardPE` de 11836 en vez de 11,2 en `COPEC.SN`).
- Los números quedan sin el envoltorio `{raw, fmt}`; los `null` y los objetos sin `raw` (p. ej. el `{}` que trae `dividendYield` en los ETF) se omiten. Los módulos de la respuesta que no están en la lista se ignoran.
- Sin opciones, los valores quedan tal como los manda Yahoo (`dividendYield` en fracción: 0,0279). `InfoPercentYield` los lleva a porcentaje (2,79), como yfinance.
- Con `InfoWithTrailingPEG`, si la request de fundamentals falla o no hay dato, la clave no se agrega e `Info` no falla.

```go
info, err := client.Info(ctx, "AAPL", gohoo.InfoPercentYield(), gohoo.InfoWithTrailingPEG())
pe, _ := gohoo.AsFloat(info["trailingPE"])
```

### TimeseriesPoints y TrailingPEG

```go
type Point struct {
    Date       string  // "YYYY-MM-DD"
    PeriodType string  // "3M", "12M", "TTM"...
    Currency   string  // vacío en ratios
    Value      float64
}

func TimeseriesPoints(data map[string]any, typ string) []Point
func (c *Client) TrailingPEG(ctx context.Context, ticker string) (v float64, ok bool, err error)
```

`TimeseriesPoints` extrae de una respuesta de `FundamentalsTimeseries` las observaciones de un tipo, **la más reciente primero**, sin las que no traen fecha o valor (`nil` si el tipo no está). `TrailingPEG` devuelve el último `trailingPegRatio`, que Yahoo publica en fundamentals-timeseries y no en `quoteSummary`; `ok` es false si no hay dato.

```go
data, _ := client.FundamentalsTimeseriesContext(ctx, "AAPL", []string{"quarterlyTotalRevenue"})
for _, p := range gohoo.TimeseriesPoints(data, "quarterlyTotalRevenue") {
    fmt.Println(p.Date, p.Value, p.Currency)
}
```

### AutoAdjust, Bar.Date y AtMidnight

```go
func AutoAdjust(bars []Bar) []Bar
func (r *ChartResult) AutoAdjusted() []Bar // AutoAdjust(r.Bars)
func (b Bar) Date() time.Time              // medianoche del día de b.Time, en su zona
func AtMidnight(bars []Bar) []Bar
```

`AutoAdjust` equivale a `auto_adjust=True` de yfinance: `Close = AdjClose` y `Open`, `High` y `Low` se escalan con el mismo factor `AdjClose/Close` (factor 1 si `Close` es 0 o NaN, o si `AdjClose` es NaN); `Time` y `Volume` no cambian. `AtMidnight` lleva cada `Time` a `Date()` para indexar velas diarias, semanales o mensuales por fecha, como yfinance (no usar con velas intradía). Ninguna modifica su entrada.

```go
bars := gohoo.AtMidnight(res.AutoAdjusted())
```

---

## Indicadores técnicos

```go
func ComputeIndicators(series []OHLCV) []IndicatorSet
```

Calcula SMA20, EMA12/26, RSI14, MACD(12,26,9), Bandas de Bollinger(20,2), ATR14, Estocástico(14,3), Williams %R14, ADX14, VWAP (anclado al inicio de la serie), OBV y SMA20 de volumen, más las señales `CrossSignal`, `RSISignal` y `BBSignal`. La serie debe estar ordenada cronológicamente (como la devuelve `ChartSeries`). El resultado está alineado por índice con la entrada; los valores sin historia suficiente son `math.NaN()`, y las señales que no aplican son `""`.

```go
series, _, _, _ := client.ChartSeries("AAPL", "1y", "1d")
ind := gohoo.ComputeIndicators(series)
fmt.Println(ind[len(ind)-1].RSI14)
```

### Sobre barras tipadas y con períodos configurables

```go
func ComputeIndicatorsBars(bars []Bar) []IndicatorSet // lo mismo que ComputeIndicators, desde []Bar

func Opens(bars []Bar) []float64 // también Highs, Lows, Closes, AdjCloses, Volumes

func SMA(values []float64, period int) []float64
func EMA(values []float64, period int) []float64
func RSI(closes []float64, period int) []float64
func MACD(closes []float64, fast, slow, signal int) (line, sig, hist []float64)
func Bollinger(closes []float64, period int, numStdDev float64) (upper, middle, lower []float64)
func ATR(highs, lows, closes []float64, period int) []float64
func Stochastic(highs, lows, closes []float64, kPeriod, dPeriod int) (k, d []float64)
func WilliamsR(highs, lows, closes []float64, period int) []float64
func ADX(highs, lows, closes []float64, period int) []float64
func OBV(closes, volumes []float64) []float64
func VWAP(highs, lows, closes, volumes []float64) []float64
func Crossovers(fast, slow []float64) []int // +1 cruce hacia arriba, -1 hacia abajo, 0 sin cruce
```

`ComputeIndicatorsBars` da exactamente el mismo `IndicatorSet` que `ComputeIndicators` para los mismos datos; usa los precios tal cual vienen (para precios ajustados, pasar `AutoAdjust(bars)`). Las funciones sueltas son los mismos cálculos con el período que se quiera: `SMA(closes, 20)` coincide con `IndicatorSet.SMA20`, `MACD(closes, 12, 26, 9)` con `MACD`/`MACDSignal`/`MACDHist`, etc.

- El resultado está alineado por índice con la entrada; los puntos sin historia suficiente son `math.NaN()`.
- Un período < 1 devuelve todo NaN; si las columnas no tienen el mismo largo, el resultado es `nil`.
- Un NaN en la entrada se propaga: en las medias móviles, a cada ventana que lo contiene; en las que acumulan (EMA, RSI, ATR, ADX, MACD, OBV, VWAP), a todos los puntos siguientes.

```go
bars := res.AutoAdjusted()
closes := gohoo.Closes(bars)
cruces := gohoo.Crossovers(gohoo.SMA(closes, 50), gohoo.SMA(closes, 200)) // golden/death cross 50/200
rsi7 := gohoo.RSI(closes, 7)
```

### Métricas de riesgo

```go
func Returns(values []float64) []float64    // v[i]/v[i-1] - 1; el primero es NaN
func LogReturns(values []float64) []float64 // ln(v[i]/v[i-1])
func Volatility(returns []float64, periodsPerYear float64) float64
func MaxDrawdown(values []float64) (dd float64, peak, trough int)
```

- `Returns` / `LogReturns`: alineados con la entrada; NaN en el primero y donde el valor o el anterior son NaN o el anterior es 0.
- `Volatility`: desviación estándar muestral (n-1) de los retornos no NaN, anualizada con `sqrt(periodsPerYear)` (252 diarios, 52 semanales, 12 mensuales; 1 para no anualizar). NaN con menos de dos retornos.
- `MaxDrawdown`: mayor caída desde un máximo previo como fracción ≤ 0 (-0,25 = -25 %), con los índices del máximo y del mínimo; ignora NaN. Sin caídas, `(0, -1, -1)`.

```go
closes := gohoo.Closes(res.AutoAdjusted())
vol := gohoo.Volatility(gohoo.Returns(closes), 252)
dd, desde, hasta := gohoo.MaxDrawdown(closes)
```

## Detección de huecos

```go
func DetectGaps(series []OHLCV, factor float64) []Gap
```

Devuelve los huecos entre barras consecutivas mayores a `factor` veces el intervalo típico (mediana de los deltas). `factor = 5` es un buen punto de partida para velas diarias. `Gap` tiene `From`, `To`, `Esperado` y `Real`.

## Tipos

### OHLCV

```go
type OHLCV struct {
    Time                                      time.Time
    Open, High, Low, Close, AdjClose, Volume interface{}
}
```

Una barra de precio/volumen de `chart`. `Time` es siempre UTC (el timestamp Unix de Yahoo convertido con `time.Unix(...).UTC()`). Los campos numéricos son `interface{}` (normalmente `float64`, y `Volume` puede venir como entero) porque Yahoo puede devolver `null` para velas sin datos (mercado cerrado, activo recién listado, etc.); usar [`AsFloat`](#asfloat) para convertirlos de forma segura.

```go
func FirstResult(chart map[string]interface{}) (map[string]interface{}, bool)
```

Extrae `chart["chart"]["result"][0]` de la respuesta cruda de `Chart`, devolviendo `false` si la forma no coincide con lo esperado. Lo usa `ChartSeries` internamente; exportada por si se necesita trabajar con la respuesta cruda de `Chart` directamente.

### ChartParams, ChartResult, ChartMeta, Bar

```go
type ChartParams struct {
    Range    string    // "5y", "max"...; excluyente con Start/End
    Start    time.Time // period1
    End      time.Time // period2; cero = ahora
    Interval string    // vacío = "1d"
    Events   string    // vacío = "div,splits"
}

type ChartResult struct {
    Meta      ChartMeta
    Bars      []Bar              // orden cronológico
    Dividends map[string]float64 // por fecha "YYYY-MM-DD", igual que ChartSeries
    Splits    map[string]float64
}

type ChartMeta struct {
    Symbol, Currency, InstrumentType string
    ExchangeTimezoneName             string
    Location                         *time.Location // UTC si no se pudo cargar
}

type Bar struct {
    Time                                     time.Time // en Meta.Location
    Open, High, Low, Close, AdjClose, Volume float64
}
```

Criterio para los `null` de Yahoo en `Bar`: si la respuesta trae la serie `adjclose`, la barra se **descarta** cuando su `AdjClose` es `null` (nunca se mezcla un cierre sin ajustar en una serie ajustada); si la respuesta no trae `adjclose` en absoluto, `AdjClose = Close` para toda la serie y se descartan las barras con `Close` `null`. Cualquier otro campo `null` queda en `math.NaN()` (nunca en `0`). `OHLCV.Time` sigue en UTC; `Bar.Time` está en la zona del exchange.

### RetryPolicy

```go
type RetryPolicy struct {
    MaxAttempts     int           // total de intentos; <= 0 = 4; 1 = sin reintentos
    BaseDelay       time.Duration // <= 0 = 200ms; se duplica en cada reintento
    MaxDelay        time.Duration // tope de cada espera; 0 = sin tope
    Jitter          float64       // 0..1: suma hasta Jitter*retardo aleatorio
    RetryRateLimit  bool          // reintentar HTTP 429
    HonorRetryAfter bool          // esperar al menos lo que pida Retry-After
    OnRetry         func(attempt int, err error, delay time.Duration)
    RateLimit       *RateLimitPolicy // nil = los 429 se tratan con los campos de arriba
}

type RateLimitPolicy struct {
    MaxRetries      int           // reintentos ante rate limit; 0 = devolverlo de inmediato
    BaseDelay       time.Duration // espera antes del primer reintento; se duplica
    MaxDelay        time.Duration // tope de la parte exponencial; 0 = sin tope
    Jitter          time.Duration // + espera aleatoria entre 0 y Jitter
    HonorRetryAfter bool          // esperar al menos Retry-After (sin acotar)
}

func DefaultRetryPolicy() RetryPolicy // 4 intentos, 200ms, RetryRateLimit: true

func WithRetryPolicyContext(ctx context.Context, p RetryPolicy) context.Context
func RetryPolicyFromContext(ctx context.Context) (RetryPolicy, bool)
```

Los booleanos se toman tal cual, así que para cambiar un solo campo conviene partir de `DefaultRetryPolicy()`. `OnRetry` recibe el número del intento que falló (1 = el primero). El jitter usa un generador propio del cliente, no el global.

**`RateLimit`** separa los rate limits del resto de las fallas: con él, un HTTP 429 (o un "too many requests" informado en el cuerpo de `ChartRange`, `Screen` o `Quotes` con status 200) se reintenta hasta `MaxRetries` veces **sin consumir `MaxAttempts`**, esperando `min(BaseDelay·2^(n-1), MaxDelay) + U(0, Jitter)` (y al menos `Retry-After` si `HonorRetryAfter`). `RetryRateLimit`, `BaseDelay`, `MaxDelay` y `Jitter` de `RetryPolicy` dejan de aplicarse a los 429. Agotado el presupuesto, el error es `rate limit: agotados N reintentos: ...` y cumple `errors.Is(err, ErrRateLimit)`.

**`WithRetryPolicyContext`** fija la política de las llamadas hechas con ese contexto, en lugar de la del cliente: por ejemplo, reintentos pacientes en un proceso batch y ninguna espera larga dentro de un request web, sobre el mismo `Client`.

```go
batch := gohoo.RetryPolicy{
    MaxAttempts: 3, BaseDelay: 200 * time.Millisecond,
    RateLimit:   &gohoo.RateLimitPolicy{MaxRetries: 3, BaseDelay: 5 * time.Second, MaxDelay: time.Minute, Jitter: time.Second, HonorRetryAfter: true},
}
client, _ := gohoo.NewClientWithOptions(gohoo.WithRetryPolicy(batch))

web := batch
web.RateLimit = &gohoo.RateLimitPolicy{} // el 429 se devuelve de inmediato
res, err := client.ChartRange(gohoo.WithRetryPolicyContext(r.Context(), web), "AAPL", gohoo.ChartParams{Range: "1y"})
```

---

## Helpers de parseo JSON

Casi todos los valores numéricos que devuelve Yahoo en `quoteSummary`/`fundamentals-timeseries` vienen envueltos como `{"raw": 123.45, "fmt": "123.45"}`. Estos helpers evitan tener que desenvolver ese patrón a mano.

### RawValue

```go
func RawValue(v interface{}) interface{}
```

Si `v` es un `map[string]interface{}` con clave `"raw"`, devuelve ese valor. Si es un mapa sin `"raw"`, devuelve `nil`. Si `v` ya es un escalar (string, bool, número), lo devuelve tal cual. Si `v` es `nil`, devuelve `nil`.

### GetPath

```go
func GetPath(m map[string]interface{}, keys ...string) interface{}
```

Navega un árbol de `map[string]interface{}` siguiendo `keys` en orden, devolviendo `nil` en cuanto una clave no existe o el valor intermedio deja de ser un mapa. Útil para bajar varios niveles sin encadenar type-assertions manuales.

```go
peRatio := gohoo.GetPath(data, "summaryDetail", "trailingPE")
```

### Field

```go
func Field(m map[string]interface{}, key string) interface{}
```

Atajo equivalente a `RawValue(m[key])`, con `nil`-check sobre `m`. Pensado para leer un campo de un módulo ya desenvuelto (p. ej. `data["price"].(map[string]interface{})`).

### AsFloat

```go
func AsFloat(v interface{}) (float64, bool)
```

Convierte `float64`, `int`, `int64` o `string` (parseable como número) a `float64`. Devuelve `(0, false)` para cualquier otro tipo o string no numérico.

### AsInt64

```go
func AsInt64(v interface{}) (int64, bool)
```

Igual que `AsFloat` pero trunca a `int64` (vía conversión directa de `float64` a `int64`, sin redondeo).

### AsString

```go
func AsString(v interface{}) string
```

Convierte cualquier valor a su representación en string: `nil` → `""`, `string` se devuelve igual, `float64` se formatea sin notación científica (`strconv.FormatFloat(t, 'f', -1, 64)`), `bool` vía `strconv.FormatBool`, y cualquier otro tipo vía `fmt.Sprintf("%v", t)`.

---

## Helpers de fecha/hora

Yahoo reporta la mayoría de las fechas como timestamps Unix (segundos, UTC).

### EpochToDate

```go
func EpochToDate(v interface{}) string
```

Convierte un timestamp Unix a `"YYYY-MM-DD"` en UTC. Devuelve `""` si `v` no se puede convertir a número con `AsFloat`.

### EpochToDateTimeNY

```go
func EpochToDateTimeNY(v interface{}, loc *time.Location) string
```

Convierte un timestamp Unix a `"YYYY-MM-DD HH:MM:SS-07:00"` en la zona horaria `loc` (pensado para `America/New_York`, zona de las fechas de resultados/earnings). Si `loc` es `nil`, usa UTC. Devuelve `""` si `v` no es convertible.

```go
ny, _ := time.LoadLocation("America/New_York")
fecha := gohoo.EpochToDateTimeNY(earningsDate, ny)
```

### EpochToMidnight

```go
func EpochToMidnight(v interface{}) string
```

Da formato `"YYYY-MM-DD 00:00:00"` a un timestamp Unix (siempre medianoche), útil para columnas de período de estados financieros donde solo importa la fecha. Internamente usa `EpochToDate` + sufijo fijo.

### DateStringToMidnight

```go
func DateStringToMidnight(s string) string
```

Igual que `EpochToMidnight` pero partiendo de un string `"YYYY-MM-DD"` en vez de un epoch. Devuelve `""` si `s` está vacío.

### ParseDateOnly

```go
func ParseDateOnly(s string) (time.Time, error)
```

Parsea un string `"YYYY-MM-DD"` (layout `"2006-01-02"`) a `time.Time`.

---

## Manejo de errores

Todas las funciones del `Client` devuelven errores envueltos con contexto (`fmt.Errorf("...: %w", err)`), así que se pueden inspeccionar con `errors.Is`/`errors.As` si el error subyacente lo soporta (p. ej. errores de `net/http`). Casos típicos a manejar:

- **HTTP != 200**: el mensaje incluye el status code, la URL y los primeros 300 bytes del body.
- **Crumb inválido** (`NewClient`): Yahoo cambió su mecanismo de auth o bloqueó la IP.
- **Error de negocio de Yahoo** (`QuoteSummary`): `quoteSummary.error` no nulo, p. ej. ticker inexistente.
- **Respuesta vacía/inválida** (`ChartSeries`): ticker sin histórico para el rango pedido, o símbolo inexistente.

### Errores tipados

```go
var (
    ErrRateLimit    error // HTTP 429, o chart.error "Too Many Requests" (ChartRange)
    ErrUnauthorized error // HTTP 401 que persistió tras renovar el crumb
    ErrNotFound     error // HTTP 404, o chart.error "Not Found" (ChartRange)
    ErrEmptyScreen  error // respuesta del screener sin bloque de resultados
)

type StatusError struct {
    StatusCode int
    URL        string
    Body       string        // primeros 300 bytes
    RetryAfter time.Duration // header Retry-After; 0 si no vino
}

func IsRateLimit(err error) bool // errors.Is(err, ErrRateLimit)
```

Toda respuesta HTTP distinta de 200 es un `*StatusError` (se obtiene con `errors.As`) y se compara con los sentinelas con `errors.Is`, también cuando viene envuelta en `agotados N intentos: ...`:

```go
_, err := client.Chart("AAPL", "1y", "1d")
if gohoo.IsRateLimit(err) {
    var se *gohoo.StatusError
    if errors.As(err, &se) && se.RetryAfter > 0 {
        time.Sleep(se.RetryAfter)
    }
}
```

### Reintentos

Con la política por defecto, el cliente reintenta automáticamente (hasta 4 intentos en total, con backoff exponencial de 200ms, 400ms, 800ms) ante errores de red, HTTP 429 y HTTP 5xx. Ante un HTTP 401 renueva cookies+crumb y reintenta. Otros códigos (p. ej. 404) se devuelven de inmediato. Si se agotan los intentos, el error es `agotados 4 intentos: ...` y envuelve el último error. Un rate limit sostenido igual puede fallar: espaciar las llamadas, o usar `WithRetryPolicy` para no reintentar 429 y aplicar un cooldown propio.

## Límites y particularidades de Yahoo Finance

- No requiere API key: solo cookies de sesión + crumb público, obtenidos automáticamente en `NewClient`.
- Yahoo limita cuánto histórico intradía entrega (p. ej. `interval=1m` solo cubre los últimos ~7 días, independientemente del `range` pedido).
- `FundamentalsTimeseries` siempre pide una ventana de 6 años; para históricos más largos hay que paginar manualmente ajustando `period1`/`period2` fuera de esta librería (actualmente no configurable vía parámetros).
- Mismo endpoint `chart` sirve acciones (`AAPL`), ETFs (`SPY`) y criptomonedas (`BTC-USD`); solo cambia qué eventos (`dividends`/`splits`) trae cada uno.
- El `User-Agent` por defecto es un string de Chrome/Windows de escritorio (ver `client.go`); si Yahoo empieza a bloquear ese UA, se puede cambiar con `WithUserAgent`.

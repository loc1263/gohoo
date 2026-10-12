# gohoo v0.3+: diseño de las extensiones para uso general

Fecha: 2026-10-12

## 1. Objetivo

Que los proyectos que usan `gohoo` (Fog Radar Quant y otros) dejen de mantener su propia capa sobre Yahoo:
sesión duplicada para endpoints nuevos, screener, cotizaciones por lote, aplanado de `info` y control de tráfico
envolviendo cada llamada desde fuera. Todo con las mismas reglas de `DISENO_EXTENSIONES.md` §2: **solo cambios
aditivos**, mismos valores por defecto, sin dependencias, pruebas sin red y `apidiff` en CI.

## 2. Brechas y plan por versiones

| # | Extensión | Qué reemplaza en los proyectos | Versión |
|---|---|---|---|
| 1 | `GetJSON` / `PostJSON`: petición autenticada genérica | Copias de la sesión cookie + crumb para llamar endpoints no cubiertos | **v0.3.0** |
| 2 | `Screen` / `ScreenPredefined` + constructor de queries | Screener propio (Fog Radar: `internal/yahoo/screener.go`) | **v0.3.0** |
| 3 | `Quotes` en lote (`/v7/finance/quote`) | Un `quoteSummary` por símbolo para market cap / precio spot | **v0.4.0** |
| 4 | `WithLimiter` / `WithRequestHook` | Envoltorios de tráfico que duplican las firmas del cliente | **v0.4.0** |
| 4b | `RetryPolicy.RateLimit` (presupuesto y backoff propios para 429) | Bucles de reintento ante 429 alrededor de cada llamada | **v0.4.0** |
| 8 | `WithRetryPolicyContext` (política por llamada) | Modos "interactivos" propios por `context` | **v0.4.0** (adelantado: lo necesita 4b) |
| 5 | `Info` / `FlattenInfo` (estilo yfinance, opciones explícitas) | Aplanado propio de `quoteSummary` | **v0.5.0** |
| 6 | `TimeseriesPoints` / `TrailingPEG` | Lectura propia de fundamentals-timeseries | **v0.5.0** |
| 7 | `Bar.Date()` / `ChartResult.AutoAdjusted()` | `AutoAdjust` propio | **v0.5.0** |
| 9 | Indicadores sobre `[]Bar`, con períodos configurables, y métricas de riesgo | `ComputeIndicators` solo acepta `[]OHLCV` con `interface{}` y períodos fijos | **v0.6.0** |
| 10 | Fixtures grabados, `-tags live`, validación de `Range`/`Interval`, CI con lint y Windows | — | continuo |

Las secciones 3 y 4 detallan v0.3.0, la 6 v0.4.0, la 7 v0.5.0 y la 8 v0.6.0; el resto se diseña en detalle al llegar a su versión.

## 3. Petición autenticada genérica (v0.3.0)

```go
func (c *Client) GetJSON(ctx context.Context, path string, q url.Values) ([]byte, error)
func (c *Client) PostJSON(ctx context.Context, path string, q url.Values, body any) ([]byte, error)
```

- `path` relativo al host del cliente (`WithBaseURL`) o URL absoluta; la query de `path` se une con `q`.
- El crumb vigente se agrega como `crumb` **en cada intento** (tras un 401 viaja el crumb renovado).
- Misma `RetryPolicy`, cancelación por `ctx`, errores `*StatusError` y bootstrap perezoso que el resto.
- Implementación: `doGet`/`doGetAuthed` pasan a delegar en `do`/`doAuthed` (cualquier método, cuerpo opcional
  que se relee en cada reintento). Hay una sola implementación de reintentos y renovación de crumb.
- `QuoteSummary`, `FundamentalsTimeseries` y `Chart*` **no** se migran a `GetJSON`: los dos últimos no mandan
  crumb y con `WithLazyBootstrap` no deben exigirlo; migrarlos cambiaría su comportamiento.

## 4. Screener (v0.3.0)

Formato verificado contra Yahoo real el 2026-10-12 (fixtures en `testdata/screener_*.json`).

```go
type Query struct{ /* no exportado */ }
func Eq(field string, v any) Query
func Gt, Lt, Gte, Lte(field string, v float64) Query
func Btwn(field string, lo, hi float64) Query
func IsIn(field string, vs ...any) Query // Or de Eq: Yahoo no tiene "is-in"
func And(qs ...Query) Query              // 1 condición = esa misma; 0 = query vacía
func Or(qs ...Query) Query

type ScreenParams struct {
    Query     Query
    QuoteType string // "EQUITY" por defecto
    Offset    int
    Size      int    // 0 = 25; máximo ScreenMaxSize (250)
    SortField string // "" = "ticker"
    SortAsc   bool
}
type ScreenQuote struct {
    Symbol, ShortName, LongName, QuoteType, Exchange, FullExchangeName, Region, Currency string
    MarketCap, Price float64
    Raw map[string]any
}
type ScreenResult struct{ Start, Count, Total int; Quotes []ScreenQuote }

func (c *Client) Screen(ctx context.Context, p ScreenParams) (*ScreenResult, error)
func (c *Client) ScreenPredefined(ctx context.Context, id string, offset, size int) (*ScreenResult, error)
```

Endpoints:

- `POST /v1/finance/screener?corsDomain=finance.yahoo.com&formatted=false&lang=en-US&region=US&crumb=…` con cuerpo
  `{offset, size, sortField, sortType, quoteType, query, userId:"", userIdType:"guid"}`.
- `GET /v1/finance/screener/predefined/saved?scrIds=<id>&count=<n>&offset=<n>&formatted=false&…&crumb=…`.

Comportamiento observado y cómo se refleja:

| Caso | Respuesta de Yahoo | gohoo |
|---|---|---|
| Query válida sin coincidencias (`region = zz`) | 200, `result[0]` con `total: 0, quotes: []` | `ScreenResult` vacío, **sin error** |
| Operador inválido / campo de orden inexistente | 400, `finance.error {code:"internal-error", …}` | error con código y descripción + `*StatusError` |
| Sin `sortField` en ETF | 400 (`intradaymarketcap` no es campo de ETF) | default `ticker`, como yfinance |
| Crumb vencido | 401 `Invalid Crumb` | renueva el crumb y reintenta (motor común) |
| Predefinido inexistente | 404, `finance.error {code:"Not Found"}` | `errors.Is(err, ErrNotFound)` |
| `finance.error` "too many…" | — | `errors.Is(err, ErrRateLimit)` |
| `result` vacío o nulo sin error | — | `ErrEmptyScreen` |

Los operadores se mandan en mayúsculas (como yfinance); Yahoo los acepta sin distinguir mayúsculas.
Validación previa a la red: query no vacía (tampoco anidada), `0 ≤ Size ≤ 250`, `Offset ≥ 0`, id no vacío.

## 5. Compatibilidad verificada en v0.3.0

- `apidiff -incompatible` contra v0.2.1: sin cambios incompatibles (solo símbolos agregados).
- Tests existentes sin modificar, en verde con `-race`.
- `go.mod` sin dependencias nuevas.

## 6. v0.4.0: cotizaciones en lote y control de tráfico

Objetivo: que un proyecto pueda aplicar su política de tráfico (espaciado global, contadores, backoff largo ante 429,
modo interactivo) **dentro** de gohoo, por intento HTTP, en vez de envolver cada llamada desde fuera.

### 6.1 `Quotes`

```go
const QuoteBatchSize = 100
type Quote struct {
    Symbol, ShortName, LongName, QuoteType, Exchange, FullExchangeName, Currency, MarketState string
    Price, PreviousClose, Change, ChangePercent, Open, DayHigh, DayLow, Volume, MarketCap float64
    Time time.Time // regularMarketTime en la zona del exchange
    Raw  map[string]any
}
func (c *Client) Quotes(ctx context.Context, symbols []string) ([]Quote, error)
```

- `GET /v7/finance/quote?symbols=A,B,…&formatted=false&crumb=…` (verificado el 2026-10-12: requiere crumb; 300 símbolos en
  una llamada funcionan; se usan lotes de 100 por prudencia con el largo de URL).
- Símbolos vacíos y repetidos se descartan; los desconocidos simplemente no vuelven (no es error).
- Si un lote falla se devuelven los lotes anteriores junto con el error (útil en pipelines).
- `MarketCap` está en `Currency` (p. ej. CLP para `.SN`), igual que `price.marketCap` de `quoteSummary`.

### 6.2 Ganchos

```go
type Limiter interface{ Wait(ctx context.Context) error }
type LimiterFunc func(ctx context.Context) error
func WithLimiter(l Limiter) Option
func WithRequestHook(fn func(RequestEvent)) Option
type RequestEvent struct {
    Method, Endpoint, Symbol, URL string // URL sin crumb
    Attempt, StatusCode int
    Duration time.Duration
    Err error
}
```

Ambos actúan sobre **cada** request HTTP: reintentos y bootstrap incluidos. Un error de `Limiter.Wait` termina la llamada sin
reintentar. `Endpoint` ∈ chart, quoteSummary, fundamentals, quote, screener, getcrumb, consent, other.

### 6.3 `RetryPolicy.RateLimit` y política por contexto

```go
type RetryPolicy struct { /* campos de siempre */ ; RateLimit *RateLimitPolicy }
type RateLimitPolicy struct {
    MaxRetries int
    BaseDelay, MaxDelay, Jitter time.Duration
    HonorRetryAfter bool
}
func WithRetryPolicyContext(ctx context.Context, p RetryPolicy) context.Context
func RetryPolicyFromContext(ctx context.Context) (RetryPolicy, bool)
```

- Con `RateLimit == nil` (por defecto) nada cambia: el 429 consume intentos de `MaxAttempts` o se devuelve si `RetryRateLimit`
  es false.
- Con `RateLimit`, los rate limits tienen su propio presupuesto (`MaxRetries`, no consume `MaxAttempts`) y espera
  `min(BaseDelay·2^(n-1), MaxDelay) + U(0, Jitter)`, y al menos `Retry-After` si `HonorRetryAfter` (sin acotar). También se
  reintentan los rate limits **informados en el cuerpo** con status 200 (`chart.error` / `finance.error` "too many…"), que
  antes solo se devolvían como `ErrRateLimit`. Esto último solo ocurre con `RateLimit` configurado, para no cambiar el defecto.
- La política del contexto reemplaza a la del cliente para esa llamada (p. ej. `RateLimit.MaxRetries = 0` dentro de un request web).

### 6.4 Compatibilidad verificada

`apidiff -incompatible` contra v0.2.1 y v0.3.0: sin cambios incompatibles. Los tests existentes no se modificaron.

## 7. v0.5.0: `Info`, fundamentals tipados y `auto_adjust`

Lógica que los proyectos replicaban sobre las respuestas crudas, ahora en la librería con tests sobre respuestas reales
(`testdata/quotesummary_copec_sn.json`, `quotesummary_spy.json`, `timeseries_aapl.json`).

- **`FlattenInfo(qs, opts...)` / `Client.Info`**: prioridad de módulos `InfoModules()` (una función, para que ningún paquete
  altere el valor por defecto de otro), primera aparición gana, `RawValue` en cada valor. Las transformaciones estilo yfinance
  son **opcionales**: `InfoPercentYield` (×100, con `yield` de respaldo) e `InfoWithTrailingPEG`. Sin opciones el resultado es
  el dato de Yahoo tal cual. `InfoWithModules` cambia la lista y su orden.
- **`TimeseriesPoints` / `Point` / `Client.TrailingPEG`**: más reciente primero; `Point` trae `PeriodType` y `Currency`.
- **`AutoAdjust` / `ChartResult.AutoAdjusted` / `Bar.Date` / `AtMidnight`**: el ajuste de precios y la normalización a
  fecha son funciones separadas (la segunda no sirve para velas intradía); un proyecto que quiera ambas las compone.
- Los proyectos que llaman a Yahoo a través de su propia interfaz (para probar con falsos) usan las funciones puras
  (`FlattenInfo`, `TimeseriesPoints`) sobre sus respuestas, en vez de los métodos del cliente.

`apidiff -incompatible` contra v0.2.1 y v0.4.0: sin cambios incompatibles.

## 8. v0.6.0: indicadores sobre `[]Bar` y métricas de riesgo

- **`ComputeIndicatorsBars([]Bar)`**: mismo `IndicatorSet` que `ComputeIndicators`. Ambas delegan en una implementación
  común sobre columnas; la salida de `ComputeIndicators` se comparó byte a byte contra v0.5.0 (120 barras, con un volumen nulo).
- **Indicadores sueltos con períodos configurables** sobre `[]float64` (`SMA`, `EMA`, `RSI`, `MACD`, `Bollinger`, `ATR`,
  `Stochastic`, `WilliamsR`, `ADX`, `OBV`, `VWAP`) y `Crossovers` con resultado numérico (+1/-1/0) en lugar de las
  señales en texto de `IndicatorSet`. Reglas uniformes: alineados con la entrada, NaN sin historia, período < 1 → todo
  NaN, largos distintos → nil.
- **Columnas** (`Opens`, `Highs`, `Lows`, `Closes`, `AdjCloses`, `Volumes`) para pasar de `[]Bar` a `[]float64`.
- **Métricas de riesgo**: `Returns`, `LogReturns`, `Volatility` (muestral, anualizada) y `MaxDrawdown` (con índices).

`apidiff -incompatible` contra v0.2.1 y v0.5.0: sin cambios incompatibles.

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
| 3 | `Quote` en lote (`/v7/finance/quote`) | Un `quoteSummary` por símbolo para market cap / precio spot | v0.4.0 |
| 4 | `WithLimiter` / `WithRequestHook` | Envoltorios de tráfico que duplican las firmas del cliente | v0.4.0 |
| 5 | `Info` / `FlattenInfo` (estilo yfinance, opciones explícitas) | Aplanado propio de `quoteSummary` | v0.4.0 |
| 6 | `TimeseriesPoints` / `TrailingPEG` | Lectura propia de fundamentals-timeseries | v0.5.0 |
| 7 | `Bar.Date()` / `ChartResult.AutoAdjusted()` | `AutoAdjust` propio | v0.5.0 |
| 8 | `WithRetryPolicyContext` (política por llamada) | Modos "interactivos" propios por `context` | v0.5.0 |
| 9 | Indicadores sobre `[]Bar` | `ComputeIndicators` solo acepta `[]OHLCV` con `interface{}` | v0.5.0 |
| 10 | Fixtures grabados, `-tags live`, validación de `Range`/`Interval`, CI con lint y Windows | — | continuo |

Las secciones 3 y 4 detallan lo implementado en v0.3.0; el resto se diseña en detalle al llegar a su versión.

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

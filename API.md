# API de gohoo

Referencia completa del paquete `yahoo` (módulo `github.com/loc1263/gohoo`). Para una introducción rápida ver [README.md](README.md). Para qué significa cada módulo/campo que devuelve Yahoo en cada endpoint, ver [ENDPOINTS.md](ENDPOINTS.md).

```go
import yahoo "github.com/loc1263/gohoo"
```

## Índice

- [Client](#client)
  - [NewClient](#newclient)
  - [QuoteSummary](#quotesummary)
  - [FundamentalsTimeseries](#fundamentalstimeseries)
  - [Chart](#chart)
  - [ChartSeries](#chartseries)
- [Tipos](#tipos)
  - [OHLCV](#ohlcv)
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

`Client` encapsula la sesión HTTP (cookies + token "crumb") necesaria para hablar con `query2.finance.yahoo.com`. Es seguro reutilizar la misma instancia para múltiples llamadas y tickers; no reutiliza estado entre tickers, así que no hace falta crear un `Client` por símbolo.

### NewClient

```go
func NewClient() (*Client, error)
```

Crea un `Client` autenticado:

1. Hace un `GET` a `https://fc.yahoo.com` para fijar cookies de consentimiento/sesión (se ignora cualquier error, incluyendo 404).
2. Hace un `GET` a `https://query2.finance.yahoo.com/v1/test/getcrumb` para obtener el token "crumb".

Devuelve error si no se pudo construir el cookie jar o si la respuesta del crumb viene vacía o es HTML (p. ej. una página de error/consentimiento en vez del token).

```go
client, err := yahoo.NewClient()
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
price := yahoo.Field(data["price"].(map[string]interface{}), "regularMarketPrice")
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

---

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
peRatio := yahoo.GetPath(data, "summaryDetail", "trailingPE")
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
fecha := yahoo.EpochToDateTimeNY(earningsDate, ny)
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

El cliente no reintenta automáticamente; ante un error transitorio (rate limit, timeout) es responsabilidad del caller reintentar con backoff.

## Límites y particularidades de Yahoo Finance

- No requiere API key: solo cookies de sesión + crumb público, obtenidos automáticamente en `NewClient`.
- Yahoo limita cuánto histórico intradía entrega (p. ej. `interval=1m` solo cubre los últimos ~7 días, independientemente del `range` pedido).
- `FundamentalsTimeseries` siempre pide una ventana de 6 años; para históricos más largos hay que paginar manualmente ajustando `period1`/`period2` fuera de esta librería (actualmente no configurable vía parámetros).
- Mismo endpoint `chart` sirve acciones (`AAPL`), ETFs (`SPY`) y criptomonedas (`BTC-USD`); solo cambia qué eventos (`dividends`/`splits`) trae cada uno.
- El `User-Agent` está fijado a un string de Chrome/Windows de escritorio (ver `client.go`); si Yahoo empieza a bloquear ese UA, es el primer lugar donde mirar.

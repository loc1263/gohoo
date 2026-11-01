# gohoo: documento técnico

Cómo funciona la librería por dentro, las reglas para cambiarla sin romper a quien la usa y cómo se prueba y se publica. Para usarla, ver el [README](README.es.md); para el detalle de cada función, [API.md](API.md); para los datos de cada endpoint de Yahoo, [ENDPOINTS.md](ENDPOINTS.md).

## 1. Origen y alcance

- Nació en [`go-finz-analysis`](https://github.com/loc1263/go-finz-analysis) como paquete interno de varios binarios (indicadores de acciones, precios de acciones, ETFs y criptomonedas). Esta es la versión reutilizable.
- Sin dependencias fuera de la librería estándar. Go 1.22 como mínimo.
- Habla con `query2.finance.yahoo.com` por `net/http`. Los endpoints son **no oficiales**: pueden cambiar sin aviso (ver el aviso del README).

## 2. Mapa de la librería

| Área | Qué hay |
|---|---|
| Cliente y sesión | `Client`, `NewClient`, `NewClientWithOptions` y las opciones `With...` |
| Históricos | `ChartRange`, `Monthly`, `AutoAdjust`, `AtMidnight`, `DailyBars`, `ChartSeries` |
| Cotizaciones | `Quotes`, `Quote`, `SharesOutstanding` |
| Datos de la empresa | `Info`, `FlattenInfo`, `QuoteSummaryContext` |
| Estados financieros | `FundamentalsTimeseriesContext`, `TimeseriesPoints`, helpers de `Point`, `TrailingPEG` |
| Screener | `Screen`, `ScreenPredefined`, el constructor de consultas (`Eq`, `And`...) |
| Reintentos y tráfico | `RetryPolicy`, `WithLimiter`, `NewIntervalLimiter`, `WithRequestHook` |
| Errores | `ErrNotFound`, `ErrRateLimit`, `ErrUnauthorized`, `StatusError` |
| Indicadores | `ComputeIndicators...`, `SMA`, `EMA`, `RSI`, `MACD`, `MFI`... |
| Riesgo | `Returns`, `LogReturns`, `Volatility`, `MaxDrawdown` |
| Utilidades | `ParallelMap`, `RawValue`, `GetPath`, `AsFloat`, `AsString`, funciones de fechas |

Los métodos antiguos (`QuoteSummary`, `FundamentalsTimeseries`, `Chart`, `ChartContext`, `FirstResult`) siguen funcionando y están marcados como `Deprecated`.

## 3. Sesión: cookie y crumb

Yahoo no pide API key, pero varios endpoints (`quoteSummary`, `quote`, `screener`, `fundamentals-timeseries`) exigen una cookie de sesión y un token público, el **crumb**.

- `NewClient` los pide al crearse. Con `WithLazyBootstrap`, el constructor no hace red y se piden la primera vez que hacen falta. `chart` (`ChartRange`, `DailyBars`, `ChartSeries`) nunca los necesita.
- Ante un HTTP 401 el cliente renueva la sesión y repite la llamada. Si el 401 persiste, el error cumple `errors.Is(err, ErrUnauthorized)`.
- `Client` es seguro entre goroutines; conviene uno solo por proceso.

## 4. Reintentos y límites de tasa

Yahoo limita por IP, así que no conviene lanzar ráfagas.

- **Política general (`RetryPolicy`).** Reintenta solo fallas transitorias (errores de red, HTTP 429 y 5xx) con espera exponencial: `MaxAttempts` (por defecto 4; 1 = sin reintentos), `BaseDelay`, `MaxDelay`, `Jitter`, `RetryRateLimit` y `Retry-After`.
- **Presupuesto propio para rate limits (`RetryPolicy.RateLimit`).** Un 429, o un «Too Many Requests» dentro de un cuerpo con status 200, se reintenta hasta `MaxRetries` veces sin gastar `MaxAttempts`, con su propio backoff y respetando `Retry-After` si se pide. Agotado el presupuesto, el error cumple `errors.Is(err, ErrRateLimit)`.
- **Por llamada.** `WithRetryPolicyContext(ctx, p)` cambia la política de las llamadas hechas con ese contexto: por ejemplo, reintentos pacientes en un proceso batch y ninguna espera larga dentro de un request web, con el mismo `Client`.
- **Ritmo de requests.** `WithLimiter(l)` llama a `l.Wait(ctx)` antes de cada request HTTP (incluidos reintentos y el arranque de la sesión). `NewIntervalLimiter(gap)` es un `Limiter` listo: una request cada `gap`, compartido entre goroutines, atiende en orden de llegada y respeta `ctx`. Un mismo limitador puede compartirse entre clientes.
- **Métricas.** `WithRequestHook(fn)` recibe cada request ya terminada, por ejemplo para contar los 429. Se llama desde la goroutine de la request: debe ser rápido y seguro entre goroutines.

## 5. Errores

Se clasifican con `errors.Is` / `errors.As`. El texto de los errores **no es parte de la API**.

| Error | Cuándo |
|---|---|
| `ErrNotFound` | HTTP 404; `chart.error` «Not Found» o «No data found»; chart vacío; error «Not Found» o «Quote not found» en el cuerpo de `quoteSummary`; `DailyBars` sin barras |
| `ErrRateLimit` | HTTP 429, o «Too Many Requests» en el cuerpo (también en `quoteSummary`) |
| `ErrUnauthorized` | HTTP 401 que persistió tras renovar el crumb |
| `*StatusError` | Cualquier respuesta distinta de 200: trae `StatusCode`, `URL`, `Body` y `RetryAfter` |

Límite conocido: `FundamentalsTimeseriesContext` no convierte en error un cuerpo con status 200 que trae un error; devuelve el mapa tal cual. Cambiarlo sería un cambio de comportamiento, así que si se hace va como función nueva.

## 6. Datos de Yahoo

- Los números de `quoteSummary` y `fundamentals-timeseries` llegan como `{"raw": x, "fmt": "texto"}`. `RawValue` y `Field` siempre extraen `raw`. `Quotes` pide `formatted=false` y recibe valores planos.
- `rang` e `interval` aceptan los mismos valores que Yahoo. Rangos: `1d, 5d, 1mo, 3mo, 6mo, 1y, 2y, 5y, 10y, ytd, max`. Intervalos: `1m, 2m, 5m, 15m, 30m, 60m/1h, 1d, 5d, 1wk, 1mo, 3mo`. Para intradía, Yahoo limita cuánta historia devuelve.
- `ChartRange` pone las barras en la **zona horaria del exchange**. En Windows sin Go instalado, la aplicación debe importar `_ "time/tzdata"`; si no, la zona cae a UTC. gohoo no lo importa para no sumar unos 450 KB a todos los binarios.
- `ChartResult.Monthly` toma el mes calendario **según la zona del exchange** (la barra mensual de marzo de Tokio es del 29 de febrero 15:00 UTC) y deja la barra más reciente si hay dos del mismo mes (la mensual y la «en vivo»).
- `AutoAdjust` equivale al `auto_adjust` de yfinance: Close = AdjClose y Open/High/Low escalados con el mismo factor. `AtMidnight` lleva la hora de cada barra a la medianoche de su día, para indexar por fecha; no usarlo con barras intradía.
- `Quotes` pide en lotes de `QuoteBatchSize` (100) y, si un lote falla, devuelve lo de los lotes anteriores junto con el error.

### Ejemplo: serie mensual por fechas, tratando los errores

```go
client, err := gohoo.NewClientWithOptions(
	gohoo.WithLazyBootstrap(), // chart no necesita cookie ni crumb
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

## 7. Indicadores

- Las funciones sueltas (`SMA`, `EMA`, `RSI`, `MACD`, `Bollinger`, `ATR`, `Stochastic`, `WilliamsR`, `ADX`, `OBV`, `VWAP`, `MFI`, `MFISkipNaN`, `Crossovers`) devuelven series **alineadas por índice** con la entrada.
- Donde falta historia suficiente el valor es `math.NaN()`. Un período menor que 1 da todo NaN; si las columnas no tienen el mismo largo, el resultado es `nil`.
- Un NaN en la entrada se propaga: a cada ventana que lo contiene en las medias móviles, y a todos los puntos siguientes en los indicadores que acumulan (EMA, RSI, ATR, ADX, MACD, OBV, VWAP).
- **Convención de cálculo.** gohoo siembra las EMA con una media simple de los primeros `period` valores. Eso **no coincide** con la convención de pandas (`ewm` con span), así que los valores difieren un poco de implementaciones basadas en pandas. Quien necesite pandas debe saberlo antes de reemplazar su propio cálculo.
- **MFI.** `MFI` deja en NaN toda ventana con un dato faltante. `MFISkipNaN` ignora las barras incompletas (no aportan flujo ni positivo ni negativo) y calcula con el resto; sin datos faltantes dan lo mismo. Ninguno forma parte de `IndicatorSet`.
- Para indicadores sobre precios ajustados, pasar `AutoAdjust(bars)`: `ComputeIndicatorsBars` usa los precios tal cual vienen.

## 8. Reglas de compatibilidad

Hay proyectos que ya usan la librería, así que **solo se agrega**:

1. No cambiar firmas, nombres ni tipos exportados existentes.
2. No cambiar el comportamiento de lo que ya existe. Si hay duda, va como función nueva.
3. Lo nuevo es opt-in: funciones u opciones nuevas cuyo valor por defecto es el comportamiento actual.
4. Los errores nuevos **envuelven** (`%w`) el error actual, para que `errors.Is`/`errors.As` sigan funcionando.
5. Agregar campos a un struct es compatible para quien los use por nombre; decirlo en el CHANGELOG.
6. Versionado semántico: lo aditivo sube la versión **menor** (0.x.0); los arreglos, la **patch**.
7. El CI corre `apidiff` contra el tag anterior y falla ante cualquier cambio incompatible.

## 9. Tests y CI

- `go test ./...` no usa red: simula a Yahoo con un `httptest.Server` local, algunos casos con respuestas reales grabadas en `testdata/`. La cobertura está en 99,9 %.
- Pruebas contra Yahoo real, solo a pedido: `go test -tags live -run Live ./...`.
- El CI (`.github/workflows/ci.yml`) corre en cada push a `main` y en cada pull request:
  - `gofmt`, `go vet` (también con `-tags live`) y `golangci-lint`.
  - `apidiff` contra el tag anterior (sin cambios incompatibles).
  - Los tests en Linux y Windows, con Go 1.22 y con la última versión.
- `.gitattributes` fuerza finales de línea LF para que `gofmt` quede limpio en Windows.

## 10. Cómo publicar una versión

1. Trabajar en una rama; cada cambio con sus tests, su entrada en `API.md` (y en el README si aplica) y su línea en `CHANGELOG.md`.
2. Pasar la sección `[Unreleased]` del CHANGELOG a `[X.Y.Z] - fecha`, con la frase sobre compatibilidad (por ejemplo «Only additions: `apidiff` against vA.B.C reports no incompatible change»).
3. Comprobar la compatibilidad en local con `apidiff` contra el tag anterior (el CI lo repite).
4. Fusionar en `main` y crear el tag anotado: `git tag -a vX.Y.Z -m "vX.Y.Z"`. Subir `main` y el tag: `git push origin main vX.Y.Z`.
5. **Crear el release en GitHub** (el tag solo no lo crea): `gh release create vX.Y.Z --title vX.Y.Z --notes-file <notas>`, con el texto de esa versión del CHANGELOG, y `--latest` si es la más nueva.
6. Esperar a que pase el CI y comprobar que `go list -m github.com/loc1263/gohoo@vX.Y.Z` resuelve.

## 11. Documentos de diseño

`DISENO_EXTENSIONES.md` (2026-10-08) y `DISENO_V0.3.md` (2026-10-12) son los diseños de las extensiones de la librería. Sirven de contexto; la referencia vigente es `API.md`.

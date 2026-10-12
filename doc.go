// Package gohoo es un cliente para los endpoints no oficiales de Yahoo
// Finance, sin dependencias externas: históricos de precios, cotizaciones,
// datos de la empresa, estados financieros y screener, más indicadores
// técnicos y métricas de riesgo sobre las series descargadas.
//
// # Cliente
//
// Un [Client] maneja la sesión (cookie + crumb) y es seguro para usar desde
// varias goroutines; conviene crear uno solo por proceso:
//
//	client, err := gohoo.NewClientWithOptions(gohoo.WithLazyBootstrap())
//
// [NewClient] pide la sesión de inmediato; con [WithLazyBootstrap] se pide
// recién cuando un endpoint la necesita. Otras opciones: [WithHTTPClient],
// [WithBaseURL], [WithUserAgent], [WithRetryPolicy], [WithLimiter] y
// [WithRequestHook].
//
// # Qué método usar
//
//   - Históricos de precios: [Client.ChartRange] (barras tipadas en la zona
//     del exchange, por rango relativo o por fechas). Para precios ajustados,
//     [AutoAdjust] o [ChartResult.AutoAdjusted]; una serie mensual,
//     [ChartResult.Monthly].
//   - Cotización actual de muchos símbolos: [Client.Quotes] (en lotes).
//   - Datos de la empresa al estilo Ticker.info de yfinance: [Client.Info] /
//     [FlattenInfo]; la respuesta cruda, [Client.QuoteSummaryContext].
//   - Estados financieros: [Client.FundamentalsTimeseriesContext] +
//     [TimeseriesPoints]; el PEG, [Client.TrailingPEG].
//   - Búsqueda por criterios: [Client.Screen] (con [Eq], [Gt], [And], ...) y
//     [Client.ScreenPredefined].
//   - Cualquier otro endpoint con la misma sesión: [Client.GetJSON] /
//     [Client.PostJSON].
//   - Indicadores: [ComputeIndicatorsBars] o las funciones sueltas con
//     período configurable ([SMA], [EMA], [RSI], [MACD], ...) sobre columnas
//     de [Closes], [Highs], ...; riesgo: [Returns], [Volatility],
//     [MaxDrawdown].
//
// QuoteSummary, FundamentalsTimeseries, Chart, ChartContext y FirstResult se
// mantienen por compatibilidad y están marcados como Deprecated. ChartSeries,
// ComputeIndicators y DetectGaps trabajan con [OHLCV] (valores sin tipar); para
// código nuevo se recomiendan ChartRange y ComputeIndicatorsBars.
//
// # Errores y reintentos
//
// Una respuesta HTTP distinta de 200 es un [*StatusError]; los casos comunes
// se distinguen con errors.Is: [ErrRateLimit] (HTTP 429), [ErrNotFound]
// (símbolo sin datos) y [ErrUnauthorized]. Siguen siendo detectables aunque el
// error venga envuelto tras agotar los reintentos.
//
// Por defecto se reintentan los errores de red, 429 y 5xx (4 intentos con
// backoff desde 200 ms) y se renueva la sesión ante un 401. [RetryPolicy]
// lo configura; [RetryPolicy.RateLimit] da a los 429 su propio presupuesto y
// [WithRetryPolicyContext] cambia la política para una sola llamada.
//
// Los textos de error son para personas, no API: para clasificar un error se
// usan errors.Is y errors.As.
//
// # Límites de Yahoo
//
// Los endpoints no son oficiales y pueden cambiar sin aviso. Yahoo limita la
// tasa de requests por IP: conviene espaciar las llamadas ([WithLimiter]) y
// preferir los endpoints en lote ([Client.Quotes], [Client.Screen]). En
// Windows sin Go instalado, la aplicación debe importar _ "time/tzdata"
// para que las barras queden en la zona horaria del exchange.
package gohoo

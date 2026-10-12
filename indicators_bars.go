package gohoo

import "math"

// Indicadores técnicos sobre barras tipadas y sobre columnas []float64, con
// períodos configurables. Convenciones comunes:
//
//   - El resultado está alineado por índice con la entrada (out[i]
//     corresponde a values[i]) y tiene su mismo largo.
//   - Los puntos sin historia suficiente quedan en math.NaN().
//   - Un período < 1 devuelve todo NaN.
//   - Las funciones que reciben varias columnas devuelven nil si no tienen
//     todas el mismo largo.
//   - Un NaN en la entrada se propaga: en las medias móviles, a cada ventana
//     que lo contiene; en las que acumulan (EMA, RSI, ATR, ADX, MACD, OBV,
//     VWAP), a todos los puntos siguientes. Las barras de ChartRange solo
//     traen NaN en campos que Yahoo mandó nulos.

// ComputeIndicatorsBars es ComputeIndicators sobre barras tipadas (por
// ejemplo, las de ChartRange): mismos indicadores, mismos períodos y mismas
// señales. Usa los precios tal cual vienen; para indicadores sobre precios
// ajustados, pasar AutoAdjust(bars).
func ComputeIndicatorsBars(bars []Bar) []IndicatorSet {
	return computeIndicators(Highs(bars), Lows(bars), Closes(bars), Volumes(bars))
}

func column(bars []Bar, f func(Bar) float64) []float64 {
	out := make([]float64, len(bars))
	for i, b := range bars {
		out[i] = f(b)
	}
	return out
}

// Opens devuelve los Open de bars.
func Opens(bars []Bar) []float64 { return column(bars, func(b Bar) float64 { return b.Open }) }

// Highs devuelve los High de bars.
func Highs(bars []Bar) []float64 { return column(bars, func(b Bar) float64 { return b.High }) }

// Lows devuelve los Low de bars.
func Lows(bars []Bar) []float64 { return column(bars, func(b Bar) float64 { return b.Low }) }

// Closes devuelve los Close de bars.
func Closes(bars []Bar) []float64 { return column(bars, func(b Bar) float64 { return b.Close }) }

// AdjCloses devuelve los AdjClose de bars.
func AdjCloses(bars []Bar) []float64 { return column(bars, func(b Bar) float64 { return b.AdjClose }) }

// Volumes devuelve los Volume de bars.
func Volumes(bars []Bar) []float64 { return column(bars, func(b Bar) float64 { return b.Volume }) }

func nanSlice(n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	return out
}

func sameLen(cols ...[]float64) bool {
	for _, c := range cols[1:] {
		if len(c) != len(cols[0]) {
			return false
		}
	}
	return true
}

// SMA es la media móvil simple de period puntos.
func SMA(values []float64, period int) []float64 {
	if period < 1 {
		return nanSlice(len(values))
	}
	return sma(values, period)
}

// EMA es la media móvil exponencial de period puntos (factor 2/(period+1)),
// semillada con la SMA de los primeros period valores no-NaN.
func EMA(values []float64, period int) []float64 {
	if period < 1 {
		return nanSlice(len(values))
	}
	return ema(values, period)
}

// RSI es el Relative Strength Index de Wilder de period puntos (0..100; 14
// es el estándar). Los primeros period valores quedan en NaN.
func RSI(closes []float64, period int) []float64 {
	if period < 1 {
		return nanSlice(len(closes))
	}
	return rsi(closes, period)
}

// MACD devuelve la línea MACD (EMA fast - EMA slow), su señal (EMA de
// signal puntos de la línea) y el histograma (línea - señal). El estándar
// es MACD(closes, 12, 26, 9).
func MACD(closes []float64, fast, slow, signal int) (line, sig, hist []float64) {
	if fast < 1 || slow < 1 || signal < 1 {
		n := len(closes)
		return nanSlice(n), nanSlice(n), nanSlice(n)
	}
	return macd(ema(closes, fast), ema(closes, slow), signal)
}

// Bollinger devuelve las Bandas de Bollinger: la media (SMA de period
// puntos) y las bandas a numStdDev desviaciones estándar poblacionales de
// esa ventana. El estándar es Bollinger(closes, 20, 2).
func Bollinger(closes []float64, period int, numStdDev float64) (upper, middle, lower []float64) {
	if period < 1 {
		n := len(closes)
		return nanSlice(n), nanSlice(n), nanSlice(n)
	}
	return bollinger(closes, period, numStdDev)
}

// ATR es el Average True Range de Wilder de period puntos.
func ATR(highs, lows, closes []float64, period int) []float64 {
	if !sameLen(highs, lows, closes) {
		return nil
	}
	if period < 1 {
		return nanSlice(len(closes))
	}
	return atr(highs, lows, closes, period)
}

// Stochastic es el Oscilador Estocástico: %K es la posición del cierre en
// el rango máximo/mínimo de las últimas kPeriod barras (0..100; 50 si el
// rango es nulo) y %D, la SMA de dPeriod puntos de %K. El estándar es
// Stochastic(highs, lows, closes, 14, 3).
func Stochastic(highs, lows, closes []float64, kPeriod, dPeriod int) (k, d []float64) {
	if !sameLen(highs, lows, closes) {
		return nil, nil
	}
	if kPeriod < 1 || dPeriod < 1 {
		return nanSlice(len(closes)), nanSlice(len(closes))
	}
	return stochastic(highs, lows, closes, kPeriod, dPeriod)
}

// WilliamsR es Williams %R de period puntos (-100..0; -50 si el rango es
// nulo).
func WilliamsR(highs, lows, closes []float64, period int) []float64 {
	if !sameLen(highs, lows, closes) {
		return nil
	}
	if period < 1 {
		return nanSlice(len(closes))
	}
	return williamsR(highs, lows, closes, period)
}

// ADX es el Average Directional Index de Wilder de period puntos (necesita
// del orden de 2·period barras antes de su primer valor).
func ADX(highs, lows, closes []float64, period int) []float64 {
	if !sameLen(highs, lows, closes) {
		return nil
	}
	if period < 1 {
		return nanSlice(len(closes))
	}
	return adx(highs, lows, closes, period)
}

// OBV es el On-Balance Volume: arranca en 0 y suma el volumen si el cierre
// sube y lo resta si baja.
func OBV(closes, volumes []float64) []float64 {
	if !sameLen(closes, volumes) {
		return nil
	}
	return obv(closes, volumes)
}

// VWAP es el precio típico (H+L+C)/3 ponderado por volumen, acumulado desde
// el inicio de la serie (anclado, no reiniciado por sesión). NaN mientras
// el volumen acumulado sea 0.
func VWAP(highs, lows, closes, volumes []float64) []float64 {
	if !sameLen(highs, lows, closes, volumes) {
		return nil
	}
	return vwap(highs, lows, closes, volumes)
}

// Crossovers marca los cruces entre una serie rápida y una lenta (p. ej.
// dos medias móviles): +1 en la barra en que fast pasa a estar por encima
// de slow, -1 cuando pasa a estar por debajo y 0 si no hay cruce o falta
// algún valor. Devuelve nil si los largos no coinciden.
func Crossovers(fast, slow []float64) []int {
	if !sameLen(fast, slow) {
		return nil
	}
	out := make([]int, len(fast))
	for i, s := range crossSignals(fast, slow) {
		switch s {
		case "Golden Cross":
			out[i] = 1
		case "Death Cross":
			out[i] = -1
		}
	}
	return out
}

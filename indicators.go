package gohoo

import "math"

// IndicatorSet contiene los indicadores técnicos derivados de una serie
// OHLCV, alineados por índice con la serie de entrada (IndicatorSet[i]
// corresponde a series[i]). Los valores que todavía no tienen suficiente
// historia para calcularse (p.ej. RSI14 en las primeras 14 barras) quedan en
// math.NaN(). Las señales derivadas son strings vacíos cuando no aplican.
type IndicatorSet struct {
	SMA20                      float64
	EMA12, EMA26               float64
	RSI14                      float64
	MACD, MACDSignal, MACDHist float64
	BBUpper, BBMiddle, BBLower float64
	OBV                        float64
	VolumeSMA20                float64
	ATR14                      float64
	StochK, StochD             float64
	WilliamsR14                float64
	VWAP                       float64
	ADX14                      float64

	// CrossSignal es "Golden Cross" cuando SMA20 cruza hacia arriba a EMA26
	// en esta barra, "Death Cross" cuando cruza hacia abajo, "" si no hay
	// cruce.
	CrossSignal string
	// RSISignal es "Sobrecompra" si RSI14>=70, "Sobreventa" si RSI14<=30,
	// "" en cualquier otro caso (incluido RSI14 aún no disponible).
	RSISignal string
	// BBSignal es "Sobre banda superior"/"Bajo banda inferior" cuando el
	// cierre queda fuera de las Bandas de Bollinger, "" si está dentro.
	BBSignal string
}

// ComputeIndicators calcula el conjunto estándar de indicadores técnicos a
// partir de una serie OHLCV ordenada cronológicamente: SMA20 y EMA12/26 de
// cierre, RSI14 (suavizado de Wilder), MACD(12,26,9), Bandas de Bollinger
// (20, 2 desviaciones estándar), ATR14, Oscilador Estocástico(14,3),
// Williams %R14, ADX14, VWAP anclado al inicio de la serie, OBV y SMA20 de
// volumen, más señales derivadas (cruce SMA/EMA, zona de RSI, ruptura de
// Bollinger).
func ComputeIndicators(series []OHLCV) []IndicatorSet {
	n := len(series)
	highs := make([]float64, n)
	lows := make([]float64, n)
	closes := make([]float64, n)
	volumes := make([]float64, n)
	for i, bar := range series {
		highs[i], _ = AsFloat(bar.High)
		lows[i], _ = AsFloat(bar.Low)
		closes[i], _ = AsFloat(bar.Close)
		volumes[i], _ = AsFloat(bar.Volume)
	}

	sma20 := sma(closes, 20)
	ema12 := ema(closes, 12)
	ema26 := ema(closes, 26)
	rsi14 := rsi(closes, 14)

	macdLine := make([]float64, n)
	for i := range n {
		if math.IsNaN(ema12[i]) || math.IsNaN(ema26[i]) {
			macdLine[i] = math.NaN()
		} else {
			macdLine[i] = ema12[i] - ema26[i]
		}
	}
	macdSignal := ema(macdLine, 9)
	macdHist := make([]float64, n)
	for i := range n {
		if math.IsNaN(macdLine[i]) || math.IsNaN(macdSignal[i]) {
			macdHist[i] = math.NaN()
		} else {
			macdHist[i] = macdLine[i] - macdSignal[i]
		}
	}

	bbUpper, bbMiddle, bbLower := bollinger(closes, 20, 2)
	obvSeries := obv(closes, volumes)
	volSMA20 := sma(volumes, 20)

	atr14 := atr(highs, lows, closes, 14)
	stochK, stochD := stochastic(highs, lows, closes, 14, 3)
	williamsR14 := williamsR(highs, lows, closes, 14)
	vwapSeries := vwap(highs, lows, closes, volumes)
	adx14 := adx(highs, lows, closes, 14)

	crossSignal := crossSignals(sma20, ema26)
	rsiSignal := rsiSignals(rsi14)
	bbSignal := bbSignals(closes, bbUpper, bbLower)

	out := make([]IndicatorSet, n)
	for i := range n {
		out[i] = IndicatorSet{
			SMA20: sma20[i], EMA12: ema12[i], EMA26: ema26[i],
			RSI14:      rsi14[i],
			MACD:       macdLine[i],
			MACDSignal: macdSignal[i],
			MACDHist:   macdHist[i],
			BBUpper:    bbUpper[i], BBMiddle: bbMiddle[i], BBLower: bbLower[i],
			OBV:         obvSeries[i],
			VolumeSMA20: volSMA20[i],
			ATR14:       atr14[i],
			StochK:      stochK[i], StochD: stochD[i],
			WilliamsR14: williamsR14[i],
			VWAP:        vwapSeries[i],
			ADX14:       adx14[i],
			CrossSignal: crossSignal[i],
			RSISignal:   rsiSignal[i],
			BBSignal:    bbSignal[i],
		}
	}
	return out
}

// sma calcula la media móvil simple de `period` puntos; los primeros
// period-1 valores quedan en NaN por falta de historia.
func sma(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	for i := range out {
		if i < period-1 {
			out[i] = math.NaN()
			continue
		}
		sum := 0.0
		for j := i - period + 1; j <= i; j++ {
			sum += values[j]
		}
		out[i] = sum / float64(period)
	}
	return out
}

// ema calcula la media móvil exponencial de `period` puntos, semillada con
// la SMA de los primeros `period` valores no-NaN de la serie (lo que permite
// encadenarla sobre series que ya empiezan con NaN, como la línea MACD).
func ema(values []float64, period int) []float64 {
	out := make([]float64, len(values))
	for i := range out {
		out[i] = math.NaN()
	}
	start := firstNonNaN(values)
	if start == -1 || len(values)-start < period {
		return out
	}
	seed := 0.0
	for i := start; i < start+period; i++ {
		seed += values[i]
	}
	seed /= float64(period)
	seedIdx := start + period - 1
	out[seedIdx] = seed
	k := 2.0 / (float64(period) + 1.0)
	for i := seedIdx + 1; i < len(values); i++ {
		out[i] = values[i]*k + out[i-1]*(1-k)
	}
	return out
}

// wilderSmooth aplica el suavizado de Wilder (usado por ATR/ADX/RSI): la
// semilla es la media simple de los primeros `period` valores no-NaN, y de
// ahí en adelante out[i] = (out[i-1]*(period-1) + values[i]) / period.
func wilderSmooth(values []float64, period int) []float64 {
	n := len(values)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	start := firstNonNaN(values)
	if start == -1 || n-start < period {
		return out
	}
	sum := 0.0
	for i := start; i < start+period; i++ {
		sum += values[i]
	}
	seedIdx := start + period - 1
	out[seedIdx] = sum / float64(period)
	for i := seedIdx + 1; i < n; i++ {
		out[i] = (out[i-1]*float64(period-1) + values[i]) / float64(period)
	}
	return out
}

func firstNonNaN(values []float64) int {
	for i, v := range values {
		if !math.IsNaN(v) {
			return i
		}
	}
	return -1
}

// rsi calcula el RSI de Wilder sobre `period` puntos (14 es el estándar).
// Los primeros `period` valores quedan en NaN.
func rsi(closes []float64, period int) []float64 {
	n := len(closes)
	out := make([]float64, n)
	for i := range out {
		out[i] = math.NaN()
	}
	if n < period+1 {
		return out
	}
	gainSum, lossSum := 0.0, 0.0
	for i := 1; i <= period; i++ {
		diff := closes[i] - closes[i-1]
		if diff > 0 {
			gainSum += diff
		} else {
			lossSum += -diff
		}
	}
	avgGain := gainSum / float64(period)
	avgLoss := lossSum / float64(period)
	out[period] = rsiFromAverages(avgGain, avgLoss)
	for i := period + 1; i < n; i++ {
		diff := closes[i] - closes[i-1]
		gain, loss := 0.0, 0.0
		if diff > 0 {
			gain = diff
		} else {
			loss = -diff
		}
		avgGain = (avgGain*float64(period-1) + gain) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + loss) / float64(period)
		out[i] = rsiFromAverages(avgGain, avgLoss)
	}
	return out
}

func rsiFromAverages(avgGain, avgLoss float64) float64 {
	if avgLoss == 0 {
		if avgGain == 0 {
			return 50
		}
		return 100
	}
	rs := avgGain / avgLoss
	return 100 - (100 / (1 + rs))
}

// bollinger calcula las Bandas de Bollinger: banda media (SMA de `period`
// puntos) y bandas superior/inferior a `numStdDev` desviaciones estándar
// poblacionales de esa misma ventana.
func bollinger(closes []float64, period int, numStdDev float64) (upper, middle, lower []float64) {
	middle = sma(closes, period)
	n := len(closes)
	upper = make([]float64, n)
	lower = make([]float64, n)
	for i := range n {
		if math.IsNaN(middle[i]) {
			upper[i] = math.NaN()
			lower[i] = math.NaN()
			continue
		}
		sumSq := 0.0
		for j := i - period + 1; j <= i; j++ {
			d := closes[j] - middle[i]
			sumSq += d * d
		}
		std := math.Sqrt(sumSq / float64(period))
		upper[i] = middle[i] + numStdDev*std
		lower[i] = middle[i] - numStdDev*std
	}
	return upper, middle, lower
}

// obv calcula el On-Balance Volume: arranca en 0 y acumula +volumen si el
// cierre sube, -volumen si baja, y se mantiene igual si no cambia.
func obv(closes, volumes []float64) []float64 {
	n := len(closes)
	out := make([]float64, n)
	for i := 1; i < n; i++ {
		switch {
		case closes[i] > closes[i-1]:
			out[i] = out[i-1] + volumes[i]
		case closes[i] < closes[i-1]:
			out[i] = out[i-1] - volumes[i]
		default:
			out[i] = out[i-1]
		}
	}
	return out
}

// trueRange calcula el True Range de cada barra: el mayor entre el rango
// intradía y la distancia al cierre anterior (la primera barra usa solo el
// rango intradía, al no tener cierre previo).
func trueRange(highs, lows, closes []float64) []float64 {
	n := len(closes)
	tr := make([]float64, n)
	for i := range n {
		if i == 0 {
			tr[i] = highs[i] - lows[i]
			continue
		}
		hl := highs[i] - lows[i]
		hc := math.Abs(highs[i] - closes[i-1])
		lc := math.Abs(lows[i] - closes[i-1])
		tr[i] = math.Max(hl, math.Max(hc, lc))
	}
	return tr
}

// atr calcula el Average True Range (suavizado de Wilder sobre el True
// Range) de `period` puntos.
func atr(highs, lows, closes []float64, period int) []float64 {
	return wilderSmooth(trueRange(highs, lows, closes), period)
}

// stochastic calcula el Oscilador Estocástico: %K es la posición del cierre
// dentro del rango máximo/mínimo de las últimas `kPeriod` barras (0-100), y
// %D es la SMA de `dPeriod` puntos de %K.
func stochastic(highs, lows, closes []float64, kPeriod, dPeriod int) (k, d []float64) {
	n := len(closes)
	k = make([]float64, n)
	for i := range n {
		if i < kPeriod-1 {
			k[i] = math.NaN()
			continue
		}
		hh, ll := highs[i], lows[i]
		for j := i - kPeriod + 1; j <= i; j++ {
			hh = math.Max(hh, highs[j])
			ll = math.Min(ll, lows[j])
		}
		if hh == ll {
			k[i] = 50
		} else {
			k[i] = 100 * (closes[i] - ll) / (hh - ll)
		}
	}
	d = sma(k, dPeriod)
	return k, d
}

// williamsR calcula Williams %R de `period` puntos: igual que %K del
// estocástico pero expresado en escala -100..0.
func williamsR(highs, lows, closes []float64, period int) []float64 {
	n := len(closes)
	out := make([]float64, n)
	for i := range n {
		if i < period-1 {
			out[i] = math.NaN()
			continue
		}
		hh, ll := highs[i], lows[i]
		for j := i - period + 1; j <= i; j++ {
			hh = math.Max(hh, highs[j])
			ll = math.Min(ll, lows[j])
		}
		if hh == ll {
			out[i] = -50
		} else {
			out[i] = -100 * (hh - closes[i]) / (hh - ll)
		}
	}
	return out
}

// vwap calcula el precio promedio ponderado por volumen, acumulado desde el
// inicio de la serie (VWAP "anclado", no reiniciado por sesión: en datos
// diarios/multi-día es una referencia de precio promedio ponderado de largo
// plazo, no el VWAP intradía clásico).
func vwap(highs, lows, closes, volumes []float64) []float64 {
	n := len(closes)
	out := make([]float64, n)
	cumPV, cumVol := 0.0, 0.0
	for i := range n {
		typical := (highs[i] + lows[i] + closes[i]) / 3
		cumPV += typical * volumes[i]
		cumVol += volumes[i]
		if cumVol == 0 {
			out[i] = math.NaN()
		} else {
			out[i] = cumPV / cumVol
		}
	}
	return out
}

// adx calcula el Average Directional Index de Wilder: a partir del
// movimiento direccional (+DM/-DM) y el True Range, ambos suavizados, se
// deriva el DX de cada barra y luego se suaviza de nuevo para obtener ADX.
// Requiere del orden de 2*period barras antes de dar su primer valor.
func adx(highs, lows, closes []float64, period int) []float64 {
	n := len(closes)
	plusDM := make([]float64, n)
	minusDM := make([]float64, n)
	for i := 1; i < n; i++ {
		upMove := highs[i] - highs[i-1]
		downMove := lows[i-1] - lows[i]
		if upMove > downMove && upMove > 0 {
			plusDM[i] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDM[i] = downMove
		}
	}
	tr := trueRange(highs, lows, closes)

	smoothTR := wilderSmooth(tr, period)
	smoothPlusDM := wilderSmooth(plusDM, period)
	smoothMinusDM := wilderSmooth(minusDM, period)

	dx := make([]float64, n)
	for i := range n {
		if math.IsNaN(smoothTR[i]) || smoothTR[i] == 0 {
			dx[i] = math.NaN()
			continue
		}
		plusDI := 100 * smoothPlusDM[i] / smoothTR[i]
		minusDI := 100 * smoothMinusDM[i] / smoothTR[i]
		sumDI := plusDI + minusDI
		if sumDI == 0 {
			dx[i] = 0
		} else {
			dx[i] = 100 * math.Abs(plusDI-minusDI) / sumDI
		}
	}
	return wilderSmooth(dx, period)
}

// crossSignals detecta cruces entre una media rápida (fast) y una lenta
// (slow): "Golden Cross" cuando fast cruza hacia arriba a slow, "Death
// Cross" cuando cruza hacia abajo.
func crossSignals(fast, slow []float64) []string {
	n := len(fast)
	out := make([]string, n)
	for i := 1; i < n; i++ {
		if math.IsNaN(fast[i]) || math.IsNaN(slow[i]) || math.IsNaN(fast[i-1]) || math.IsNaN(slow[i-1]) {
			continue
		}
		prevDiff := fast[i-1] - slow[i-1]
		curDiff := fast[i] - slow[i]
		switch {
		case prevDiff <= 0 && curDiff > 0:
			out[i] = "Golden Cross"
		case prevDiff >= 0 && curDiff < 0:
			out[i] = "Death Cross"
		}
	}
	return out
}

// rsiSignals marca "Sobrecompra" (RSI>=70) y "Sobreventa" (RSI<=30).
func rsiSignals(rsi []float64) []string {
	out := make([]string, len(rsi))
	for i, v := range rsi {
		switch {
		case math.IsNaN(v):
		case v >= 70:
			out[i] = "Sobrecompra"
		case v <= 30:
			out[i] = "Sobreventa"
		}
	}
	return out
}

// bbSignals marca cuando el cierre queda fuera de las Bandas de Bollinger.
func bbSignals(closes, upper, lower []float64) []string {
	out := make([]string, len(closes))
	for i := range closes {
		if math.IsNaN(upper[i]) || math.IsNaN(lower[i]) {
			continue
		}
		switch {
		case closes[i] > upper[i]:
			out[i] = "Sobre banda superior"
		case closes[i] < lower[i]:
			out[i] = "Bajo banda inferior"
		}
	}
	return out
}

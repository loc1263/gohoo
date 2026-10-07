package yahoo

import (
	"math"
	"testing"
	"time"
)

func mustNaN(t *testing.T, name string, i int, v float64) {
	t.Helper()
	if !math.IsNaN(v) {
		t.Errorf("%s[%d] = %v, want NaN (todavía sin historia suficiente)", name, i, v)
	}
}

func almostEqual(a, b float64) bool {
	return math.Abs(a-b) < 1e-9
}

// barsFromCloses construye una serie OHLCV sintética donde Open=High=Low=
// Close=closes[i] (sin rango intradía) y Volume=volumes[i], una barra por
// día consecutivo. Sirve para que los indicadores basados en High/Low se
// comporten de forma predecible en los tests.
func barsFromCloses(closes, volumes []float64) []OHLCV {
	series := make([]OHLCV, len(closes))
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	for i, c := range closes {
		series[i] = OHLCV{
			Time: base.AddDate(0, 0, i),
			Open: c, High: c, Low: c, Close: c, AdjClose: c,
			Volume: volumes[i],
		}
	}
	return series
}

func TestSMA(t *testing.T) {
	got := sma([]float64{1, 2, 3, 4, 5}, 3)
	want := []float64{math.NaN(), math.NaN(), 2, 3, 4}
	for i := range want {
		if math.IsNaN(want[i]) {
			mustNaN(t, "sma", i, got[i])
			continue
		}
		if !almostEqual(got[i], want[i]) {
			t.Errorf("sma[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestEMA(t *testing.T) {
	// Semilla en el índice 2 = media(1,2,3) = 2; luego k=2/(3+1)=0.5:
	// ema[3] = 4*0.5 + 2*0.5 = 3; ema[4] = 5*0.5 + 3*0.5 = 4.
	got := ema([]float64{1, 2, 3, 4, 5}, 3)
	want := []float64{math.NaN(), math.NaN(), 2, 3, 4}
	for i := range want {
		if math.IsNaN(want[i]) {
			mustNaN(t, "ema", i, got[i])
			continue
		}
		if !almostEqual(got[i], want[i]) {
			t.Errorf("ema[%d] = %v, want %v", i, got[i], want[i])
		}
	}
}

func TestComputeIndicatorsConstantPrice(t *testing.T) {
	n := 40
	closes := make([]float64, n)
	volumes := make([]float64, n)
	for i := range n {
		closes[i] = 100
		volumes[i] = 1000
	}
	series := barsFromCloses(closes, volumes)
	ind := ComputeIndicators(series)

	last := ind[n-1]
	if last.SMA20 != 100 {
		t.Errorf("SMA20 = %v, want 100", last.SMA20)
	}
	if last.EMA12 != 100 || last.EMA26 != 100 {
		t.Errorf("EMA12/26 = %v/%v, want 100/100", last.EMA12, last.EMA26)
	}
	if last.RSI14 != 50 {
		t.Errorf("RSI14 = %v, want 50 (sin variación de precio)", last.RSI14)
	}
	if last.MACD != 0 || last.MACDHist != 0 {
		t.Errorf("MACD/Hist = %v/%v, want 0/0", last.MACD, last.MACDHist)
	}
	if last.BBUpper != 100 || last.BBLower != 100 {
		t.Errorf("BBUpper/Lower = %v/%v, want 100/100 (desviación 0)", last.BBUpper, last.BBLower)
	}
	if last.OBV != 0 {
		t.Errorf("OBV = %v, want 0 (el cierre nunca cambia)", last.OBV)
	}
	if last.ATR14 != 0 {
		t.Errorf("ATR14 = %v, want 0 (sin rango intradía ni gaps)", last.ATR14)
	}
	if last.StochK != 50 || last.StochD != 50 {
		t.Errorf("StochK/D = %v/%v, want 50/50", last.StochK, last.StochD)
	}
	if last.WilliamsR14 != -50 {
		t.Errorf("WilliamsR14 = %v, want -50", last.WilliamsR14)
	}
	if last.VWAP != 100 {
		t.Errorf("VWAP = %v, want 100", last.VWAP)
	}
	if last.CrossSignal != "" || last.RSISignal != "" || last.BBSignal != "" {
		t.Errorf("señales = %q/%q/%q, want todas vacías con precio constante",
			last.CrossSignal, last.RSISignal, last.BBSignal)
	}
}

func TestComputeIndicatorsStrictlyIncreasing(t *testing.T) {
	n := 40
	closes := make([]float64, n)
	volumes := make([]float64, n)
	for i := range n {
		closes[i] = float64(i + 1) // 1, 2, 3, ...
		volumes[i] = 100
	}
	series := barsFromCloses(closes, volumes)
	ind := ComputeIndicators(series)

	for i := 14; i < n; i++ {
		if ind[i].RSI14 != 100 {
			t.Errorf("RSI14[%d] = %v, want 100 (nunca hay pérdidas)", i, ind[i].RSI14)
		}
	}
	for i := range n {
		want := float64(i) * 100 // obv[0]=0, cada barra suma volumes[i]=100
		if ind[i].OBV != want {
			t.Errorf("OBV[%d] = %v, want %v", i, ind[i].OBV, want)
		}
	}
	// En una tendencia alcista sostenida, MACD queda positivo (EMA12>EMA26)
	// una vez ambas EMAs están disponibles.
	if ind[n-1].MACD <= 0 {
		t.Errorf("MACD[%d] = %v, want > 0 en tendencia alcista sostenida", n-1, ind[n-1].MACD)
	}
}

func TestComputeIndicatorsWarmupIsNaN(t *testing.T) {
	n := 30
	closes := make([]float64, n)
	volumes := make([]float64, n)
	for i := range n {
		closes[i] = float64(50 + i%5)
		volumes[i] = 500
	}
	series := barsFromCloses(closes, volumes)
	ind := ComputeIndicators(series)

	mustNaN(t, "SMA20", 0, ind[0].SMA20)
	mustNaN(t, "SMA20", 18, ind[18].SMA20)
	if math.IsNaN(ind[19].SMA20) {
		t.Errorf("SMA20[19] = NaN, want un valor ya disponible (barra 20)")
	}
	mustNaN(t, "RSI14", 13, ind[13].RSI14)
	if math.IsNaN(ind[14].RSI14) {
		t.Errorf("RSI14[14] = NaN, want un valor ya disponible")
	}
	mustNaN(t, "MACD", 24, ind[24].MACD)
	if math.IsNaN(ind[25].MACD) {
		t.Errorf("MACD[25] = NaN, want un valor ya disponible (EMA26 lista desde el índice 25)")
	}
}

func TestDetectGapsFlagsUnusualGap(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	mk := func(days ...int) []OHLCV {
		s := make([]OHLCV, len(days))
		for i, d := range days {
			s[i] = OHLCV{Time: base.AddDate(0, 0, d)}
		}
		return s
	}
	// Cadencia diaria normal, con un salto de fin de semana (día 4->7, un
	// delta de 3 días contra una mediana de 1 día) y luego un hueco real de
	// 20 días que sí debería marcarse.
	series := mk(0, 1, 2, 3, 4, 7, 8, 9, 29)
	gaps := DetectGaps(series, 5)
	if len(gaps) != 1 {
		t.Fatalf("len(gaps) = %d, want 1 (solo el hueco de 20 días debería superar el umbral)", len(gaps))
	}
	if gaps[0].Real != 20*24*time.Hour {
		t.Errorf("gaps[0].Real = %v, want 480h", gaps[0].Real)
	}
}

func TestDetectGapsNoGapsOnRegularSeries(t *testing.T) {
	base := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)
	series := make([]OHLCV, 10)
	for i := range series {
		series[i] = OHLCV{Time: base.AddDate(0, 0, i)}
	}
	if gaps := DetectGaps(series, 5); len(gaps) != 0 {
		t.Errorf("gaps = %v, want ninguno en una serie perfectamente regular", gaps)
	}
	if gaps := DetectGaps(series[:1], 5); gaps != nil {
		t.Errorf("gaps con 1 sola barra = %v, want nil", gaps)
	}
}

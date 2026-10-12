package gohoo

import (
	"math"
	"reflect"
	"testing"
	"time"
)

// sampleSeries arma la misma serie como []Bar y como []OHLCV (60 barras con subidas y bajadas).
func sampleSeries() ([]Bar, []OHLCV) {
	bars := make([]Bar, 60)
	ohlcv := make([]OHLCV, 60)
	t0 := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	for i := range bars {
		c := 100 + 10*math.Sin(float64(i)/5) + float64(i)/3
		h, l, o, v := c+1.5, c-1.2, c-0.3, 1000+float64(i%7)*150
		bars[i] = Bar{Time: t0.AddDate(0, 0, i), Open: o, High: h, Low: l, Close: c, AdjClose: c * 0.98, Volume: v}
		ohlcv[i] = OHLCV{Time: bars[i].Time, Open: o, High: h, Low: l, Close: c, AdjClose: c * 0.98, Volume: v}
	}
	return bars, ohlcv
}

func sameFloats(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if math.IsNaN(a[i]) != math.IsNaN(b[i]) || (!math.IsNaN(a[i]) && math.Abs(a[i]-b[i]) > 1e-9) {
			return false
		}
	}
	return true
}

func TestComputeIndicatorsBarsMatchesComputeIndicators(t *testing.T) {
	bars, ohlcv := sampleSeries()
	got, want := ComputeIndicatorsBars(bars), ComputeIndicators(ohlcv)
	if len(got) != len(want) {
		t.Fatalf("largo %d vs %d", len(got), len(want))
	}
	for i := range got {
		g, w := reflect.ValueOf(got[i]), reflect.ValueOf(want[i])
		for f := 0; f < g.NumField(); f++ {
			gv, wv := g.Field(f).Interface(), w.Field(f).Interface()
			if gf, ok := gv.(float64); ok {
				if !sameFloats([]float64{gf}, []float64{wv.(float64)}) {
					t.Fatalf("barra %d, %s: %v vs %v", i, g.Type().Field(f).Name, gv, wv)
				}
			} else if gv != wv {
				t.Fatalf("barra %d, %s: %v vs %v", i, g.Type().Field(f).Name, gv, wv)
			}
		}
	}
	if !math.IsNaN(got[59].ADX14) && got[59].ADX14 < 0 {
		t.Error("ADX fuera de rango")
	}
}

func TestStandardPeriodsMatchIndicatorSet(t *testing.T) {
	bars, _ := sampleSeries()
	set := ComputeIndicatorsBars(bars)
	h, l, c, v := Highs(bars), Lows(bars), Closes(bars), Volumes(bars)
	pick := func(f func(IndicatorSet) float64) []float64 {
		out := make([]float64, len(set))
		for i, s := range set {
			out[i] = f(s)
		}
		return out
	}
	line, sig, hist := MACD(c, 12, 26, 9)
	up, mid, lo := Bollinger(c, 20, 2)
	k, d := Stochastic(h, l, c, 14, 3)
	checks := map[string][2][]float64{
		"SMA20":      {SMA(c, 20), pick(func(s IndicatorSet) float64 { return s.SMA20 })},
		"EMA12":      {EMA(c, 12), pick(func(s IndicatorSet) float64 { return s.EMA12 })},
		"RSI14":      {RSI(c, 14), pick(func(s IndicatorSet) float64 { return s.RSI14 })},
		"MACD":       {line, pick(func(s IndicatorSet) float64 { return s.MACD })},
		"MACDSignal": {sig, pick(func(s IndicatorSet) float64 { return s.MACDSignal })},
		"MACDHist":   {hist, pick(func(s IndicatorSet) float64 { return s.MACDHist })},
		"BBUpper":    {up, pick(func(s IndicatorSet) float64 { return s.BBUpper })},
		"BBMiddle":   {mid, pick(func(s IndicatorSet) float64 { return s.BBMiddle })},
		"BBLower":    {lo, pick(func(s IndicatorSet) float64 { return s.BBLower })},
		"ATR14":      {ATR(h, l, c, 14), pick(func(s IndicatorSet) float64 { return s.ATR14 })},
		"StochK":     {k, pick(func(s IndicatorSet) float64 { return s.StochK })},
		"StochD":     {d, pick(func(s IndicatorSet) float64 { return s.StochD })},
		"WilliamsR":  {WilliamsR(h, l, c, 14), pick(func(s IndicatorSet) float64 { return s.WilliamsR14 })},
		"ADX14":      {ADX(h, l, c, 14), pick(func(s IndicatorSet) float64 { return s.ADX14 })},
		"OBV":        {OBV(c, v), pick(func(s IndicatorSet) float64 { return s.OBV })},
		"VWAP":       {VWAP(h, l, c, v), pick(func(s IndicatorSet) float64 { return s.VWAP })},
	}
	for name, pair := range checks {
		if !sameFloats(pair[0], pair[1]) {
			t.Errorf("%s no coincide con IndicatorSet", name)
		}
	}
	cross := Crossovers(SMA(c, 20), EMA(c, 26))
	for i, s := range set {
		want := map[string]int{"": 0, "Golden Cross": 1, "Death Cross": -1}[s.CrossSignal]
		if cross[i] != want {
			t.Fatalf("Crossovers[%d] = %d, señal %q", i, cross[i], s.CrossSignal)
		}
	}
}

func TestIndicatorsKnownValues(t *testing.T) {
	v := []float64{1, 2, 3, 4, 5, 6}
	if got := SMA(v, 3); !sameFloats(got, []float64{math.NaN(), math.NaN(), 2, 3, 4, 5}) {
		t.Errorf("SMA = %v", got)
	}
	// EMA(3): semilla SMA(1,2,3)=2, k=0,5 → 3, 4, 5.
	if got := EMA(v, 3); !sameFloats(got, []float64{math.NaN(), math.NaN(), 2, 3, 4, 5}) {
		t.Errorf("EMA = %v", got)
	}
	// Siempre sube: RSI 100; constante: 50.
	if got := RSI(v, 3); !math.IsNaN(got[2]) || got[3] != 100 || got[5] != 100 {
		t.Errorf("RSI = %v", got)
	}
	if got := RSI([]float64{5, 5, 5, 5}, 2); got[2] != 50 {
		t.Errorf("RSI constante = %v", got)
	}
	up, mid, lo := Bollinger([]float64{1, 3, 1, 3}, 2, 2)
	if mid[1] != 2 || up[1] != 4 || lo[1] != 0 { // desviación poblacional 1
		t.Errorf("Bollinger = %v %v %v", up, mid, lo)
	}
	if got := OBV([]float64{1, 2, 2, 1}, []float64{10, 20, 30, 40}); !sameFloats(got, []float64{0, 20, 20, -20}) {
		t.Errorf("OBV = %v", got)
	}
	if got := Crossovers([]float64{1, 3, 1}, []float64{2, 2, 2}); !reflect.DeepEqual(got, []int{0, 1, -1}) {
		t.Errorf("Crossovers = %v", got)
	}
}

func TestIndicatorsValidation(t *testing.T) {
	v := []float64{1, 2, 3}
	for name, out := range map[string][]float64{
		"SMA": SMA(v, 0), "EMA": EMA(v, -1), "RSI": RSI(v, 0), "ATR": ATR(v, v, v, 0),
		"WilliamsR": WilliamsR(v, v, v, 0), "ADX": ADX(v, v, v, 0),
	} {
		if len(out) != 3 || !math.IsNaN(out[0]) || !math.IsNaN(out[2]) {
			t.Errorf("%s con período < 1 = %v", name, out)
		}
	}
	if l, s, h := MACD(v, 0, 26, 9); len(l) != 3 || !math.IsNaN(s[2]) || !math.IsNaN(h[2]) {
		t.Error("MACD con período < 1")
	}
	if u, _, _ := Bollinger(v, 0, 2); !math.IsNaN(u[0]) {
		t.Error("Bollinger con período < 1")
	}
	if k, d := Stochastic(v, v, v, 0, 3); !math.IsNaN(k[0]) || !math.IsNaN(d[0]) {
		t.Error("Stochastic con período < 1")
	}
	short := []float64{1, 2}
	if ATR(v, short, v, 14) != nil || ADX(v, v, short, 14) != nil || WilliamsR(short, v, v, 14) != nil ||
		OBV(v, short) != nil || VWAP(v, v, v, short) != nil || Crossovers(v, short) != nil {
		t.Error("largos distintos deben devolver nil")
	}
	if k, d := Stochastic(v, short, v, 14, 3); k != nil || d != nil {
		t.Error("Stochastic con largos distintos")
	}
	if len(ComputeIndicatorsBars(nil)) != 0 || len(SMA(nil, 3)) != 0 {
		t.Error("entrada vacía")
	}
}

func TestColumns(t *testing.T) {
	bars := []Bar{{Open: 1, High: 2, Low: 0.5, Close: 1.5, AdjClose: 1.4, Volume: 9}}
	if Opens(bars)[0] != 1 || Highs(bars)[0] != 2 || Lows(bars)[0] != 0.5 || Closes(bars)[0] != 1.5 ||
		AdjCloses(bars)[0] != 1.4 || Volumes(bars)[0] != 9 {
		t.Error("columnas")
	}
}

func TestReturnsAndLogReturns(t *testing.T) {
	v := []float64{100, 110, 99, math.NaN(), 120, 0, 50}
	r := Returns(v)
	if !math.IsNaN(r[0]) || math.Abs(r[1]-0.1) > 1e-12 || math.Abs(r[2]-(-0.1)) > 1e-12 ||
		!math.IsNaN(r[3]) || !math.IsNaN(r[4]) || r[5] != -1 || !math.IsNaN(r[6]) {
		t.Errorf("Returns = %v", r)
	}
	lr := LogReturns(v)
	if math.Abs(lr[1]-math.Log(1.1)) > 1e-12 || !math.IsNaN(lr[5]) { // cociente 0: NaN
		t.Errorf("LogReturns = %v", lr)
	}
	if len(Returns(nil)) != 0 || len(Returns([]float64{1})) != 1 {
		t.Error("bordes")
	}
}

func TestVolatility(t *testing.T) {
	// retornos 0,01 y -0,01: media 0, desviación muestral sqrt(0,0002) ≈ 0,014142.
	r := []float64{math.NaN(), 0.01, -0.01}
	if got := Volatility(r, 1); math.Abs(got-math.Sqrt(0.0002)) > 1e-12 {
		t.Errorf("Volatility = %v", got)
	}
	if got := Volatility(r, 252); math.Abs(got-math.Sqrt(0.0002)*math.Sqrt(252)) > 1e-12 {
		t.Errorf("anualizada = %v", got)
	}
	if !math.IsNaN(Volatility([]float64{0.01}, 252)) || !math.IsNaN(Volatility(r, 0)) {
		t.Error("menos de dos retornos o periodsPerYear <= 0 deben dar NaN")
	}
}

func TestMaxDrawdown(t *testing.T) {
	dd, peak, trough := MaxDrawdown([]float64{100, 120, math.NaN(), 90, 130, 104, 125})
	if math.Abs(dd-(-0.25)) > 1e-12 || peak != 1 || trough != 3 {
		t.Errorf("dd=%v peak=%d trough=%d", dd, peak, trough)
	}
	if dd, p, tr := MaxDrawdown([]float64{1, 2, 3}); dd != 0 || p != -1 || tr != -1 {
		t.Errorf("sin caídas: %v %d %d", dd, p, tr)
	}
	if dd, _, _ := MaxDrawdown([]float64{math.NaN()}); !math.IsNaN(dd) {
		t.Error("sin datos válidos debe dar NaN")
	}
}

func TestMaxDrawdownNonPositivePeak(t *testing.T) {
	if dd, p, tr := MaxDrawdown([]float64{0, -1, -2}); dd != 0 || p != -1 || tr != -1 {
		t.Errorf("máximo <= 0: dd=%v peak=%d trough=%d, want 0 -1 -1", dd, p, tr)
	}
}

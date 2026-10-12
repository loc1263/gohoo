package gohoo

import (
	"math"
	"time"
)

// AutoAdjust devuelve una copia de bars ajustada por dividendos y splits,
// como auto_adjust=True de yfinance: Close = AdjClose y Open, High y Low se
// escalan con el mismo factor AdjClose/Close. Si Close es 0 o NaN, o
// AdjClose es NaN, el factor es 1. Time y Volume no cambian; bars no se
// modifica.
func AutoAdjust(bars []Bar) []Bar {
	out := make([]Bar, len(bars))
	for i, b := range bars {
		f := 1.0
		if b.Close != 0 && !math.IsNaN(b.Close) && !math.IsNaN(b.AdjClose) {
			f = b.AdjClose / b.Close
		}
		out[i] = Bar{Time: b.Time, Open: b.Open * f, High: b.High * f, Low: b.Low * f,
			Close: b.AdjClose, AdjClose: b.AdjClose, Volume: b.Volume}
	}
	return out
}

// AutoAdjusted es AutoAdjust(r.Bars).
func (r *ChartResult) AutoAdjusted() []Bar { return AutoAdjust(r.Bars) }

// Date es la fecha de la barra: medianoche del día de Time en su misma zona
// (la del exchange en las barras de ChartRange).
func (b Bar) Date() time.Time {
	y, m, d := b.Time.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, b.Time.Location())
}

// AtMidnight devuelve una copia de bars con cada Time llevado a Date(), para
// indexar velas diarias, semanales o mensuales por fecha (como yfinance) y
// compararlas por día. No usar con barras intradía: varias quedarían con la
// misma hora. bars no se modifica.
func AtMidnight(bars []Bar) []Bar {
	out := make([]Bar, len(bars))
	for i, b := range bars {
		b.Time = b.Date()
		out[i] = b
	}
	return out
}

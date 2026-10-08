package gohoo

import (
	"sort"
	"time"
)

// Monthly devuelve una observación por mes calendario: la fecha es el día 1
// a medianoche UTC del mes en que cae la barra según la zona del exchange
// (Bar.Time), y el valor es su AdjClose. Si hay dos barras en el mismo mes
// (típicamente la mensual y la "en vivo" del mes en curso), gana la de Time
// más reciente; a igual Time, la que aparece después en Bars. El resultado
// queda en orden cronológico.
func (r *ChartResult) Monthly() (dates []time.Time, adj []float64) {
	if r == nil || len(r.Bars) == 0 {
		return nil, nil
	}
	type obs struct {
		t   time.Time
		adj float64
	}
	byMonth := make(map[time.Time]obs, len(r.Bars))
	for _, b := range r.Bars {
		y, m, _ := b.Time.Date()
		key := time.Date(y, m, 1, 0, 0, 0, 0, time.UTC)
		if prev, ok := byMonth[key]; ok && b.Time.Before(prev.t) {
			continue
		}
		byMonth[key] = obs{t: b.Time, adj: b.AdjClose}
	}

	dates = make([]time.Time, 0, len(byMonth))
	for k := range byMonth {
		dates = append(dates, k)
	}
	sort.Slice(dates, func(i, j int) bool { return dates[i].Before(dates[j]) })
	adj = make([]float64, len(dates))
	for i, d := range dates {
		adj[i] = byMonth[d].adj
	}
	return dates, adj
}

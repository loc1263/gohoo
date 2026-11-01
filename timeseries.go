package gohoo

import (
	"context"
	"sort"
)

// Point es una observación de fundamentals-timeseries.
type Point struct {
	Date       string  // asOfDate, "YYYY-MM-DD"
	PeriodType string  // "3M", "12M", "TTM"...
	Currency   string  // currencyCode (vacío en ratios como trailingPegRatio)
	Value      float64 // reportedValue.raw
}

// TimeseriesPoints extrae de una respuesta de FundamentalsTimeseries las
// observaciones del tipo dado (p. ej. "quarterlyTotalRevenue"), la más
// reciente primero y sin las que no traen fecha o valor. Devuelve nil si el
// tipo no está en la respuesta.
func TimeseriesPoints(data map[string]any, typ string) []Point {
	var out []Point
	res, _ := GetPath(data, "timeseries", "result").([]any)
	for _, it := range res {
		m, _ := it.(map[string]any)
		arr, _ := m[typ].([]any)
		for _, e := range arr {
			em, _ := e.(map[string]any)
			if em == nil {
				continue
			}
			date, _ := em["asOfDate"].(string)
			v, ok := AsFloat(GetPath(em, "reportedValue", "raw"))
			if date == "" || !ok {
				continue
			}
			pt, _ := em["periodType"].(string)
			cur, _ := em["currencyCode"].(string)
			out = append(out, Point{Date: date, PeriodType: pt, Currency: cur, Value: v})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	return out
}

// LatestPoint devuelve la observación más reciente de points (la primera,
// como las deja TimeseriesPoints). ok es false si points está vacío.
func LatestPoint(points []Point) (p Point, ok bool) {
	if len(points) == 0 {
		return Point{}, false
	}
	return points[0], true
}

// PointOn devuelve la observación de points con la fecha dada ("YYYY-MM-DD",
// la de asOfDate). Sirve para alinear varias series por fecha de cierre. ok es
// false si no hay una observación en esa fecha.
func PointOn(points []Point, date string) (p Point, ok bool) {
	for _, pt := range points {
		if pt.Date == date {
			return pt, true
		}
	}
	return Point{}, false
}

// FilterPeriodType devuelve las observaciones de points cuyo PeriodType es
// periodType ("3M" trimestral, "12M" anual, "TTM"...), en el mismo orden. No
// modifica points. Devuelve nil si ninguna coincide.
func FilterPeriodType(points []Point, periodType string) []Point {
	var out []Point
	for _, pt := range points {
		if pt.PeriodType == periodType {
			out = append(out, pt)
		}
	}
	return out
}

// SumLast suma el valor de las n observaciones más recientes de points (las
// primeras, como las deja TimeseriesPoints); con series trimestrales y n = 4
// da el acumulado de 12 meses (TTM). ok es false si n < 1 o points tiene
// menos de n observaciones.
func SumLast(points []Point, n int) (sum float64, ok bool) {
	if n < 1 || len(points) < n {
		return 0, false
	}
	for _, pt := range points[:n] {
		sum += pt.Value
	}
	return sum, true
}

// TrailingPEG devuelve el último trailingPegRatio de ticker, que Yahoo
// publica en fundamentals-timeseries y no en quoteSummary (yfinance lo
// agrega a Ticker.info desde ahí). ok es false si Yahoo no tiene el dato.
func (c *Client) TrailingPEG(ctx context.Context, ticker string) (v float64, ok bool, err error) {
	data, err := c.FundamentalsTimeseriesContext(ctx, ticker, []string{"trailingPegRatio"})
	if err != nil {
		return 0, false, err
	}
	pts := TimeseriesPoints(data, "trailingPegRatio")
	if len(pts) == 0 {
		return 0, false, nil
	}
	return pts[0].Value, true, nil
}

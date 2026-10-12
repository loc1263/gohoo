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

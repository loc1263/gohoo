package gohoo

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// OHLCV es una barra de precio/volumen del endpoint "chart", con su marca de
// tiempo completa (la granularidad depende del intervalo pedido).
type OHLCV struct {
	Time                                     time.Time
	Open, High, Low, Close, AdjClose, Volume interface{}
}

// FirstResult devuelve el primer elemento de chart.result, tal como lo
// entrega el endpoint v8/finance/chart.
func FirstResult(chart map[string]interface{}) (map[string]interface{}, bool) {
	c, ok := chart["chart"].(map[string]interface{})
	if !ok {
		return nil, false
	}
	results, ok := c["result"].([]interface{})
	if !ok || len(results) == 0 {
		return nil, false
	}
	r, ok := results[0].(map[string]interface{})
	return r, ok
}

// ChartSeries descarga el chart de `ticker` en el rango e intervalo dados y
// lo devuelve como una serie OHLCV ordenada cronológicamente, junto con los
// dividendos y splits del período indexados por fecha ("YYYY-MM-DD"). Sirve
// tanto para acciones como para criptomonedas (que normalmente no traen
// dividendos ni splits, por lo que esos mapas quedarán vacíos).
func (c *Client) ChartSeries(ticker, rang, interval string) ([]OHLCV, map[string]float64, map[string]float64, error) {
	return c.ChartSeriesContext(context.Background(), ticker, rang, interval)
}

// ChartSeriesContext es ChartSeries con cancelación y plazos por ctx.
func (c *Client) ChartSeriesContext(ctx context.Context, ticker, rang, interval string) ([]OHLCV, map[string]float64, map[string]float64, error) {
	chart, err := c.ChartContext(ctx, ticker, rang, interval)
	if err != nil {
		return nil, nil, nil, err
	}
	result, ok := FirstResult(chart)
	if !ok {
		return nil, nil, nil, fmt.Errorf("respuesta de chart vacía o inválida para %s", ticker)
	}

	timestamps, _ := result["timestamp"].([]interface{})
	indicators, _ := result["indicators"].(map[string]interface{})
	if indicators == nil || len(timestamps) == 0 {
		return nil, nil, nil, fmt.Errorf("respuesta de chart vacía o inválida para %s", ticker)
	}
	quoteArr, _ := indicators["quote"].([]interface{})
	if len(quoteArr) == 0 {
		return nil, nil, nil, fmt.Errorf("respuesta de chart vacía o inválida para %s", ticker)
	}
	quote, _ := quoteArr[0].(map[string]interface{})

	var adjCloses []interface{}
	if adjArr, _ := indicators["adjclose"].([]interface{}); len(adjArr) > 0 {
		if adj, ok := adjArr[0].(map[string]interface{}); ok {
			adjCloses, _ = adj["adjclose"].([]interface{})
		}
	}

	opens := toInterfaceSlice(quote["open"])
	highs := toInterfaceSlice(quote["high"])
	lows := toInterfaceSlice(quote["low"])
	closes := toInterfaceSlice(quote["close"])
	volumes := toInterfaceSlice(quote["volume"])

	series := make([]OHLCV, 0, len(timestamps))
	for i, tsRaw := range timestamps {
		f, ok := AsFloat(tsRaw)
		if !ok {
			continue
		}
		var adjClose interface{}
		if i < len(adjCloses) {
			adjClose = adjCloses[i]
		}
		series = append(series, OHLCV{
			Time:     time.Unix(int64(f), 0).UTC(),
			Open:     valOrNil(opens, i),
			High:     valOrNil(highs, i),
			Low:      valOrNil(lows, i),
			Close:    valOrNil(closes, i),
			AdjClose: adjClose,
			Volume:   valOrNil(volumes, i),
		})
	}

	dividends := map[string]float64{}
	splits := map[string]float64{}
	if events, ok := result["events"].(map[string]interface{}); ok {
		if divs, ok := events["dividends"].(map[string]interface{}); ok {
			for _, dRaw := range divs {
				d, _ := dRaw.(map[string]interface{})
				if d == nil {
					continue
				}
				date := EpochToDate(d["date"])
				amt, _ := AsFloat(d["amount"])
				dividends[date] = amt
			}
		}
		if sp, ok := events["splits"].(map[string]interface{}); ok {
			for _, sRaw := range sp {
				s, _ := sRaw.(map[string]interface{})
				if s == nil {
					continue
				}
				date := EpochToDate(s["date"])
				num, _ := AsFloat(s["numerator"])
				den, _ := AsFloat(s["denominator"])
				if den != 0 {
					splits[date] = num / den
				}
			}
		}
	}

	return series, dividends, splits, nil
}

// Gap describe un hueco inusual entre dos barras consecutivas de una serie
// OHLCV: la diferencia de tiempo entre ellas (Actual) es mayor que
// `factor` veces el intervalo esperado de la serie (Esperado).
type Gap struct {
	From, To       time.Time
	Esperado, Real time.Duration
}

// DetectGaps revisa que las barras de `series` (ya ordenada cronológicamente)
// estén separadas por aproximadamente el intervalo esperado, inferido como
// la mediana de los deltas de tiempo entre barras consecutivas (robusto
// frente a fines de semana/feriados, que son la minoría de los deltas en
// datos diarios), y devuelve los huecos que superan `factor` veces esa
// mediana. Un `factor` de 5 es un punto de partida razonable para velas
// diarias (tolera fines de semana largos y feriados consecutivos sin
// generar falsos positivos); para velas intradía puede necesitarse un
// factor menor.
func DetectGaps(series []OHLCV, factor float64) []Gap {
	n := len(series)
	if n < 2 {
		return nil
	}
	deltas := make([]time.Duration, n-1)
	for i := 1; i < n; i++ {
		deltas[i-1] = series[i].Time.Sub(series[i-1].Time)
	}
	median := medianDuration(deltas)
	if median <= 0 {
		return nil
	}

	var gaps []Gap
	for i, d := range deltas {
		if float64(d) > factor*float64(median) {
			gaps = append(gaps, Gap{
				From: series[i].Time, To: series[i+1].Time,
				Esperado: median, Real: d,
			})
		}
	}
	return gaps
}

func medianDuration(deltas []time.Duration) time.Duration {
	if len(deltas) == 0 {
		return 0
	}
	sorted := make([]time.Duration, len(deltas))
	copy(sorted, deltas)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	mid := len(sorted) / 2
	if len(sorted)%2 == 0 {
		return (sorted[mid-1] + sorted[mid]) / 2
	}
	return sorted[mid]
}

func toInterfaceSlice(v interface{}) []interface{} {
	arr, _ := v.([]interface{})
	return arr
}

func valOrNil(arr []interface{}, i int) interface{} {
	if i < 0 || i >= len(arr) {
		return nil
	}
	return arr[i]
}

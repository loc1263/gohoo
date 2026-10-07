package yahoo

import (
	"fmt"
	"time"
)

// OHLCV es una barra de precio/volumen del endpoint "chart", con su marca de
// tiempo completa (la granularidad depende del intervalo pedido).
type OHLCV struct {
	Time                                      time.Time
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
	chart, err := c.Chart(ticker, rang, interval)
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

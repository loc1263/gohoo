package gohoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ChartParams son los parámetros de ChartRange. Se indica Range o bien
// Start (y opcionalmente End), nunca ambos.
type ChartParams struct {
	// Range es un rango relativo de Yahoo ("1mo", "5y", "max"...).
	Range string
	// Start es el inicio del período (period1). Si no es cero se ignora
	// Range y se envían period1/period2.
	Start time.Time
	// End es el fin del período (period2); cero = ahora. Requiere Start.
	End time.Time
	// Interval es la granularidad de las barras ("1d", "1wk", "1mo"...);
	// vacío = "1d".
	Interval string
	// Events es el parámetro events de Yahoo; vacío = "div,splits".
	Events string
}

// Bar es una barra de precio con valores tipados.
//
// Criterio para los null de Yahoo: si la respuesta trae la serie adjclose,
// la barra se descarta cuando su AdjClose es null (nunca se mezcla un
// cierre sin ajustar en una serie ajustada); si la respuesta no trae
// adjclose en absoluto, AdjClose = Close y se descartan las barras con
// Close null. Cualquier otro campo null (Open, High, Low, Close o Volume)
// queda en math.NaN(), nunca en 0.
type Bar struct {
	// Time está expresado en la zona horaria del exchange (ChartMeta.Location).
	Time                                     time.Time
	Open, High, Low, Close, AdjClose, Volume float64
}

// ChartMeta son los metadatos del símbolo que devuelve chart.
type ChartMeta struct {
	Symbol, Currency, InstrumentType string
	ExchangeTimezoneName             string
	// Location es la zona de ExchangeTimezoneName, o UTC si no se pudo
	// cargar. En Windows sin Go instalado hace falta que la aplicación
	// importe _ "time/tzdata" (gohoo no lo hace para no agrandar binarios).
	Location *time.Location
}

// ChartResult es el resultado tipado de ChartRange.
type ChartResult struct {
	Meta ChartMeta
	// Bars en orden cronológico, sin las barras cuyo AdjClose es null (ver
	// Bar).
	Bars []Bar
	// Dividends y Splits indexados por fecha "YYYY-MM-DD" (UTC), con el
	// mismo criterio que ChartSeries.
	Dividends map[string]float64
	Splits    map[string]float64
}

// chartResponse es la forma del JSON de v8/finance/chart; usa punteros para
// distinguir null de 0.
type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				Symbol               string `json:"symbol"`
				Currency             string `json:"currency"`
				InstrumentType       string `json:"instrumentType"`
				ExchangeTimezoneName string `json:"exchangeTimezoneName"`
			} `json:"meta"`
			Timestamp  []*int64               `json:"timestamp"`
			Events     map[string]interface{} `json:"events"`
			Indicators struct {
				Quote []struct {
					Open   []*float64 `json:"open"`
					High   []*float64 `json:"high"`
					Low    []*float64 `json:"low"`
					Close  []*float64 `json:"close"`
					Volume []*float64 `json:"volume"`
				} `json:"quote"`
				AdjClose []struct {
					AdjClose []*float64 `json:"adjclose"`
				} `json:"adjclose"`
			} `json:"indicators"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"chart"`
}

// ChartRange descarga el chart de ticker por rango relativo o por fechas
// (period1/period2) y lo devuelve tipado. A diferencia de Chart, un
// {"chart":{"error":...}} de Yahoo se devuelve como error clasificable:
// errors.Is(err, ErrNotFound) para símbolos sin datos y
// errors.Is(err, ErrRateLimit) para "Too Many Requests", incluso con
// status 200.
func (c *Client) ChartRange(ctx context.Context, ticker string, p ChartParams) (*ChartResult, error) {
	query, err := p.query(time.Now())
	if err != nil {
		return nil, fmt.Errorf("chart de %s: %w", ticker, err)
	}
	body, err := c.doAuthedCheck(ctx, http.MethodGet, func() string {
		return fmt.Sprintf("%s/v8/finance/chart/%s?%s", c.baseURL(), url.PathEscape(ticker), query)
	}, nil, func(b []byte) error {
		if parsed, ok := parseChartBody(b); ok && parsed.Chart.Error != nil {
			return chartAPIError(ticker, parsed.Chart.Error.Code, parsed.Chart.Error.Description)
		}
		return nil
	})
	if err != nil {
		// Un 404 de chart suele traer el detalle en el cuerpo JSON.
		var statusErr *StatusError
		if errors.As(err, &statusErr) {
			if parsed, ok := parseChartBody([]byte(statusErr.Body)); ok && parsed.Chart.Error != nil {
				return nil, fmt.Errorf("%w (%w)", chartAPIError(ticker, parsed.Chart.Error.Code, parsed.Chart.Error.Description), err)
			}
		}
		return nil, err
	}
	parsed, ok := parseChartBody(body)
	if !ok {
		return nil, fmt.Errorf("no se pudo parsear chart de %s", ticker)
	}
	return parsed.toResult(ticker)
}

func parseChartBody(body []byte) (*chartResponse, bool) {
	var parsed chartResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, false
	}
	return &parsed, true
}

// query arma la query string de chart, validando los parámetros antes de
// tocar la red.
func (p ChartParams) query(now time.Time) (string, error) {
	q := url.Values{}
	interval := p.Interval
	if interval == "" {
		interval = "1d"
	}
	events := p.Events
	if events == "" {
		events = "div,splits"
	}
	switch {
	case !p.Start.IsZero() && p.Range != "":
		return "", errors.New("ChartParams: Range y Start/End son excluyentes")
	case !p.Start.IsZero():
		end := p.End
		if end.IsZero() {
			end = now
		}
		if !end.After(p.Start) {
			return "", fmt.Errorf("ChartParams: End (%s) debe ser posterior a Start (%s)", end.Format(time.RFC3339), p.Start.Format(time.RFC3339))
		}
		q.Set("period1", fmt.Sprint(p.Start.Unix()))
		q.Set("period2", fmt.Sprint(end.Unix()))
	case !p.End.IsZero():
		return "", errors.New("ChartParams: End requiere Start")
	case p.Range != "":
		q.Set("range", p.Range)
	default:
		return "", errors.New("ChartParams: falta Range o Start")
	}
	q.Set("interval", interval)
	q.Set("events", events)
	return q.Encode(), nil
}

func (r *chartResponse) toResult(ticker string) (*ChartResult, error) {
	if e := r.Chart.Error; e != nil {
		return nil, chartAPIError(ticker, e.Code, e.Description)
	}
	if len(r.Chart.Result) == 0 {
		return nil, fmt.Errorf("respuesta de chart vacía para %s: %w", ticker, ErrNotFound)
	}
	res := r.Chart.Result[0]

	loc := loadLocation(res.Meta.ExchangeTimezoneName)
	out := &ChartResult{
		Meta: ChartMeta{
			Symbol:               res.Meta.Symbol,
			Currency:             res.Meta.Currency,
			InstrumentType:       res.Meta.InstrumentType,
			ExchangeTimezoneName: res.Meta.ExchangeTimezoneName,
			Location:             loc,
		},
	}
	out.Dividends, out.Splits = parseEvents(res.Events)

	var opens, highs, lows, closes, volumes, adjs []*float64
	if len(res.Indicators.Quote) > 0 {
		q := res.Indicators.Quote[0]
		opens, highs, lows, closes, volumes = q.Open, q.High, q.Low, q.Close, q.Volume
	}
	if len(res.Indicators.AdjClose) > 0 {
		adjs = res.Indicators.AdjClose[0].AdjClose
	}
	// Sin serie ajustada (p. ej. algunos índices) se usa el cierre para toda
	// la serie; con serie ajustada, nunca se completa un hueco con Close.
	if len(adjs) == 0 {
		adjs = closes
	}

	out.Bars = make([]Bar, 0, len(res.Timestamp))
	for i, ts := range res.Timestamp {
		if ts == nil {
			continue
		}
		cl, adj := at(closes, i), at(adjs, i)
		if adj == nil {
			continue
		}
		out.Bars = append(out.Bars, Bar{
			Time:     time.Unix(*ts, 0).In(loc),
			Open:     orNaN(at(opens, i)),
			High:     orNaN(at(highs, i)),
			Low:      orNaN(at(lows, i)),
			Close:    orNaN(cl),
			AdjClose: *adj,
			Volume:   orNaN(at(volumes, i)),
		})
	}
	return out, nil
}

func at(arr []*float64, i int) *float64 {
	if i < 0 || i >= len(arr) {
		return nil
	}
	return arr[i]
}

func orNaN(v *float64) float64 {
	if v == nil {
		return math.NaN()
	}
	return *v
}

var locCache sync.Map // nombre de zona -> *time.Location

// loadLocation carga (y cachea) la zona del exchange; UTC si el nombre está
// vacío o no se encuentra la base de zonas horarias.
func loadLocation(name string) *time.Location {
	if name == "" {
		return time.UTC
	}
	if l, ok := locCache.Load(name); ok {
		return l.(*time.Location)
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.UTC
	}
	locCache.Store(name, loc)
	return loc
}

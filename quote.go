package gohoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// QuoteBatchSize es cuántos símbolos pide Quotes en cada request a Yahoo.
const QuoteBatchSize = 100

// Quote es la cotización de un símbolo según /v7/finance/quote. Los campos
// tipados son los más usados (cero si Yahoo no los manda); Raw trae todos
// los campos tal cual.
type Quote struct {
	Symbol           string
	ShortName        string
	LongName         string
	QuoteType        string // "EQUITY", "ETF", "CURRENCY", "INDEX", "CRYPTOCURRENCY"...
	Exchange         string
	FullExchangeName string
	Currency         string // moneda de Price y MarketCap
	MarketState      string // "REGULAR", "PRE", "POST", "CLOSED"...

	Price         float64 // regularMarketPrice
	PreviousClose float64 // regularMarketPreviousClose
	Change        float64 // regularMarketChange
	ChangePercent float64 // regularMarketChangePercent, en porcentaje
	Open          float64 // regularMarketOpen
	DayHigh       float64 // regularMarketDayHigh
	DayLow        float64 // regularMarketDayLow
	Volume        float64 // regularMarketVolume
	MarketCap     float64 // en Currency; 0 en índices, monedas y la mayoría de los ETF

	// Time es regularMarketTime en la zona del exchange (UTC si no se
	// conoce la zona); cero si Yahoo no lo manda.
	Time time.Time

	Raw map[string]any
}

// Quotes pide la cotización de varios símbolos con /v7/finance/quote, en
// lotes de QuoteBatchSize (un request por lote, en serie). Los símbolos
// vacíos y repetidos se descartan; los que Yahoo no conoce no aparecen en
// el resultado (no es un error). El orden es el de Yahoo: para buscar por
// símbolo, indexar por Quote.Symbol.
//
// Si un lote falla, devuelve las cotizaciones de los lotes anteriores junto
// con el error.
func (c *Client) Quotes(ctx context.Context, symbols []string) ([]Quote, error) {
	syms := uniqueSymbols(symbols)
	var out []Quote
	for start := 0; start < len(syms); start += QuoteBatchSize {
		batch := syms[start:min(start+QuoteBatchSize, len(syms))]
		qs, err := c.quoteBatch(ctx, batch)
		out = append(out, qs...)
		if err != nil {
			return out, err
		}
	}
	return out, nil
}

func uniqueSymbols(symbols []string) []string {
	seen := make(map[string]bool, len(symbols))
	out := make([]string, 0, len(symbols))
	for _, s := range symbols {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}

func (c *Client) quoteBatch(ctx context.Context, batch []string) ([]Quote, error) {
	what := "quote"
	if len(batch) == 1 {
		what = "quote de " + batch[0]
	}
	q := url.Values{"symbols": {strings.Join(batch, ",")}, "formatted": {"false"}, "lang": {"en-US"}, "region": {"US"}}
	raw, err := c.authedRequest(ctx, http.MethodGet, "/v7/finance/quote", q, nil, financeRateLimit(what))
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) {
			var r struct {
				Finance struct {
					Error *struct{ Code, Description string } `json:"error"`
				} `json:"finance"`
			}
			if json.Unmarshal([]byte(se.Body), &r) == nil && r.Finance.Error != nil {
				return nil, fmt.Errorf("%w (%w)", financeAPIError(what, r.Finance.Error.Code, r.Finance.Error.Description), err)
			}
		}
		return nil, err
	}
	var r struct {
		QuoteResponse struct {
			Result []map[string]any `json:"result"`
			Error  *struct {
				Code        string `json:"code"`
				Description string `json:"description"`
			} `json:"error"`
		} `json:"quoteResponse"`
	}
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("gohoo: no se pudo parsear la respuesta de %s: %w", what, err)
	}
	if e := r.QuoteResponse.Error; e != nil {
		return nil, financeAPIError(what, e.Code, e.Description)
	}
	out := make([]Quote, 0, len(r.QuoteResponse.Result))
	for _, m := range r.QuoteResponse.Result {
		out = append(out, newQuote(m))
	}
	return out, nil
}

func newQuote(m map[string]any) Quote {
	for k, v := range m {
		m[k] = RawValue(v)
	}
	num := func(k string) float64 {
		f, _ := AsFloat(m[k])
		return f
	}
	str := func(k string) string {
		s, _ := m[k].(string)
		return s
	}
	q := Quote{
		Symbol: str("symbol"), ShortName: str("shortName"), LongName: str("longName"),
		QuoteType: str("quoteType"), Exchange: str("exchange"), FullExchangeName: str("fullExchangeName"),
		Currency: str("currency"), MarketState: str("marketState"),
		Price: num("regularMarketPrice"), PreviousClose: num("regularMarketPreviousClose"),
		Change: num("regularMarketChange"), ChangePercent: num("regularMarketChangePercent"),
		Open: num("regularMarketOpen"), DayHigh: num("regularMarketDayHigh"), DayLow: num("regularMarketDayLow"),
		Volume: num("regularMarketVolume"), MarketCap: num("marketCap"),
		Raw: m,
	}
	if t, ok := AsInt64(m["regularMarketTime"]); ok && t > 0 {
		loc := time.UTC
		if tz := str("exchangeTimezoneName"); tz != "" {
			if l, err := time.LoadLocation(tz); err == nil {
				loc = l
			}
		}
		q.Time = time.Unix(t, 0).In(loc)
	}
	return q
}

package gohoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ScreenMaxSize es el máximo de resultados por página que acepta el
// screener de Yahoo.
const ScreenMaxSize = 250

// screenDefaultSize es el tamaño de página cuando ScreenParams.Size es 0.
const screenDefaultSize = 25

// Query es una condición del screener de Yahoo, armada con Eq, Gt, Lt, Gte,
// Lte, Btwn, IsIn, And y Or. Su valor cero es una query vacía, que Screen
// rechaza.
type Query struct {
	op       string
	operands []any
}

// Eq: field == v (p. ej. Eq("region", "us"), Eq("exchange", "NMS")).
func Eq(field string, v any) Query { return Query{"EQ", []any{field, v}} }

// Gt: field > v.
func Gt(field string, v float64) Query { return Query{"GT", []any{field, v}} }

// Lt: field < v.
func Lt(field string, v float64) Query { return Query{"LT", []any{field, v}} }

// Gte: field >= v.
func Gte(field string, v float64) Query { return Query{"GTE", []any{field, v}} }

// Lte: field <= v.
func Lte(field string, v float64) Query { return Query{"LTE", []any{field, v}} }

// Btwn: lo <= field <= hi.
func Btwn(field string, lo, hi float64) Query { return Query{"BTWN", []any{field, lo, hi}} }

// IsIn: field es alguno de vs. Yahoo no tiene un operador "is-in": se arma
// como Or de Eq, igual que yfinance.
func IsIn(field string, vs ...any) Query {
	qs := make([]Query, len(vs))
	for i, v := range vs {
		qs[i] = Eq(field, v)
	}
	return Or(qs...)
}

// And: se cumplen todas las condiciones. Con una sola condición devuelve
// esa misma; sin condiciones, una query vacía.
func And(qs ...Query) Query { return combine("AND", qs) }

// Or: se cumple alguna de las condiciones (mismas reglas que And).
func Or(qs ...Query) Query { return combine("OR", qs) }

func combine(op string, qs []Query) Query {
	switch len(qs) {
	case 0:
		return Query{}
	case 1:
		return qs[0]
	}
	ops := make([]any, len(qs))
	for i, q := range qs {
		ops[i] = q
	}
	return Query{op, ops}
}

// IsZero indica si la query está vacía.
func (q Query) IsZero() bool { return q.op == "" }

// MarshalJSON serializa la query en el formato del screener:
// {"operator":"EQ","operands":["region","us"]}.
func (q Query) MarshalJSON() ([]byte, error) {
	if q.IsZero() {
		return nil, errors.New("gohoo: query vacía en el screener")
	}
	return json.Marshal(struct {
		Operator string `json:"operator"`
		Operands []any  `json:"operands"`
	}{q.op, q.operands})
}

// String devuelve la query en JSON (útil para logs).
func (q Query) String() string {
	b, err := json.Marshal(q)
	if err != nil {
		return "<query vacía>"
	}
	return string(b)
}

// ScreenParams es una consulta al screener (Screen).
type ScreenParams struct {
	// Query es la condición; obligatoria.
	Query Query
	// QuoteType es "EQUITY" (por defecto), "ETF", "MUTUALFUND", etc.
	QuoteType string
	// Offset es el primer resultado a devolver (para paginar).
	Offset int
	// Size es el tamaño de página: 0 = 25, máximo ScreenMaxSize.
	Size int
	// SortField es el campo de orden; vacío = "ticker". Debe existir para
	// el QuoteType ("intradaymarketcap" vale para EQUITY pero no para ETF,
	// donde se usa p. ej. "fundnetassets").
	SortField string
	// SortAsc ordena de menor a mayor; por defecto es descendente.
	SortAsc bool
}

// ScreenQuote es un resultado del screener. Los campos tipados son los más
// usados (cero si Yahoo no los manda); Raw trae todos los campos tal cual,
// con los números ya sin el envoltorio {raw, fmt}.
type ScreenQuote struct {
	Symbol           string
	ShortName        string
	LongName         string
	QuoteType        string
	Exchange         string
	FullExchangeName string
	Region           string
	Currency         string
	MarketCap        float64
	Price            float64 // regularMarketPrice
	Raw              map[string]any
}

// ScreenResult es una página del screener.
type ScreenResult struct {
	Start  int // offset de la página
	Count  int // resultados en esta página
	Total  int // resultados en total para la query
	Quotes []ScreenQuote
}

// ErrEmptyScreen indica que la respuesta del screener no trajo ningún
// bloque de resultados. Una query válida sin coincidencias no es un error:
// devuelve un ScreenResult con Total 0 y sin Quotes.
var ErrEmptyScreen = errors.New("gohoo: respuesta del screener sin resultados")

// screenQuery son los parámetros fijos de query de los dos endpoints del
// screener (formatted=false: números sin el envoltorio {raw, fmt}).
func screenQuery() url.Values {
	return url.Values{
		"corsDomain": {"finance.yahoo.com"},
		"formatted":  {"false"},
		"lang":       {"en-US"},
		"region":     {"US"},
	}
}

// Screen consulta el screener de Yahoo (POST /v1/finance/screener) y
// devuelve una página de resultados. Los parámetros se validan antes de
// tocar la red. Un error de Yahoo en el cuerpo ({"finance":{"error":...}})
// se devuelve con su código y descripción; "too many requests" es
// clasificable con errors.Is(err, ErrRateLimit).
//
//	res, err := c.Screen(ctx, gohoo.ScreenParams{
//	    Query:     gohoo.And(gohoo.Eq("region", "us"), gohoo.Gte("intradaymarketcap", 200e9)),
//	    Size:      100,
//	    SortField: "intradaymarketcap",
//	})
func (c *Client) Screen(ctx context.Context, p ScreenParams) (*ScreenResult, error) {
	if p.Query.IsZero() {
		return nil, errors.New("gohoo: el screener necesita una Query")
	}
	size, err := screenSize(p.Offset, p.Size)
	if err != nil {
		return nil, err
	}
	if p.QuoteType == "" {
		p.QuoteType = "EQUITY"
	}
	if p.SortField == "" {
		p.SortField = "ticker"
	}
	sortType := "DESC"
	if p.SortAsc {
		sortType = "ASC"
	}
	body := map[string]any{
		"offset": p.Offset, "size": size, "sortField": p.SortField, "sortType": sortType,
		"quoteType": p.QuoteType, "query": p.Query, "userId": "", "userIdType": "guid",
	}
	if _, err := json.Marshal(body); err != nil { // query mal armada (p. ej. And con una query vacía)
		return nil, err
	}
	raw, err := c.PostJSON(ctx, "/v1/finance/screener", screenQuery(), body)
	return parseScreen("screener", raw, err)
}

// ScreenPredefined consulta un screener predefinido de Yahoo por su id
// (p. ej. "day_gainers", "day_losers", "most_actives",
// "undervalued_large_caps", "aggressive_small_caps"). Un id inexistente
// devuelve un error clasificable con errors.Is(err, ErrNotFound).
func (c *Client) ScreenPredefined(ctx context.Context, id string, offset, size int) (*ScreenResult, error) {
	if strings.TrimSpace(id) == "" {
		return nil, errors.New("gohoo: falta el id del screener predefinido")
	}
	size, err := screenSize(offset, size)
	if err != nil {
		return nil, err
	}
	q := screenQuery()
	q.Set("scrIds", id)
	q.Set("count", strconv.Itoa(size))
	q.Set("offset", strconv.Itoa(offset))
	raw, err := c.GetJSON(ctx, "/v1/finance/screener/predefined/saved", q)
	return parseScreen("screener "+id, raw, err)
}

func screenSize(offset, size int) (int, error) {
	if offset < 0 {
		return 0, fmt.Errorf("gohoo: offset negativo (%d) en el screener", offset)
	}
	if size < 0 || size > ScreenMaxSize {
		return 0, fmt.Errorf("gohoo: Yahoo limita el screener a %d resultados por página (pedido: %d)", ScreenMaxSize, size)
	}
	if size == 0 {
		size = screenDefaultSize
	}
	return size, nil
}

type screenResponse struct {
	Finance struct {
		Result []struct {
			Start  int              `json:"start"`
			Count  int              `json:"count"`
			Total  int              `json:"total"`
			Quotes []map[string]any `json:"quotes"`
		} `json:"result"`
		Error *struct {
			Code        string `json:"code"`
			Description string `json:"description"`
		} `json:"error"`
	} `json:"finance"`
}

// parseScreen interpreta la respuesta (o el error HTTP, cuyo cuerpo suele
// traer el detalle en finance.error) de los dos endpoints del screener.
func parseScreen(what string, raw []byte, err error) (*ScreenResult, error) {
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) {
			var r screenResponse
			if json.Unmarshal([]byte(se.Body), &r) == nil && r.Finance.Error != nil {
				return nil, fmt.Errorf("%w (%w)", screenAPIError(what, r.Finance.Error.Code, r.Finance.Error.Description), err)
			}
		}
		return nil, err
	}
	var r screenResponse
	if err := json.Unmarshal(raw, &r); err != nil {
		return nil, fmt.Errorf("no se pudo parsear la respuesta del %s: %w", what, err)
	}
	if e := r.Finance.Error; e != nil {
		return nil, screenAPIError(what, e.Code, e.Description)
	}
	if len(r.Finance.Result) == 0 {
		return nil, fmt.Errorf("%s: %w", what, ErrEmptyScreen)
	}
	res := r.Finance.Result[0]
	out := &ScreenResult{Start: res.Start, Count: res.Count, Total: res.Total, Quotes: make([]ScreenQuote, 0, len(res.Quotes))}
	for _, q := range res.Quotes {
		out.Quotes = append(out.Quotes, newScreenQuote(q))
	}
	return out, nil
}

func newScreenQuote(m map[string]any) ScreenQuote {
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
	return ScreenQuote{
		Symbol: str("symbol"), ShortName: str("shortName"), LongName: str("longName"),
		QuoteType: str("quoteType"), Exchange: str("exchange"), FullExchangeName: str("fullExchangeName"),
		Region: str("region"), Currency: str("currency"),
		MarketCap: num("marketCap"), Price: num("regularMarketPrice"),
		Raw: m,
	}
}

func screenAPIError(what, code, description string) error {
	msg := strings.TrimSpace(code + ": " + description)
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "too many"):
		return fmt.Errorf("%s: %s: %w", what, msg, ErrRateLimit)
	case strings.EqualFold(code, "Not Found"):
		return fmt.Errorf("%s: %s: %w", what, msg, ErrNotFound)
	}
	return fmt.Errorf("%s devolvió error: %s", what, msg)
}

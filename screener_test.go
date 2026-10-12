package gohoo

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestQueryJSON(t *testing.T) {
	cases := []struct {
		q    Query
		want string
	}{
		{Eq("region", "us"), `{"operator":"EQ","operands":["region","us"]}`},
		{Btwn("intradaymarketcap", 1e9, 2e9), `{"operator":"BTWN","operands":["intradaymarketcap",1000000000,2000000000]}`},
		{And(Eq("region", "us"), Gte("intradaymarketcap", 2e11)),
			`{"operator":"AND","operands":[{"operator":"EQ","operands":["region","us"]},{"operator":"GTE","operands":["intradaymarketcap",200000000000]}]}`},
		{IsIn("exchange", "NMS", "NYQ"),
			`{"operator":"OR","operands":[{"operator":"EQ","operands":["exchange","NMS"]},{"operator":"EQ","operands":["exchange","NYQ"]}]}`},
		{IsIn("region", "cl"), `{"operator":"EQ","operands":["region","cl"]}`},
		{Or(Gt("a", 1), Lt("b", 2), Lte("c", 3)),
			`{"operator":"OR","operands":[{"operator":"GT","operands":["a",1]},{"operator":"LT","operands":["b",2]},{"operator":"LTE","operands":["c",3]}]}`},
	}
	for _, tc := range cases {
		b, err := json.Marshal(tc.q)
		if err != nil {
			t.Fatal(err)
		}
		if string(b) != tc.want {
			t.Errorf("got  %s\nwant %s", b, tc.want)
		}
		if tc.q.String() != tc.want {
			t.Errorf("String() = %s", tc.q.String())
		}
	}
	if !And().IsZero() || !IsIn("x").IsZero() {
		t.Error("And() e IsIn sin valores deben ser queries vacías")
	}
	if _, err := json.Marshal(Query{}); err == nil {
		t.Error("una query vacía no debe serializarse")
	}
}

func TestScreenRequestAndParse(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/finance/screener" {
			t.Errorf("request inesperada: %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("crumb") != "test-crumb" || q.Get("formatted") != "false" {
			t.Errorf("query = %v", q)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		_, _ = w.Write(readFixture(t, "screener_us_megacap.json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	res, err := c.Screen(context.Background(), ScreenParams{
		Query: And(Eq("region", "us"), Gt("intradaymarketcap", 2e11)), Size: 2, SortField: "intradaymarketcap",
	})
	if err != nil {
		t.Fatal(err)
	}
	if body["size"] != float64(2) || body["offset"] != float64(0) || body["sortType"] != "DESC" ||
		body["quoteType"] != "EQUITY" || body["sortField"] != "intradaymarketcap" || body["userIdType"] != "guid" {
		t.Errorf("cuerpo = %v", body)
	}
	if op := body["query"].(map[string]any)["operator"]; op != "AND" {
		t.Errorf("operator = %v", op)
	}
	if res.Total != 122 || res.Count != 2 || len(res.Quotes) != 2 {
		t.Fatalf("res = %+v", res)
	}
	q := res.Quotes[0]
	if q.Symbol != "NVDA" || q.ShortName != "NVIDIA Corporation" || q.Exchange != "NMS" || q.FullExchangeName != "NasdaqGS" ||
		q.Region != "US" || q.Currency != "USD" || q.QuoteType != "EQUITY" || q.MarketCap != 5466880475136 || q.Price != 226.4 {
		t.Errorf("quote = %+v", q)
	}
	if q.Raw["forwardPE"] != 14.22593 || res.Quotes[1].Symbol != "AAPL" {
		t.Errorf("Raw incompleto: forwardPE=%v, segundo=%s", q.Raw["forwardPE"], res.Quotes[1].Symbol)
	}
}

func TestScreenDefaults(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&body)
		_, _ = w.Write(readFixture(t, "screener_empty.json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	res, err := c.Screen(context.Background(), ScreenParams{Query: Eq("region", "zz"), QuoteType: "ETF", SortAsc: true})
	if err != nil {
		t.Fatal(err)
	}
	if body["size"] != float64(25) || body["sortField"] != "ticker" || body["sortType"] != "ASC" || body["quoteType"] != "ETF" {
		t.Errorf("cuerpo = %v", body)
	}
	if res.Total != 0 || len(res.Quotes) != 0 || res.Quotes == nil {
		t.Errorf("una región sin coincidencias debe dar una página vacía sin error: %+v", res)
	}
}

func TestScreenValidationBeforeNetwork(t *testing.T) {
	c := &Client{crumb: "x"} // sin http.Client: cualquier request entraría en pánico
	for name, p := range map[string]ScreenParams{
		"sin query":       {},
		"size grande":     {Query: Eq("region", "us"), Size: ScreenMaxSize + 1},
		"size negativo":   {Query: Eq("region", "us"), Size: -1},
		"offset negativo": {Query: Eq("region", "us"), Offset: -5},
		"query anidada":   {Query: Query{"AND", []any{Eq("a", 1), Query{}}}},
	} {
		if _, err := c.Screen(context.Background(), p); err == nil {
			t.Errorf("%s: se esperaba error", name)
		}
	}
	if _, err := c.ScreenPredefined(context.Background(), " ", 0, 10); err == nil {
		t.Error("id vacío: se esperaba error")
	}
}

func TestScreenAPIErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(error) bool
		substr string
	}{
		{"operador inválido (400)", 400, string(readFixture(t, "screener_bad_operator.json")),
			func(err error) bool { return err != nil }, "Unsupported operator: FOO"},
		{"rate limit en el cuerpo (200)", 200, `{"finance":{"result":null,"error":{"code":"Too Many Requests","description":"Rate limited"}}}`,
			IsRateLimit, "Too Many Requests"},
		{"sin result", 200, `{"finance":{"result":[],"error":null}}`,
			func(err error) bool { return errors.Is(err, ErrEmptyScreen) }, ""},
		{"JSON inválido", 200, `<html>`, func(err error) bool { return err != nil }, "parsear"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		}))
		c := newTestClient(t, srv)
		_, err := c.Screen(context.Background(), ScreenParams{Query: Eq("region", "us")})
		if !tc.check(err) || !strings.Contains(errString(err), tc.substr) {
			t.Errorf("%s: err = %v", tc.name, err)
		}
		srv.Close()
	}
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func TestScreenPredefined(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if r.Method != http.MethodGet || r.URL.Path != "/v1/finance/screener/predefined/saved" {
			t.Errorf("request inesperada: %s %s", r.Method, r.URL.Path)
		}
		if q.Get("scrIds") == "no_existe" {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write(readFixture(t, "screener_predefined_not_found.json"))
			return
		}
		if q.Get("scrIds") != "day_gainers" || q.Get("count") != "2" || q.Get("offset") != "0" || q.Get("crumb") != "test-crumb" {
			t.Errorf("query = %v", q)
		}
		_, _ = w.Write(readFixture(t, "screener_predefined_day_gainers.json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	res, err := c.ScreenPredefined(context.Background(), "day_gainers", 0, 2)
	if err != nil {
		t.Fatal(err)
	}
	if res.Total != 45 || len(res.Quotes) != 2 || res.Quotes[0].Symbol != "FIGS" || res.Quotes[1].Symbol != "CCC" {
		t.Errorf("res = %+v", res)
	}

	_, err = c.ScreenPredefined(context.Background(), "no_existe", 0, 0)
	var se *StatusError
	if !errors.Is(err, ErrNotFound) || !errors.As(err, &se) || !strings.Contains(err.Error(), "No screener records found") {
		t.Errorf("err = %v, se esperaba ErrNotFound con el detalle de Yahoo", err)
	}
}

func TestQueryStringZero(t *testing.T) {
	if got := (Query{}).String(); got != "<query vacía>" {
		t.Errorf("Query{}.String() = %q", got)
	}
}

func TestScreenPredefinedValidationAndPlainHTTPError(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("no es json"))
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	if _, err := c.ScreenPredefined(context.Background(), "day_gainers", -1, 0); err == nil || calls != 0 {
		t.Errorf("offset negativo: err=%v requests=%d, want error sin requests", err, calls)
	}
	if _, err := c.ScreenPredefined(context.Background(), "day_gainers", 0, 0); !errors.Is(err, ErrNotFound) {
		t.Errorf("404 sin JSON: err = %v, want ErrNotFound", err)
	}
}

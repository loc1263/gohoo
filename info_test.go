package gohoo

import (
	"context"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func fixtureQuoteSummary(t *testing.T, name string) map[string]any {
	t.Helper()
	var r struct {
		QuoteSummary struct {
			Result []map[string]any `json:"result"`
		} `json:"quoteSummary"`
	}
	if err := json.Unmarshal(readFixture(t, name), &r); err != nil || len(r.QuoteSummary.Result) == 0 {
		t.Fatal(err)
	}
	return r.QuoteSummary.Result[0]
}

func TestFlattenInfoPriorityWithRealResponse(t *testing.T) {
	qs := fixtureQuoteSummary(t, "quotesummary_copec_sn.json")
	info := FlattenInfo(qs)
	// summaryDetail gana: el forwardPE de defaultKeyStatistics viene corrupto (11836).
	if info["forwardPE"] != 11.162578 {
		t.Errorf("forwardPE = %v", info["forwardPE"])
	}
	if info["dividendYield"] != 0.0279 { // sin opción, tal como lo manda Yahoo
		t.Errorf("dividendYield = %v", info["dividendYield"])
	}
	if info["currency"] != "CLP" || info["symbol"] != "COPEC.SN" || info["sector"] == nil || info["quoteType"] != "EQUITY" {
		t.Errorf("info incompleta: currency=%v symbol=%v sector=%v quoteType=%v", info["currency"], info["symbol"], info["sector"], info["quoteType"])
	}
	for k, v := range info {
		if m, ok := v.(map[string]any); ok {
			t.Errorf("%s quedó como objeto: %v", k, m)
		}
	}
	// No modifica qs.
	if _, ok := qs["summaryDetail"].(map[string]any)["forwardPE"].(map[string]any); !ok {
		t.Error("FlattenInfo modificó qs")
	}

	pct := FlattenInfo(qs, InfoPercentYield())
	if y := pct["dividendYield"].(float64); math.Abs(y-2.79) > 1e-9 {
		t.Errorf("dividendYield en %% = %v", y)
	}
}

func TestFlattenInfoETFYieldAndModules(t *testing.T) {
	qs := fixtureQuoteSummary(t, "quotesummary_spy.json")
	// dividendYield viene como {} en ETFs: se omite y con InfoPercentYield se toma "yield".
	if _, ok := FlattenInfo(qs)["dividendYield"]; ok {
		t.Error("un objeto vacío no debe quedar como dividendYield")
	}
	if y := FlattenInfo(qs, InfoPercentYield())["dividendYield"].(float64); math.Abs(y-0.99) > 1e-9 {
		t.Errorf("ETF dividendYield = %v", y)
	}
	// fundProfile no está en los módulos por defecto; con InfoWithModules sí, y en el orden dado.
	if _, ok := FlattenInfo(qs)["categoryName"]; ok {
		t.Error("los módulos fuera de la lista se ignoran")
	}
	if FlattenInfo(qs, InfoWithModules("fundProfile"))["categoryName"] == nil {
		t.Error("InfoWithModules debe aplanar fundProfile")
	}
	if _, ok := FlattenInfo(map[string]any{}, InfoPercentYield())["dividendYield"]; ok {
		t.Error("sin datos no hay dividendYield")
	}
}

func TestInfoModulesIsACopy(t *testing.T) {
	m := InfoModules()
	m[0] = "x"
	if InfoModules()[0] != "summaryDetail" {
		t.Error("InfoModules debe devolver una copia")
	}
}

func TestInfoRequestsModulesAndTrailingPEG(t *testing.T) {
	var gotModules string
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/v10/finance/quoteSummary/COPEC.SN"):
			gotModules = r.URL.Query().Get("modules")
			_, _ = w.Write(readFixture(t, "quotesummary_copec_sn.json"))
		case strings.HasPrefix(r.URL.Path, "/ws/fundamentals-timeseries/"):
			if r.URL.Query().Get("type") != "trailingPegRatio" {
				t.Errorf("type = %s", r.URL.Query().Get("type"))
			}
			if fail {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			_, _ = w.Write(readFixture(t, "timeseries_aapl.json"))
		}
	}))
	defer srv.Close()
	c := newTestClient(t, srv)

	info, err := c.Info(context.Background(), "COPEC.SN", InfoPercentYield(), InfoWithTrailingPEG())
	if err != nil {
		t.Fatal(err)
	}
	if gotModules != strings.Join(InfoModules(), ",") {
		t.Errorf("modules = %s", gotModules)
	}
	if info["trailingPegRatio"] != 2.706 || math.Abs(info["dividendYield"].(float64)-2.79) > 1e-9 {
		t.Errorf("peg=%v divY=%v", info["trailingPegRatio"], info["dividendYield"])
	}

	fail = true
	info, err = c.Info(context.Background(), "COPEC.SN", InfoWithTrailingPEG())
	if err != nil || info["trailingPegRatio"] != nil || info["forwardPE"] == nil {
		t.Errorf("si falla el PEG, Info sigue sin la clave: %v %v", err, info["trailingPegRatio"])
	}
	if _, err := c.Info(context.Background(), "COPEC.SN", InfoWithModules("price")); err != nil || gotModules != "price" {
		t.Errorf("InfoWithModules: %v %s", err, gotModules)
	}
}

func TestTimeseriesPointsRealResponse(t *testing.T) {
	var data map[string]any
	if err := json.Unmarshal(readFixture(t, "timeseries_aapl.json"), &data); err != nil {
		t.Fatal(err)
	}
	rev := TimeseriesPoints(data, "quarterlyTotalRevenue")
	if len(rev) != 5 || rev[0].Date != "2026-06-30" || rev[0].Value != 109417000000 || rev[0].PeriodType != "3M" || rev[0].Currency != "USD" {
		t.Fatalf("ingresos = %+v", rev)
	}
	for i := 1; i < len(rev); i++ {
		if rev[i].Date >= rev[i-1].Date {
			t.Errorf("no está ordenado de más nuevo a más viejo: %v", rev)
		}
	}
	if capex := TimeseriesPoints(data, "quarterlyCapitalExpenditure"); len(capex) != 5 || capex[0].Value >= 0 {
		t.Errorf("capex = %+v", capex)
	}
	if peg := TimeseriesPoints(data, "trailingPegRatio"); len(peg) == 0 || peg[0].Value != 2.706 || peg[0].Currency != "" {
		t.Errorf("peg = %+v", peg)
	}
	if TimeseriesPoints(data, "noExiste") != nil || TimeseriesPoints(nil, "x") != nil {
		t.Error("tipo ausente = nil")
	}
}

func TestTimeseriesPointsSkipsMissing(t *testing.T) {
	data := map[string]any{"timeseries": map[string]any{"result": []any{
		map[string]any{"x": []any{
			nil,
			map[string]any{"asOfDate": "2025-01-01"},
			map[string]any{"reportedValue": map[string]any{"raw": 1.0}},
			map[string]any{"asOfDate": "2024-01-01", "reportedValue": map[string]any{"raw": 2.0}},
		}},
	}}}
	if pts := TimeseriesPoints(data, "x"); len(pts) != 1 || pts[0].Value != 2 {
		t.Errorf("%+v", pts)
	}
}

func TestTrailingPEGWithoutData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, `{"timeseries":{"result":[{"meta":{"type":["trailingPegRatio"]}}],"error":null}}`)
	}))
	defer srv.Close()
	v, ok, err := newTestClient(t, srv).TrailingPEG(context.Background(), "X")
	if err != nil || ok || v != 0 {
		t.Errorf("%v %v %v", v, ok, err)
	}
}

func TestAutoAdjustAndAtMidnight(t *testing.T) {
	ny, _ := time.LoadLocation("America/New_York")
	in := []Bar{
		{Time: time.Date(2026, 1, 2, 9, 30, 0, 0, ny), Open: 2, High: 3, Low: 1, Close: 2, AdjClose: 1, Volume: 10},
		{Time: time.Date(2026, 1, 5, 9, 30, 0, 0, ny), Open: 4, High: 4, Low: 4, Close: 0, AdjClose: 5, Volume: 20},
		{Time: time.Date(2026, 1, 6, 9, 30, 0, 0, ny), Open: 4, High: 4, Low: 4, Close: 4, AdjClose: math.NaN(), Volume: 30},
	}
	out := AutoAdjust(in)
	if b := out[0]; b.Open != 1 || b.High != 1.5 || b.Low != 0.5 || b.Close != 1 || b.AdjClose != 1 || b.Volume != 10 || !b.Time.Equal(in[0].Time) {
		t.Errorf("factor 0,5: %+v", b)
	}
	if b := out[1]; b.Open != 4 || b.Close != 5 { // Close 0: factor 1
		t.Errorf("Close 0: %+v", b)
	}
	if b := out[2]; b.Open != 4 || !math.IsNaN(b.Close) { // AdjClose NaN: factor 1
		t.Errorf("AdjClose NaN: %+v", b)
	}
	if in[0].Close != 2 {
		t.Error("AutoAdjust modificó la entrada")
	}
	r := &ChartResult{Bars: in}
	if got := r.AutoAdjusted(); len(got) != 3 || got[0].Close != 1 {
		t.Errorf("AutoAdjusted = %+v", got)
	}

	d := in[0].Date()
	if d.Hour() != 0 || d.Day() != 2 || d.Location() != ny {
		t.Errorf("Date = %v", d)
	}
	mid := AtMidnight(in)
	if !mid[1].Time.Equal(time.Date(2026, 1, 5, 0, 0, 0, 0, ny)) || mid[1].Open != 4 || in[1].Time.Hour() != 9 {
		t.Errorf("AtMidnight = %+v (entrada %v)", mid[1], in[1].Time)
	}
}

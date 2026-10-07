// Package yahoo es un cliente Go para los endpoints no oficiales de Yahoo
// Finance (quoteSummary, fundamentals-timeseries y chart): maneja la
// autenticación por cookie+crumb y expone los datos crudos como
// map[string]interface{}, más un helper de series OHLCV ya parseadas
// (ChartSeries) para precios históricos de acciones, ETFs y criptomonedas.
package yahoo

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// Client encapsula la sesión HTTP (cookies + crumb) usada para hablar con los
// endpoints no oficiales de Yahoo Finance.
type Client struct {
	http  *http.Client
	crumb string
}

// NewClient crea un Client ya autenticado (cookies de sesión + crumb).
func NewClient() (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	c := &Client{http: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
	if err := c.bootstrap(); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Client) doGet(rawURL string) ([]byte, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "*/*")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return body, fmt.Errorf("HTTP %d en %s: %s", resp.StatusCode, rawURL, string(body[:min(len(body), 300)]))
	}
	return body, nil
}

// bootstrap obtiene cookies de consentimiento y el "crumb" requerido por las
// APIs de query1/query2.finance.yahoo.com desde 2024.
func (c *Client) bootstrap() error {
	// Primero una visita a fc.yahoo.com para fijar la cookie de sesión (A3, etc)
	// antes de pedir el crumb. La respuesta puede ser 404 pero de todas formas
	// fija la cookie necesaria.
	_, _ = c.doGet("https://fc.yahoo.com")

	body, err := c.doGet("https://query2.finance.yahoo.com/v1/test/getcrumb")
	if err != nil {
		return fmt.Errorf("no se pudo obtener crumb de Yahoo Finance: %w", err)
	}
	crumb := strings.TrimSpace(string(body))
	if crumb == "" || strings.Contains(crumb, "<html") {
		return fmt.Errorf("respuesta de crumb inválida: %q", crumb)
	}
	c.crumb = crumb
	return nil
}

// QuoteSummary llama a v10/finance/quoteSummary con los módulos solicitados y
// devuelve el primer resultado como un árbol genérico de interface{}.
func (c *Client) QuoteSummary(ticker string, modules []string) (map[string]interface{}, error) {
	u := fmt.Sprintf("https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=%s&crumb=%s",
		url.PathEscape(ticker), url.QueryEscape(strings.Join(modules, ",")), url.QueryEscape(c.crumb))
	body, err := c.doGet(u)
	if err != nil {
		return nil, err
	}
	var parsed struct {
		QuoteSummary struct {
			Result []map[string]interface{} `json:"result"`
			Error  interface{}              `json:"error"`
		} `json:"quoteSummary"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("no se pudo parsear quoteSummary de %s: %w", ticker, err)
	}
	if parsed.QuoteSummary.Error != nil {
		return nil, fmt.Errorf("quoteSummary de %s devolvió error: %v", ticker, parsed.QuoteSummary.Error)
	}
	if len(parsed.QuoteSummary.Result) == 0 {
		return map[string]interface{}{}, nil
	}
	merged := map[string]interface{}{}
	for _, r := range parsed.QuoteSummary.Result {
		for k, v := range r {
			merged[k] = v
		}
	}
	return merged, nil
}

// FundamentalsTimeseries llama al endpoint de fundamentals-timeseries para un
// conjunto de "type" (p.ej. annualTotalRevenue, quarterlyNetIncome, ...).
func (c *Client) FundamentalsTimeseries(ticker string, types []string) (map[string]interface{}, error) {
	period2 := time.Now().Unix()
	period1 := time.Now().AddDate(-6, 0, 0).Unix()
	u := fmt.Sprintf("https://query2.finance.yahoo.com/ws/fundamentals-timeseries/v1/finance/timeseries/%s?symbol=%s&type=%s&period1=%d&period2=%d",
		url.PathEscape(ticker), url.QueryEscape(ticker), url.QueryEscape(strings.Join(types, ",")), period1, period2)
	body, err := c.doGet(u)
	if err != nil {
		return nil, err
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("no se pudo parsear fundamentals-timeseries de %s: %w", ticker, err)
	}
	return parsed, nil
}

// Chart llama al endpoint v8/finance/chart con el rango e intervalos pedidos,
// incluyendo dividendos y splits.
func (c *Client) Chart(ticker, rang, interval string) (map[string]interface{}, error) {
	u := fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?range=%s&interval=%s&events=div,splits",
		url.PathEscape(ticker), url.QueryEscape(rang), url.QueryEscape(interval))
	body, err := c.doGet(u)
	if err != nil {
		return nil, err
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("no se pudo parsear chart de %s: %w", ticker, err)
	}
	return parsed, nil
}

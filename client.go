// Package yahoo es un cliente Go para los endpoints no oficiales de Yahoo
// Finance (quoteSummary, fundamentals-timeseries y chart): maneja la
// autenticación por cookie+crumb y expone los datos crudos como
// map[string]interface{}, más un helper de series OHLCV ya parseadas
// (ChartSeries) para precios históricos de acciones, ETFs y criptomonedas.
package yahoo

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// maxAttempts es el número total de intentos (1 inicial + reintentos) que
// hacen QuoteSummary/FundamentalsTimeseries/Chart ante fallas transitorias.
const maxAttempts = 4

// retryBaseDelay es la base del backoff exponencial entre reintentos
// (200ms, 400ms, 800ms, ...).
const retryBaseDelay = 200 * time.Millisecond

// httpStatusError es el error que devuelve doGet ante una respuesta HTTP
// distinta de 200, con el código de estado accesible vía errors.As para que
// doGetAuthed decida si reintentar.
type httpStatusError struct {
	StatusCode int
	URL        string
	Body       string
}

func (e *httpStatusError) Error() string {
	return fmt.Sprintf("HTTP %d en %s: %s", e.StatusCode, e.URL, e.Body)
}

// isRetryableStatus indica qué códigos HTTP vale la pena reintentar: 429
// (rate limit) y 5xx (errores transitorios del servidor). 401 se maneja
// aparte porque requiere renovar el crumb, no solo esperar.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

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
		return body, &httpStatusError{
			StatusCode: resp.StatusCode,
			URL:        rawURL,
			Body:       string(body[:min(len(body), 300)]),
		}
	}
	return body, nil
}

// doGetAuthed llama a buildURL() y hace el GET resultante, reintentando ante
// fallas transitorias: errores de red (timeouts, conexión rechazada, etc.)
// y HTTP 429/5xx se reintentan con backoff exponencial; un HTTP 401 dispara
// un `bootstrap` (cookies+crumb nuevos) y se reintenta con la URL
// reconstruida a partir del crumb ya renovado. buildURL recibe una función
// en lugar de una URL fija porque la URL de la mayoría de los endpoints
// incluye el crumb vigente, que puede cambiar entre intentos.
func (c *Client) doGetAuthed(buildURL func() string) ([]byte, error) {
	var lastErr error
	for attempt := range maxAttempts {
		if attempt > 0 {
			time.Sleep(retryBaseDelay * time.Duration(1<<(attempt-1)))
		}
		body, err := c.doGet(buildURL())
		if err == nil {
			return body, nil
		}
		lastErr = err

		var statusErr *httpStatusError
		if errors.As(err, &statusErr) {
			if statusErr.StatusCode == http.StatusUnauthorized {
				if bootErr := c.bootstrap(); bootErr != nil {
					return nil, fmt.Errorf("re-autenticación tras HTTP 401 falló: %w", bootErr)
				}
				continue
			}
			if !isRetryableStatus(statusErr.StatusCode) {
				return nil, err
			}
			continue
		}
		// Error de red (timeout, conexión reiniciada, DNS, etc.): reintentable.
	}
	return nil, fmt.Errorf("agotados %d intentos: %w", maxAttempts, lastErr)
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
	body, err := c.doGetAuthed(func() string {
		return fmt.Sprintf("https://query2.finance.yahoo.com/v10/finance/quoteSummary/%s?modules=%s&crumb=%s",
			url.PathEscape(ticker), url.QueryEscape(strings.Join(modules, ",")), url.QueryEscape(c.crumb))
	})
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
	body, err := c.doGetAuthed(func() string {
		return fmt.Sprintf("https://query2.finance.yahoo.com/ws/fundamentals-timeseries/v1/finance/timeseries/%s?symbol=%s&type=%s&period1=%d&period2=%d",
			url.PathEscape(ticker), url.QueryEscape(ticker), url.QueryEscape(strings.Join(types, ",")), period1, period2)
	})
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
	body, err := c.doGetAuthed(func() string {
		return fmt.Sprintf("https://query2.finance.yahoo.com/v8/finance/chart/%s?range=%s&interval=%s&events=div,splits",
			url.PathEscape(ticker), url.QueryEscape(rang), url.QueryEscape(interval))
	})
	if err != nil {
		return nil, err
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("no se pudo parsear chart de %s: %w", ticker, err)
	}
	return parsed, nil
}

// Package gohoo es un cliente Go para los endpoints no oficiales de Yahoo
// Finance (quoteSummary, fundamentals-timeseries, chart y screener): maneja la
// autenticación por cookie+crumb y expone los datos crudos como
// map[string]interface{}, más un helper de series OHLCV ya parseadas
// (ChartSeries) para precios históricos de acciones, ETFs y criptomonedas.
package gohoo

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

const userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0 Safari/537.36"

// maxAttempts es el número total de intentos (1 inicial + reintentos) que
// hacen QuoteSummary/FundamentalsTimeseries/Chart ante fallas transitorias
// con la política por defecto.
const maxAttempts = 4

// retryBaseDelay es la base del backoff exponencial entre reintentos
// (200ms, 400ms, 800ms, ...) con la política por defecto.
const retryBaseDelay = 200 * time.Millisecond

// isRetryableStatus indica qué códigos HTTP vale la pena reintentar: 429
// (rate limit) y 5xx (errores transitorios del servidor). 401 se maneja
// aparte porque requiere renovar el crumb, no solo esperar.
func isRetryableStatus(code int) bool {
	return code == http.StatusTooManyRequests || code >= 500
}

// Client encapsula la sesión HTTP (cookies + crumb) usada para hablar con los
// endpoints no oficiales de Yahoo Finance.
type Client struct {
	http *http.Client
	cfg  *config // nil = configuración por defecto; puntero para que Client siga siendo comparable

	mu    sync.RWMutex // protege crumb (se renueva ante un HTTP 401)
	crumb string

	bootMu sync.Mutex // serializa el bootstrap perezoso (WithLazyBootstrap)
	rnd    jitterRand
}

func (c *Client) getCrumb() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.crumb
}

// NewClient crea un Client ya autenticado (cookies de sesión + crumb).
// Equivale a NewClientWithOptions() sin opciones.
func NewClient() (*Client, error) {
	return NewClientWithOptions()
}

func (c *Client) doGet(ctx context.Context, rawURL string) ([]byte, error) {
	return c.do(ctx, http.MethodGet, rawURL, nil)
}

// do hace una request con el User-Agent del cliente. body, si no es nil, se
// manda como JSON; se relee en cada llamada, así que do se puede reintentar.
func (c *Client) do(ctx context.Context, method, rawURL string, body []byte) ([]byte, error) {
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, rd)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent())
	req.Header.Set("Accept", "*/*")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != 200 {
		return respBody, &httpStatusError{
			StatusCode: resp.StatusCode,
			URL:        rawURL,
			Body:       string(respBody[:min(len(respBody), 300)]),
			RetryAfter: parseRetryAfter(resp.Header.Get("Retry-After"), time.Now()),
		}
	}
	return respBody, nil
}

// doGetAuthed llama a buildURL() y hace el GET resultante, reintentando ante
// fallas transitorias según la RetryPolicy del cliente: errores de red
// (timeouts, conexión rechazada, etc.) y HTTP 429/5xx se reintentan con
// backoff exponencial; un HTTP 401 dispara un `bootstrap` (cookies+crumb
// nuevos) y se reintenta con la URL reconstruida a partir del crumb ya
// renovado. buildURL recibe una función en lugar de una URL fija porque la
// URL de la mayoría de los endpoints incluye el crumb vigente, que puede
// cambiar entre intentos. Si ctx termina, se devuelve de inmediato sin
// consumir los intentos restantes.
func (c *Client) doGetAuthed(ctx context.Context, buildURL func() string) ([]byte, error) {
	return c.doAuthed(ctx, http.MethodGet, buildURL, nil)
}

// doAuthed es doGetAuthed para cualquier método; body (JSON) puede ser nil.
// Es la única implementación de los reintentos y la renovación del crumb.
func (c *Client) doAuthed(ctx context.Context, method string, buildURL func() string, body []byte) ([]byte, error) {
	policy := c.retryPolicy()
	var lastErr error
	var lastStatus *httpStatusError
	for attempt := range policy.MaxAttempts {
		if attempt > 0 {
			d := policy.delay(attempt, lastStatus, c.rnd.Float64)
			if policy.OnRetry != nil {
				policy.OnRetry(attempt, lastErr, d)
			}
			if err := c.sleep(ctx, d); err != nil {
				return nil, fmt.Errorf("cancelado tras %d intentos: %w (último error: %w)", attempt, err, lastErr)
			}
		}
		respBody, err := c.do(ctx, method, buildURL(), body)
		if err == nil {
			return respBody, nil
		}
		if ctx.Err() != nil {
			return nil, err
		}
		lastErr = err
		lastStatus = nil

		var statusErr *httpStatusError
		if errors.As(err, &statusErr) {
			lastStatus = statusErr
			if statusErr.StatusCode == http.StatusUnauthorized {
				if bootErr := c.bootstrap(ctx); bootErr != nil {
					return nil, fmt.Errorf("re-autenticación tras HTTP 401 falló: %w", bootErr)
				}
				continue
			}
			if statusErr.StatusCode == http.StatusTooManyRequests && !policy.RetryRateLimit {
				return nil, err
			}
			if !isRetryableStatus(statusErr.StatusCode) {
				return nil, err
			}
			continue
		}
		// Error de red (timeout, conexión reiniciada, DNS, etc.): reintentable.
	}
	return nil, fmt.Errorf("agotados %d intentos: %w", policy.MaxAttempts, lastErr)
}

// bootstrap obtiene cookies de consentimiento y el "crumb" requerido por las
// APIs de query1/query2.finance.yahoo.com desde 2024.
func (c *Client) bootstrap(ctx context.Context) error {
	// Primero una visita a fc.yahoo.com para fijar la cookie de sesión (A3, etc)
	// antes de pedir el crumb. La respuesta puede ser 404 pero de todas formas
	// fija la cookie necesaria.
	_, _ = c.doGet(ctx, c.consentURL())

	body, err := c.doGet(ctx, c.baseURL()+"/v1/test/getcrumb")
	if err != nil {
		return fmt.Errorf("no se pudo obtener crumb de Yahoo Finance: %w", err)
	}
	crumb := strings.TrimSpace(string(body))
	if crumb == "" || strings.Contains(crumb, "<html") {
		return fmt.Errorf("respuesta de crumb inválida: %q", crumb)
	}
	c.mu.Lock()
	c.crumb = crumb
	c.mu.Unlock()
	return nil
}

// ensureCrumb hace el bootstrap si todavía no hay crumb (solo ocurre con
// WithLazyBootstrap). Varias goroutines que llegan a la vez esperan al
// mismo bootstrap en lugar de repetirlo.
func (c *Client) ensureCrumb(ctx context.Context) error {
	if c.getCrumb() != "" {
		return nil
	}
	c.bootMu.Lock()
	defer c.bootMu.Unlock()
	if c.getCrumb() != "" {
		return nil
	}
	return c.bootstrap(ctx)
}

// QuoteSummary llama a v10/finance/quoteSummary con los módulos solicitados y
// devuelve el primer resultado como un árbol genérico de interface{}.
func (c *Client) QuoteSummary(ticker string, modules []string) (map[string]interface{}, error) {
	return c.QuoteSummaryContext(context.Background(), ticker, modules)
}

// QuoteSummaryContext es QuoteSummary con cancelación y plazos por ctx.
func (c *Client) QuoteSummaryContext(ctx context.Context, ticker string, modules []string) (map[string]interface{}, error) {
	if err := c.ensureCrumb(ctx); err != nil {
		return nil, fmt.Errorf("quoteSummary de %s: %w", ticker, err)
	}
	body, err := c.doGetAuthed(ctx, func() string {
		return fmt.Sprintf("%s/v10/finance/quoteSummary/%s?modules=%s&crumb=%s",
			c.baseURL(), url.PathEscape(ticker), url.QueryEscape(strings.Join(modules, ",")), url.QueryEscape(c.getCrumb()))
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
	return c.FundamentalsTimeseriesContext(context.Background(), ticker, types)
}

// FundamentalsTimeseriesContext es FundamentalsTimeseries con cancelación y
// plazos por ctx.
func (c *Client) FundamentalsTimeseriesContext(ctx context.Context, ticker string, types []string) (map[string]interface{}, error) {
	period2 := time.Now().Unix()
	period1 := time.Now().AddDate(-6, 0, 0).Unix()
	body, err := c.doGetAuthed(ctx, func() string {
		return fmt.Sprintf("%s/ws/fundamentals-timeseries/v1/finance/timeseries/%s?symbol=%s&type=%s&period1=%d&period2=%d",
			c.baseURL(), url.PathEscape(ticker), url.QueryEscape(ticker), url.QueryEscape(strings.Join(types, ",")), period1, period2)
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
	return c.ChartContext(context.Background(), ticker, rang, interval)
}

// ChartContext es Chart con cancelación y plazos por ctx.
func (c *Client) ChartContext(ctx context.Context, ticker, rang, interval string) (map[string]interface{}, error) {
	body, err := c.doGetAuthed(ctx, func() string {
		return fmt.Sprintf("%s/v8/finance/chart/%s?range=%s&interval=%s&events=div,splits",
			c.baseURL(), url.PathEscape(ticker), url.QueryEscape(rang), url.QueryEscape(interval))
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

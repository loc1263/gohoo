package gohoo

import (
	"context"
	"fmt"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strings"
	"time"
)

// defaultBaseURL es el host de quoteSummary, fundamentals-timeseries, chart
// y getcrumb cuando no se usa WithBaseURL.
const defaultBaseURL = "https://query2.finance.yahoo.com"

// defaultConsentURL fija la cookie de sesión antes de pedir el crumb.
const defaultConsentURL = "https://fc.yahoo.com"

// Option configura un Client creado con NewClientWithOptions.
type Option func(*config)

// config es la configuración interna del Client. Su valor cero equivale a
// la configuración de NewClient(): cada accesor de Client cae en el default
// cuando el campo está vacío.
type config struct {
	httpClient *http.Client
	baseURL    string
	consentURL string
	userAgent  string
	retry      *RetryPolicy
	lazy       bool

	limiter     Limiter
	requestHook func(RequestEvent)

	// sleep espera d o hasta que ctx termine; se reemplaza en tests para
	// verificar los retardos sin esperar.
	sleep func(ctx context.Context, d time.Duration) error
}

// defaultConfig es la configuración vacía (todo por defecto); no se modifica.
var defaultConfig config

// WithHTTPClient usa h para todas las requests. Si h no trae Jar se usa una
// copia de h con un cookie jar nuevo (el crumb depende de las cookies); h no
// se modifica.
func WithHTTPClient(h *http.Client) Option {
	return func(c *config) { c.httpClient = h }
}

// WithBaseURL cambia el host de chart, quoteSummary,
// fundamentals-timeseries y getcrumb (por defecto
// https://query2.finance.yahoo.com), p. ej. a
// https://query1.finance.yahoo.com o a un httptest.Server.
func WithBaseURL(u string) Option {
	return func(c *config) { c.baseURL = u }
}

// WithUserAgent cambia el header User-Agent de todas las requests.
func WithUserAgent(ua string) Option {
	return func(c *config) { c.userAgent = ua }
}

// WithRetryPolicy reemplaza la política de reintentos por defecto (ver
// RetryPolicy y DefaultRetryPolicy).
func WithRetryPolicy(p RetryPolicy) Option {
	return func(c *config) { c.retry = &p }
}

// WithLazyBootstrap hace que NewClientWithOptions no haga ninguna request:
// el bootstrap de cookie+crumb corre la primera vez que un endpoint lo
// necesita (QuoteSummaryContext, Info, Quotes, Screen, ScreenPredefined,
// GetJSON y PostJSON) o tras un HTTP 401. Chart, ChartSeries y ChartRange
// nunca lo exigen.
func WithLazyBootstrap() Option {
	return func(c *config) { c.lazy = true }
}

// WithLimiter hace que el cliente llame a l.Wait antes de CADA request HTTP
// (incluidos los reintentos y el bootstrap de cookie+crumb), para espaciar
// o limitar el tráfico hacia Yahoo. Si Wait devuelve error, la llamada
// termina con ese error, sin reintentar. Un mismo Limiter puede
// compartirse entre varios clientes.
func WithLimiter(l Limiter) Option {
	return func(c *config) { c.limiter = l }
}

// WithRequestHook hace que el cliente llame a fn después de CADA request
// HTTP (incluidos los reintentos y el bootstrap), con su resultado. fn se
// llama desde la goroutine de la request y debe ser seguro para uso
// concurrente y rápido.
func WithRequestHook(fn func(RequestEvent)) Option {
	return func(c *config) { c.requestHook = fn }
}

// NewClientWithOptions crea un Client configurable. Sin opciones se comporta
// exactamente igual que NewClient(): bootstrap inmediato de cookie+crumb,
// host query2, 4 intentos con backoff de 200 ms.
func NewClientWithOptions(opts ...Option) (*Client, error) {
	var cfg config
	for _, opt := range opts {
		if opt != nil {
			opt(&cfg)
		}
	}
	if cfg.baseURL != "" {
		cfg.baseURL = strings.TrimRight(cfg.baseURL, "/")
		u, err := url.Parse(cfg.baseURL)
		if err != nil || u.Scheme == "" || u.Host == "" {
			return nil, fmt.Errorf("gohoo: base URL inválida %q", cfg.baseURL)
		}
	}

	var hc *http.Client
	if cfg.httpClient != nil {
		cp := *cfg.httpClient
		hc = &cp
	} else {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	if hc.Jar == nil {
		jar, err := cookiejar.New(nil)
		if err != nil {
			return nil, err
		}
		hc.Jar = jar
	}

	c := &Client{http: hc, cfg: &cfg}
	if !cfg.lazy {
		if err := c.bootstrap(context.Background()); err != nil {
			return nil, fmt.Errorf("gohoo: %w", err)
		}
	}
	return c, nil
}

// conf devuelve la configuración del cliente; un Client sin ella (nil) usa
// los valores por defecto.
func (c *Client) conf() *config {
	if c.cfg == nil {
		return &defaultConfig
	}
	return c.cfg
}

func (c *Client) baseURL() string {
	if c.conf().baseURL != "" {
		return c.conf().baseURL
	}
	return defaultBaseURL
}

func (c *Client) consentURL() string {
	if c.conf().consentURL != "" {
		return c.conf().consentURL
	}
	return defaultConsentURL
}

func (c *Client) userAgent() string {
	if c.conf().userAgent != "" {
		return c.conf().userAgent
	}
	return userAgent
}

func (c *Client) sleep(ctx context.Context, d time.Duration) error {
	if c.conf().sleep != nil {
		return c.conf().sleep(ctx, d)
	}
	return sleepContext(ctx, d)
}

// sleepContext espera d, o devuelve ctx.Err() si el contexto termina antes.
func sleepContext(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return ctx.Err()
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

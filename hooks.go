package gohoo

import (
	"context"
	"net/url"
	"strings"
	"time"
)

// Limiter controla el ritmo de las requests de un Client (ver WithLimiter).
// Wait bloquea hasta que se pueda hacer la siguiente request, o devuelve
// error si ctx termina antes. golang.org/x/time/rate.Limiter lo cumple.
type Limiter interface {
	Wait(ctx context.Context) error
}

// LimiterFunc adapta una función a Limiter.
type LimiterFunc func(ctx context.Context) error

// Wait llama a f(ctx).
func (f LimiterFunc) Wait(ctx context.Context) error { return f(ctx) }

// limiterError envuelve el error de Limiter.Wait para no reintentarlo.
type limiterError struct{ err error }

func (e *limiterError) Error() string { return "gohoo: limiter: " + e.err.Error() }
func (e *limiterError) Unwrap() error { return e.err }

// RequestEvent describe una request HTTP ya terminada (ver WithRequestHook).
type RequestEvent struct {
	// Method es el método HTTP ("GET", "POST").
	Method string
	// Endpoint clasifica la request: "chart", "quoteSummary", "fundamentals",
	// "quote", "screener", "getcrumb", "consent" u "other".
	Endpoint string
	// Symbol es el ticker de la request si la URL lo trae (en "quote", la
	// lista de símbolos separada por comas; en "screener" predefinido, su id).
	Symbol string
	// URL es la URL pedida, sin el crumb.
	URL string
	// Attempt es el número de intento dentro de la llamada (1 = el primero).
	Attempt int
	// StatusCode es el status HTTP, o 0 si no hubo respuesta (error de red).
	StatusCode int
	// Duration es lo que tardó la request (sin la espera del Limiter).
	Duration time.Duration
	// Err es el error de la request (nil si respondió 200); con status
	// distinto de 200 es un *StatusError.
	Err error
}

func newRequestEvent(method, rawURL, consentURL string, attempt, status int, d time.Duration, err error) RequestEvent {
	ev := RequestEvent{Method: method, URL: rawURL, Attempt: attempt, StatusCode: status, Duration: d, Err: err, Endpoint: "other"}
	u, perr := url.Parse(rawURL)
	if perr != nil {
		return ev
	}
	q := u.Query()
	if q.Has("crumb") {
		q.Del("crumb")
		u.RawQuery = q.Encode()
		ev.URL = u.String()
	}
	if c, cerr := url.Parse(consentURL); cerr == nil && c.Host == u.Host && strings.TrimSuffix(c.Path, "/") == strings.TrimSuffix(u.Path, "/") {
		ev.Endpoint = "consent"
		return ev
	}
	last := func(prefix string) string {
		s, _ := url.PathUnescape(strings.TrimPrefix(u.Path, prefix))
		return s
	}
	switch p := u.Path; {
	case strings.HasPrefix(p, "/v8/finance/chart/"):
		ev.Endpoint, ev.Symbol = "chart", last("/v8/finance/chart/")
	case strings.HasPrefix(p, "/v10/finance/quoteSummary/"):
		ev.Endpoint, ev.Symbol = "quoteSummary", last("/v10/finance/quoteSummary/")
	case strings.HasPrefix(p, "/ws/fundamentals-timeseries/v1/finance/timeseries/"):
		ev.Endpoint, ev.Symbol = "fundamentals", last("/ws/fundamentals-timeseries/v1/finance/timeseries/")
	case p == "/v7/finance/quote":
		ev.Endpoint, ev.Symbol = "quote", q.Get("symbols")
	case strings.HasPrefix(p, "/v1/finance/screener"):
		ev.Endpoint, ev.Symbol = "screener", q.Get("scrIds")
	case p == "/v1/test/getcrumb":
		ev.Endpoint = "getcrumb"
	}
	return ev
}

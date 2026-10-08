package gohoo

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Errores sentinela para clasificar fallas con errors.Is. Siguen siendo
// detectables aunque el error venga envuelto en "agotados N intentos: ...".
var (
	// ErrRateLimit indica que Yahoo limitó la tasa de requests (HTTP 429, o
	// un chart.error "Too Many Requests" con status 200 en ChartRange).
	ErrRateLimit = errors.New("gohoo: rate limit")
	// ErrUnauthorized indica un HTTP 401 que persistió tras renovar el crumb.
	ErrUnauthorized = errors.New("gohoo: no autorizado")
	// ErrNotFound indica que Yahoo no tiene datos para el símbolo (HTTP 404
	// o chart.error.code "Not Found").
	ErrNotFound = errors.New("gohoo: sin datos")
)

// StatusError es el error que devuelven los métodos de red ante una
// respuesta HTTP distinta de 200. Se obtiene con errors.As y se compara con
// los sentinelas vía errors.Is (429 → ErrRateLimit, 404 → ErrNotFound,
// 401 → ErrUnauthorized).
type StatusError struct {
	StatusCode int
	URL        string
	Body       string // primeros 300 bytes del cuerpo
	// RetryAfter es el header Retry-After de la respuesta (en segundos o
	// fecha HTTP), o 0 si Yahoo no lo mandó.
	RetryAfter time.Duration
}

// httpStatusError es el nombre que tenía StatusError antes de exportarse;
// se conserva como alias para el código interno y los tests existentes.
type httpStatusError = StatusError

func (e *StatusError) Error() string {
	return fmt.Sprintf("HTTP %d en %s: %s", e.StatusCode, e.URL, e.Body)
}

// Is permite errors.Is(err, ErrRateLimit) y similares sobre un StatusError.
func (e *StatusError) Is(target error) bool {
	switch target {
	case ErrRateLimit:
		return e.StatusCode == http.StatusTooManyRequests
	case ErrNotFound:
		return e.StatusCode == http.StatusNotFound
	case ErrUnauthorized:
		return e.StatusCode == http.StatusUnauthorized
	}
	return false
}

// IsRateLimit es un atajo de errors.Is(err, ErrRateLimit).
func IsRateLimit(err error) bool {
	return errors.Is(err, ErrRateLimit)
}

// parseRetryAfter interpreta el header Retry-After, que puede venir en
// segundos ("120") o como fecha HTTP. Devuelve 0 si falta o no se entiende.
func parseRetryAfter(h string, now time.Time) time.Duration {
	h = strings.TrimSpace(h)
	if h == "" {
		return 0
	}
	if secs, err := strconv.Atoi(h); err == nil {
		if secs <= 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(h); err == nil {
		if d := t.Sub(now); d > 0 {
			return d
		}
	}
	return 0
}

// chartAPIError convierte un {"chart":{"error":{...}}} de Yahoo en un error
// clasificable con errors.Is.
func chartAPIError(ticker, code, description string) error {
	msg := strings.TrimSpace(code + ": " + description)
	lower := strings.ToLower(code + " " + description)
	switch {
	case strings.Contains(lower, "too many"):
		return fmt.Errorf("chart de %s: %s: %w", ticker, msg, ErrRateLimit)
	case strings.EqualFold(code, "Not Found") || strings.Contains(lower, "no data found"):
		return fmt.Errorf("chart de %s: %s: %w", ticker, msg, ErrNotFound)
	}
	return fmt.Errorf("chart de %s devolvió error: %s", ticker, msg)
}

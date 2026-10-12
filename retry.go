package gohoo

import (
	"context"
	"math/rand/v2"
	"sync"
	"time"
)

// RetryPolicy controla cómo los métodos de red reintentan ante errores de
// red, HTTP 429 y 5xx (un HTTP 401 renueva el crumb y también consume un
// intento).
//
// MaxAttempts <= 0 y BaseDelay <= 0 toman los valores por defecto; los
// booleanos se toman tal cual, así que para cambiar solo un campo conviene
// partir de DefaultRetryPolicy().
type RetryPolicy struct {
	// MaxAttempts es el total de intentos (1 = sin reintentos). Default 4.
	MaxAttempts int
	// BaseDelay es el retardo antes del primer reintento; se duplica en cada
	// reintento (200ms, 400ms, 800ms...). Default 200ms.
	BaseDelay time.Duration
	// MaxDelay acota cada retardo, incluido el que pide Retry-After.
	// 0 = sin tope.
	MaxDelay time.Duration
	// Jitter (0..1) suma al retardo una fracción aleatoria de hasta
	// Jitter*retardo. Default 0.
	Jitter float64
	// RetryRateLimit reintenta los HTTP 429. Con false el 429 se devuelve
	// de inmediato (errors.Is(err, ErrRateLimit)) para que el llamador
	// aplique su propio cooldown.
	RetryRateLimit bool
	// HonorRetryAfter espera al menos lo que indique el header Retry-After
	// de la respuesta anterior.
	HonorRetryAfter bool
	// OnRetry, si no es nil, se llama antes de cada espera con el número de
	// intento que falló (1 = el primero), su error y el retardo a aplicar.
	OnRetry func(attempt int, err error, delay time.Duration)
	// RateLimit, si no es nil, da a los rate limits (HTTP 429, o un error
	// "too many requests" en el cuerpo de chart/screener/quote) su propio
	// presupuesto de reintentos y backoff: no consumen MaxAttempts y se
	// ignoran RetryRateLimit, BaseDelay, MaxDelay y Jitter para ellos. Con
	// nil (por defecto) los 429 se tratan como siempre.
	RateLimit *RateLimitPolicy
}

// RateLimitPolicy es el backoff ante rate limits de RetryPolicy.RateLimit:
// antes del reintento n (1 = el primero) espera
// min(BaseDelay·2^(n-1), MaxDelay) + U(0, Jitter), y al menos el
// Retry-After de la respuesta si HonorRetryAfter (este no se acota).
type RateLimitPolicy struct {
	// MaxRetries es cuántas veces se reintenta un rate limit; 0 = devolverlo
	// de inmediato (errors.Is(err, ErrRateLimit)).
	MaxRetries int
	// BaseDelay es la espera antes del primer reintento; se duplica en cada uno.
	BaseDelay time.Duration
	// MaxDelay acota la parte exponencial; 0 = sin tope.
	MaxDelay time.Duration
	// Jitter suma una espera aleatoria uniforme entre 0 y Jitter.
	Jitter time.Duration
	// HonorRetryAfter espera al menos el header Retry-After.
	HonorRetryAfter bool
}

func (p RateLimitPolicy) delay(retry int, lastErr *StatusError, rnd func() float64) time.Duration {
	d := max(p.BaseDelay, 0) << min(retry-1, 30)
	if d < 0 { // desborde
		d = time.Duration(1<<63 - 1)
	}
	if p.MaxDelay > 0 && d > p.MaxDelay {
		d = p.MaxDelay
	}
	if p.Jitter > 0 && rnd != nil {
		d += time.Duration(rnd() * float64(p.Jitter))
	}
	if p.HonorRetryAfter && lastErr != nil && lastErr.RetryAfter > d {
		d = lastErr.RetryAfter
	}
	return d
}

// DefaultRetryPolicy devuelve la política que usa NewClient: 4 intentos,
// backoff exponencial desde 200 ms, sin tope ni jitter, reintentando 429 e
// ignorando Retry-After.
func DefaultRetryPolicy() RetryPolicy {
	return RetryPolicy{
		MaxAttempts:    maxAttempts,
		BaseDelay:      retryBaseDelay,
		RetryRateLimit: true,
	}
}

type retryPolicyKey struct{}

// WithRetryPolicyContext devuelve un contexto con el que las llamadas del
// Client usan p en lugar de la política del cliente (p. ej. sin esperas
// largas dentro de un request web, y con reintentos pacientes en un
// proceso batch, sobre el mismo Client).
func WithRetryPolicyContext(ctx context.Context, p RetryPolicy) context.Context {
	return context.WithValue(ctx, retryPolicyKey{}, p)
}

// RetryPolicyFromContext devuelve la política puesta con
// WithRetryPolicyContext, si la hay.
func RetryPolicyFromContext(ctx context.Context) (RetryPolicy, bool) {
	p, ok := ctx.Value(retryPolicyKey{}).(RetryPolicy)
	return p, ok
}

// retryPolicyFor es la política de una llamada: la del contexto o la del cliente.
func (c *Client) retryPolicyFor(ctx context.Context) RetryPolicy {
	if p, ok := RetryPolicyFromContext(ctx); ok {
		return p.normalized()
	}
	return c.retryPolicy()
}

func (c *Client) retryPolicy() RetryPolicy {
	if c.conf().retry == nil {
		return DefaultRetryPolicy()
	}
	return c.conf().retry.normalized()
}

// normalized aplica los valores por defecto a los campos en cero.
func (p RetryPolicy) normalized() RetryPolicy {
	if p.MaxAttempts <= 0 {
		p.MaxAttempts = maxAttempts
	}
	if p.BaseDelay <= 0 {
		p.BaseDelay = retryBaseDelay
	}
	p.Jitter = min(max(p.Jitter, 0), 1)
	return p
}

// delay calcula la espera antes del intento número `attempt` (0-based,
// attempt >= 1) dado el error del intento anterior.
func (p RetryPolicy) delay(attempt int, lastErr *StatusError, rnd func() float64) time.Duration {
	d := p.BaseDelay << min(attempt-1, 30)
	if d <= 0 { // desborde
		d = time.Duration(1<<63 - 1)
	}
	if p.MaxDelay > 0 && d > p.MaxDelay {
		d = p.MaxDelay
	}
	if p.Jitter > 0 && rnd != nil {
		d += time.Duration(float64(d) * p.Jitter * rnd())
	}
	if p.HonorRetryAfter && lastErr != nil && lastErr.RetryAfter > d {
		d = lastErr.RetryAfter
	}
	if p.MaxDelay > 0 && d > p.MaxDelay {
		d = p.MaxDelay
	}
	return d
}

// jitterRand es un generador propio (no el global de math/rand) protegido
// por mutex, para que el jitter no altere secuencias deterministas de
// otros paquetes y sea seguro entre goroutines.
type jitterRand struct {
	mu sync.Mutex
	r  *rand.Rand
}

func (j *jitterRand) Float64() float64 {
	j.mu.Lock()
	defer j.mu.Unlock()
	if j.r == nil {
		j.r = rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0x9e3779b97f4a7c15))
	}
	return j.r.Float64()
}

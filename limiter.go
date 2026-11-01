package gohoo

import (
	"context"
	"sync"
	"time"
)

// intervalLimiter es el Limiter de NewIntervalLimiter.
type intervalLimiter struct {
	mu   sync.Mutex
	next time.Time
	gap  time.Duration
}

// NewIntervalLimiter devuelve un Limiter que deja pasar como máximo una
// request cada gap, sin importar cuántas goroutines esperen (se atienden en
// el orden en que llegan). Pasarlo a WithLimiter para fijar un ritmo máximo
// de todo el Client, p. ej. NewIntervalLimiter(40*time.Millisecond) son unas
// 25 requests por segundo. Con gap <= 0 no espera nunca.
//
// Wait devuelve ctx.Err() si ctx termina antes de que llegue el turno. Un
// turno ya reservado no se devuelve: las requests siguientes igual esperan.
func NewIntervalLimiter(gap time.Duration) Limiter {
	return &intervalLimiter{gap: gap}
}

func (l *intervalLimiter) Wait(ctx context.Context) error {
	l.mu.Lock()
	now := time.Now()
	at := l.next
	if at.Before(now) {
		at = now
	}
	if l.gap > 0 {
		l.next = at.Add(l.gap)
	}
	l.mu.Unlock()
	d := at.Sub(now)
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

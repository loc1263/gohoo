package gohoo

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestIntervalLimiterSpacesCalls(t *testing.T) {
	const gap = 30 * time.Millisecond
	l := NewIntervalLimiter(gap)
	start := time.Now()
	for i := 0; i < 4; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatalf("Wait: %v", err)
		}
	}
	// La primera pasa de inmediato; las otras tres esperan un gap cada una.
	if got := time.Since(start); got < 3*gap {
		t.Errorf("4 Wait tardaron %v, want >= %v", got, 3*gap)
	}
}

func TestIntervalLimiterConcurrent(t *testing.T) {
	const gap = 20 * time.Millisecond
	l := NewIntervalLimiter(gap)
	var wg sync.WaitGroup
	start := time.Now()
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = l.Wait(context.Background())
		}()
	}
	wg.Wait()
	if got := time.Since(start); got < 4*gap {
		t.Errorf("5 Wait concurrentes tardaron %v, want >= %v", got, 4*gap)
	}
}

func TestIntervalLimiterContextCanceled(t *testing.T) {
	l := NewIntervalLimiter(time.Hour)
	if err := l.Wait(context.Background()); err != nil { // toma el primer turno
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := l.Wait(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("err = %v, want DeadlineExceeded", err)
	}
}

func TestIntervalLimiterCanceledBeforeTurn(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := NewIntervalLimiter(0).Wait(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want Canceled", err)
	}
}

func TestIntervalLimiterZeroGapNeverWaits(t *testing.T) {
	l := NewIntervalLimiter(0)
	start := time.Now()
	for i := 0; i < 100; i++ {
		if err := l.Wait(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
	if got := time.Since(start); got > 50*time.Millisecond {
		t.Errorf("gap 0 tardó %v", got)
	}
}

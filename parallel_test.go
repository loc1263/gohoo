package gohoo

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestParallelMapKeepsOrderAndErrors(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8}
	boom := errors.New("boom")
	res := ParallelMap(context.Background(), items, 3, func(_ context.Context, n int) (int, error) {
		time.Sleep(time.Duration(9-n) * time.Millisecond) // los primeros terminan al final
		if n == 5 {
			return 0, boom
		}
		return n * n, nil
	})
	if len(res) != len(items) {
		t.Fatalf("len = %d", len(res))
	}
	for i, r := range res {
		if items[i] == 5 {
			if !errors.Is(r.Err, boom) {
				t.Errorf("res[%d].Err = %v", i, r.Err)
			}
			continue
		}
		if r.Err != nil || r.Value != items[i]*items[i] {
			t.Errorf("res[%d] = %+v", i, r)
		}
	}
}

func TestParallelMapLimitsWorkers(t *testing.T) {
	var cur, peak atomic.Int32
	ParallelMap(context.Background(), make([]int, 20), 4, func(context.Context, int) (int, error) {
		n := cur.Add(1)
		for {
			p := peak.Load()
			if n <= p || peak.CompareAndSwap(p, n) {
				break
			}
		}
		time.Sleep(5 * time.Millisecond)
		cur.Add(-1)
		return 0, nil
	})
	if p := peak.Load(); p > 4 || p < 2 {
		t.Errorf("concurrencia máxima = %d, want entre 2 y 4", p)
	}
}

func TestParallelMapEdgeCases(t *testing.T) {
	if res := ParallelMap(context.Background(), []int(nil), 4, func(context.Context, int) (int, error) { return 0, nil }); len(res) != 0 {
		t.Errorf("sin items: %v", res)
	}
	res := ParallelMap(context.Background(), []string{"a", "b"}, 0, func(_ context.Context, s string) (string, error) { return s + s, nil })
	if res[0].Value != "aa" || res[1].Value != "bb" {
		t.Errorf("workers 0: %+v", res)
	}
}

func TestParallelMapContextCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	var calls atomic.Int32
	res := ParallelMap(ctx, make([]int, 10), 1, func(context.Context, int) (int, error) {
		if calls.Add(1) == 3 {
			cancel()
		}
		return 1, nil
	})
	if got := calls.Load(); got != 3 {
		t.Errorf("fn se llamó %d veces, want 3", got)
	}
	for i, r := range res {
		if i < 3 && r.Err != nil {
			t.Errorf("res[%d].Err = %v", i, r.Err)
		}
		if i >= 3 && !errors.Is(r.Err, context.Canceled) {
			t.Errorf("res[%d].Err = %v, want Canceled", i, r.Err)
		}
	}
}

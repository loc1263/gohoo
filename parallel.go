package gohoo

import (
	"context"
	"sync"
)

// MapResult es lo que devolvió la función de ParallelMap para un elemento.
type MapResult[R any] struct {
	Value R
	Err   error
}

// ParallelMap corre fn sobre cada elemento de items con hasta workers
// goroutines y devuelve los resultados en el mismo orden que items (el
// resultado i es el del elemento i). Sirve para bajar muchos símbolos con un
// mismo Client, que es seguro entre goroutines; el ritmo de las requests lo
// fija WithLimiter, no workers.
//
// workers < 1 se toma como 1. Si ctx termina, los elementos que todavía no
// empezaron quedan con Err = ctx.Err() sin llamar a fn. No recupera panics
// de fn.
func ParallelMap[T, R any](ctx context.Context, items []T, workers int, fn func(ctx context.Context, item T) (R, error)) []MapResult[R] {
	out := make([]MapResult[R], len(items))
	workers = max(1, min(workers, len(items)))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				if err := ctx.Err(); err != nil {
					out[i].Err = err
					continue
				}
				v, err := fn(ctx, items[i])
				out[i] = MapResult[R]{v, err}
			}
		}()
	}
	for i := range items {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	return out
}

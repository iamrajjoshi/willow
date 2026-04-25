package parallel

import "sync"

const DefaultLimit = 8

// Map applies fn to each item with the default worker limit and preserves input order.
func Map[T, R any](items []T, fn func(int, T) R) []R {
	return MapLimit(items, DefaultLimit, fn)
}

// MapLimit applies fn to each item with at most limit workers and preserves input order.
func MapLimit[T, R any](items []T, limit int, fn func(int, T) R) []R {
	results := make([]R, len(items))
	if len(items) == 0 {
		return results
	}
	if limit <= 0 || limit > len(items) {
		limit = len(items)
	}

	var wg sync.WaitGroup
	jobs := make(chan int)

	for range limit {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				results[i] = fn(i, items[i])
			}
		}()
	}

	for i := range items {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	return results
}

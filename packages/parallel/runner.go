package parallel

import (
	"fmt"
	"sync"
)

// Run executes multiple workers in parallel and awaits for all of them to finish before returning
func Run(workers ...func() (err error)) error {
	var wg sync.WaitGroup
	wg.Add(len(workers))
	var mutex sync.Mutex
	errs := make([]error, 0, len(workers))
	for _, w := range workers {
		go func(worker func() error) {
			if err := worker(); err != nil {
				mutex.Lock()
				errs = append(errs, err)
				mutex.Unlock()
			}
			wg.Done()
		}(w)
	}
	wg.Wait()
	if len(errs) > 0 {
		return fmt.Errorf("failed %v out of %v workers, 1st error: %v", len(errs), len(workers), errs[0])
	}
	return nil
}

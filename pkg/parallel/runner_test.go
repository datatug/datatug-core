package parallel

import (
	"fmt"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRun(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		assert.Nil(t, Run())
	})

	t.Run("success", func(t *testing.T) {
		var counter int
		var mutex sync.Mutex
		worker := func() error {
			mutex.Lock()
			counter++
			mutex.Unlock()
			return nil
		}
		err := Run(worker, worker, worker)
		assert.NoError(t, err)
		assert.Equal(t, 3, counter)
	})

	t.Run("with_errors", func(t *testing.T) {
		err1 := fmt.Errorf("error 1")
		err2 := fmt.Errorf("error 2")
		workerSuccess := func() error { return nil }
		workerErr1 := func() error { return err1 }
		workerErr2 := func() error { return err2 }

		err := Run(workerSuccess, workerErr1, workerErr2)
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "failed 2 out of 3 workers")
		assert.Contains(t, err.Error(), "1st error:")
	})
}

package dataaccess

import (
	"context"
	"sync"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type recordsReader struct {
	operation sync.Mutex
	mu        sync.Mutex
	authority *Authority
	ctx       context.Context
	bound     *lease
	cancel    context.CancelFunc
	reader    dal.RecordsReader
	stop      func() bool
	closed    bool
	terminal  error
	closeErr  error
	closeOnce sync.Once
}

func newRecordsReader(authority *Authority, ctx context.Context, bound *lease, cancel context.CancelFunc, reader dal.RecordsReader) *recordsReader {
	r := &recordsReader{authority: authority, ctx: ctx, bound: bound, cancel: cancel, reader: reader}
	r.stop = context.AfterFunc(ctx, func() {
		r.terminate(authority.check(ctx, bound))
	})
	return r
}

// Closing never takes the operation mutex: providers may need Close to unblock
// Next/Cursor, and authority resolution itself may be blocked on the network.
func (r *recordsReader) terminate(terminal error) error {
	r.mu.Lock()
	if !r.closed {
		r.closed = true
		r.terminal = terminal
	}
	r.mu.Unlock()
	r.cancel()
	r.closeOnce.Do(func() {
		err := r.reader.Close()
		r.mu.Lock()
		r.closeErr = err
		r.mu.Unlock()
	})
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeErr
}

func (r *recordsReader) Close() error {
	r.stop()
	return r.terminate(nil)
}

func (r *recordsReader) stateError() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		if r.terminal != nil {
			return r.terminal
		}
		return r.authority.denial(DataRead, "READER_CLOSED")
	}
	return nil
}

func (r *recordsReader) release(operation func() error) error {
	r.operation.Lock()
	defer r.operation.Unlock()
	if err := r.stateError(); err != nil {
		return err
	}
	ctx, authorization, cancel, err := r.authority.prepare(r.ctx, r.bound)
	if err == nil {
		defer cancel()
		err = operation()
		if err == nil {
			err = r.authority.recheck(ctx, authorization)
		}
	}
	if err != nil {
		r.terminate(err)
		return err
	}
	return r.stateError()
}

func (r *recordsReader) Next() (rec record.Record, err error) {
	err = r.release(func() error {
		var readErr error
		rec, readErr = r.reader.Next()
		return readErr
	})
	if err != nil {
		return nil, err
	}
	return rec, nil
}

func (r *recordsReader) Cursor() (cursor string, err error) {
	err = r.release(func() error {
		var readErr error
		cursor, readErr = r.reader.Cursor()
		return readErr
	})
	if err != nil {
		return "", err
	}
	return cursor, nil
}

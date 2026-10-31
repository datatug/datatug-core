package dataaccess

import (
	"context"
	"sync"
	"time"

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
	timer     *time.Timer // retains any freshly shortened ceiling between pages
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
// The provider's close error is retained for Close; a read or authorization
// failure remains the primary error returned by the operation that failed.
func (r *recordsReader) terminate(terminal error) {
	r.mu.Lock()
	if r.timer != nil {
		r.timer.Stop()
	}
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
}

func (r *recordsReader) Close() error {
	r.stop()
	r.terminate(nil)
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.closeErr
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

// Snapshot immutable lease pointers briefly; resolution and naming I/O never
// hold the ownership lock used by Close and deadline cancellation.
func (r *recordsReader) boundSnapshot() *lease {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.bound
}

func (r *recordsReader) tightenCeiling(authorization *lease) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.closed && authorization.deadline.Before(r.bound.deadline) {
		r.bound = authorization
		// Next/Cursor have no context argument. Retain a shorter ceiling even
		// while fact/native I/O stalls or no later page is requested.
		if r.timer == nil {
			r.timer = time.AfterFunc(time.Until(authorization.deadline), func() { r.terminate(r.authority.denial(DataRead, "AUTHORIZATION_EXPIRED")) })
		} else {
			r.timer.Reset(time.Until(authorization.deadline))
		}
	}
}

func (r *recordsReader) release(operation func() error) error {
	r.operation.Lock()
	defer r.operation.Unlock()
	if err := r.stateError(); err != nil {
		return err
	}
	ctx, authorization, cancel, err := r.authority.prepareWithCeiling(r.ctx, r.boundSnapshot(), nil, r.tightenCeiling)
	if err == nil {
		defer cancel()
		err = r.stateError()
		if err == nil {
			err = operation()
		}
		if err == nil {
			err = r.authority.recheckWithCeiling(ctx, authorization, r.tightenCeiling)
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

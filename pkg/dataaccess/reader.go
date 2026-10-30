package dataaccess

import (
	"context"
	"sync"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/record"
)

type recordsReader struct {
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
}

func newRecordsReader(authority *Authority, ctx context.Context, bound *lease, cancel context.CancelFunc, reader dal.RecordsReader) *recordsReader {
	r := &recordsReader{authority: authority, ctx: ctx, bound: bound, cancel: cancel, reader: reader}
	r.stop = context.AfterFunc(ctx, func() {
		r.mu.Lock()
		defer r.mu.Unlock()
		if !r.closed {
			r.terminal = authority.check(ctx, bound)
			r.closeLocked()
		}
	})
	return r
}

func (r *recordsReader) closeLocked() {
	if !r.closed {
		r.closed = true
		r.closeErr = r.reader.Close()
		r.cancel()
	}
}

func (r *recordsReader) Close() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.stop()
	r.closeLocked()
	return r.closeErr
}

func (r *recordsReader) release(operation func() error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		if r.terminal != nil {
			return r.terminal
		}
		return r.authority.denial(DataRead, "READER_CLOSED")
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
		r.terminal = err
		r.closeLocked()
	}
	return err
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

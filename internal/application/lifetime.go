package application

import (
	"context"
	"errors"
	"sync"
)

var ErrClosed = errors.New("service is closing or closed")

// lifetime serializes operation admission with shutdown. Every admitted call is
// canceled and joined before its shared repository may be closed.
type lifetime struct {
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
	closed bool
	wg     sync.WaitGroup
}

func newLifetime(parent context.Context) *lifetime {
	ctx, cancel := context.WithCancel(parent)
	return &lifetime{ctx: ctx, cancel: cancel}
}
func (l *lifetime) begin(parent context.Context) (context.Context, func(), error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return nil, nil, ErrClosed
	}
	if e := l.ctx.Err(); e != nil {
		return nil, nil, e
	}
	if e := parent.Err(); e != nil {
		return nil, nil, e
	}
	ctx, cancel := context.WithCancel(parent)
	stop := context.AfterFunc(l.ctx, cancel)
	l.wg.Add(1)
	return ctx, func() { stop(); cancel(); l.wg.Done() }, nil
}
func (l *lifetime) stop() { l.mu.Lock(); l.closed = true; l.cancel(); l.mu.Unlock() }
func (l *lifetime) wait() { l.wg.Wait() }

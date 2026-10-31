package session

import (
	"context"
	"sync"
)

// flightGroup runs one computation per key at a time; concurrent callers
// share its result. A caller whose context ends stops waiting, but the shared
// computation keeps running for the others.
type flightGroup struct {
	mu    sync.Mutex
	calls map[string]*flightCall
}

type flightCall struct {
	done  chan struct{}
	value any
	err   error
}

func (g *flightGroup) Do(ctx context.Context, key string, fn func() (any, error)) (any, error) {
	g.mu.Lock()
	if g.calls == nil {
		g.calls = map[string]*flightCall{}
	}
	call, running := g.calls[key]
	if !running {
		call = &flightCall{done: make(chan struct{})}
		g.calls[key] = call
		go func() {
			call.value, call.err = fn()
			g.mu.Lock()
			delete(g.calls, key)
			g.mu.Unlock()
			close(call.done)
		}()
	}
	g.mu.Unlock()
	select {
	case <-call.done:
		return call.value, call.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

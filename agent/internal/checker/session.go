package checker

import "sync"

// peerGate keeps Reinit off the peer while any check on it is still running.
// A closed connection is marked dirty and rebuilt only after the last request ends.
type peerGate struct {
	mu       sync.Mutex
	inflight int
	dirty    bool
	reinit   func()
}

func newPeerGate(reinit func()) *peerGate {
	return &peerGate{reinit: reinit}
}

func (g *peerGate) begin() {
	g.mu.Lock()
	if g.dirty && g.inflight == 0 && g.reinit != nil {
		g.reinit()
		g.dirty = false
	}
	g.inflight++
	g.mu.Unlock()
}

func (g *peerGate) end(connClosed bool) {
	g.mu.Lock()
	if connClosed {
		g.dirty = true
	}
	if g.inflight > 0 {
		g.inflight--
	}
	if g.dirty && g.inflight == 0 && g.reinit != nil {
		g.reinit()
		g.dirty = false
	}
	g.mu.Unlock()
}

package checker

import "testing"

func TestPeerGateReinitWaitsUntilIdle(t *testing.T) {
	var n int
	g := newPeerGate(func() { n++ })

	g.begin()
	g.begin()
	g.end(true)
	if n != 0 {
		t.Fatalf("reinit while a request was still running, count = %d", n)
	}

	g.begin()
	if n != 0 {
		t.Fatalf("reinit while requests were in flight, count = %d", n)
	}
	g.end(false)
	g.end(false)
	if n != 1 {
		t.Fatalf("reinit count = %d, want 1", n)
	}
}

func TestPeerGateReinitOnLastClosedRequest(t *testing.T) {
	var n int
	g := newPeerGate(func() { n++ })

	g.begin()
	g.end(true)
	if n != 1 {
		t.Fatalf("reinit count = %d, want 1", n)
	}

	g.begin()
	if n != 1 {
		t.Fatalf("reinit on a clean session, count = %d", n)
	}
	g.end(false)
	if n != 1 {
		t.Fatalf("reinit without a closed connection, count = %d", n)
	}
}

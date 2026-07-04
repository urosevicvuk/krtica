// Package forward moves bytes between connections — the data plane's
// innermost loop (§8.1).
package forward

import (
	"io"
	"net"
	"sync"
)

// halfCloser is satisfied by connections that can close their write side
// while still reading (net.TCPConn, yamux streams via Close semantics).
type halfCloser interface {
	CloseWrite() error
}

// Splice copies bytes in both directions between a and b until both sides
// finish, propagating EOF via half-close so each peer sees the other's
// shutdown in order. It closes both connections before returning.
func Splice(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go cp(&wg, a, b)
	go cp(&wg, b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}

// cp copies src→dst, then signals EOF downstream: CloseWrite when the
// conn supports half-close, full Close otherwise (yamux streams treat
// Close as FIN and still deliver pending reads on the other direction).
func cp(wg *sync.WaitGroup, dst, src net.Conn) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	if hc, ok := dst.(halfCloser); ok {
		_ = hc.CloseWrite()
		return
	}
	_ = dst.Close()
}

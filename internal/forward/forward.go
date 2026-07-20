package forward

import (
	"io"
	"net"
	"sync"
)

// halfCloser is a net.Conn that can be half-closed
type halfCloser interface {
	CloseWrite() error
}

// Splice splices two net.Conns
func Splice(a, b net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)
	go cp(&wg, a, b)
	go cp(&wg, b, a)
	wg.Wait()
	_ = a.Close()
	_ = b.Close()
}

// cp copies data between two net.Conns
func cp(wg *sync.WaitGroup, dst, src net.Conn) {
	defer wg.Done()
	_, _ = io.Copy(dst, src)
	if hc, ok := dst.(halfCloser); ok {
		_ = hc.CloseWrite()
		return
	}
	_ = dst.Close()
}

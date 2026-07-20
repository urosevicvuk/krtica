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

	wg.Go(func() { cp(a, b) })
	wg.Go(func() { cp(b, a) })

	wg.Wait()

	_ = a.Close()
	_ = b.Close()
}

// cp copies data between two net.Conns
func cp(dst, src net.Conn) {
	_, _ = io.Copy(dst, src)
	if hc, ok := dst.(halfCloser); ok {
		_ = hc.CloseWrite()
		return
	}
	_ = dst.Close()
}

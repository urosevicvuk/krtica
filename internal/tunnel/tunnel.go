// Package tunnel defines the carrier seam between krtica's tunnels
// and the layers above them (router, forwarder, control plane); see
// docs/DESIGN.md §4.3. Upper layers depend only on Tunnel — carrier
// libraries (yamux, QUIC) must not be imported outside this package, which
// is what keeps carriers swappable (Decision #3).
package tunnel

import (
	"context"
	"errors"
	"net"
)

var (
	ErrClosed = errors.New("tunnel: closed")

	ErrDatagramsUnsupported = errors.New("tunnel: datagrams unsupported by this carrier")
)

type Tunnel interface {
	OpenStream(ctx context.Context) (net.Conn, error)

	AcceptStream(ctx context.Context) (net.Conn, error)

	SendDatagram(ctx context.Context, p []byte) error

	RecvDatagram(ctx context.Context) ([]byte, error)

	Close() error
}

package tunnel

import (
	"context"
	"errors"
	"net"
)

var (
	// ErrClosed is returned when a tunnel is closed
	ErrClosed = errors.New("tunnel: closed")

	// ErrDatagramsUnsupported is returned when datagrams are unsupported by the carrier
	ErrDatagramsUnsupported = errors.New("tunnel: datagrams unsupported by this carrier")
)

// Tunnel is a multiplexed, encrypted, bidirectional stream
type Tunnel interface {
	OpenStream(ctx context.Context) (net.Conn, error)

	AcceptStream(ctx context.Context) (net.Conn, error)

	SendDatagram(ctx context.Context, p []byte) error

	RecvDatagram(ctx context.Context) ([]byte, error)

	Close() error
}

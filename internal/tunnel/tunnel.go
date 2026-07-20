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

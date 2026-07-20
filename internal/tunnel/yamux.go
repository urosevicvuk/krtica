package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/hashicorp/yamux"
)

// Yamux is a yamux-over-TCP tunnel
type Yamux struct {
	sess *yamux.Session
}

var _ Tunnel = (*Yamux)(nil)

// NewYamuxClient creates a new Yamux client from a net.Conn
func NewYamuxClient(conn net.Conn) (*Yamux, error) {
	sess, err := yamux.Client(conn, yamuxConfig())
	if err != nil {
		return nil, fmt.Errorf("tunnel: yamux client: %w", err)
	}
	return &Yamux{sess: sess}, nil
}

// NewYamuxServer creates a new Yamux server from a net.Conn
func NewYamuxServer(conn net.Conn) (*Yamux, error) {
	sess, err := yamux.Server(conn, yamuxConfig())
	if err != nil {
		return nil, fmt.Errorf("tunnel: yamux server: %w", err)
	}
	return &Yamux{sess: sess}, nil
}

// yamuxConfig returns the default yamux config
func yamuxConfig() *yamux.Config {
	return yamux.DefaultConfig()
}

// OpenStream opens a stream
func (t *Yamux) OpenStream(ctx context.Context) (net.Conn, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s, err := t.sess.OpenStream()
	if err != nil {
		return nil, mapErr("open stream", err)
	}
	return s, nil
}

// AcceptStream accepts a stream
func (t *Yamux) AcceptStream(ctx context.Context) (net.Conn, error) {
	s, err := t.sess.AcceptStreamWithContext(ctx)
	if err != nil {
		return nil, mapErr("accept stream", err)
	}
	return s, nil
}

// SendDatagram sends a datagram
func (t *Yamux) SendDatagram(_ context.Context, _ []byte) error {
	return ErrDatagramsUnsupported
}

// RecvDatagram receives a datagram
func (t *Yamux) RecvDatagram(_ context.Context) ([]byte, error) {
	return nil, ErrDatagramsUnsupported
}

// Close closes the tunnel
func (t *Yamux) Close() error {
	return t.sess.Close()
}

// mapErr maps yamux errors to tunnel errors
func mapErr(op string, err error) error {
	if errors.Is(err, yamux.ErrSessionShutdown) {
		return fmt.Errorf("tunnel: %s: %w", op, ErrClosed)
	}
	return fmt.Errorf("tunnel: %s: %w", op, err)
}

package tunnel

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/hashicorp/yamux"
)

type Yamux struct {
	sess *yamux.Session
}

var _ Tunnel = (*Yamux)(nil)

func NewYamuxClient(conn net.Conn) (*Yamux, error) {
	sess, err := yamux.Client(conn, yamuxConfig())
	if err != nil {
		return nil, fmt.Errorf("tunnel: yamux client: %w", err)
	}
	return &Yamux{sess: sess}, nil
}

func NewYamuxServer(conn net.Conn) (*Yamux, error) {
	sess, err := yamux.Server(conn, yamuxConfig())
	if err != nil {
		return nil, fmt.Errorf("tunnel: yamux server: %w", err)
	}
	return &Yamux{sess: sess}, nil
}

func yamuxConfig() *yamux.Config {
	return yamux.DefaultConfig()
}

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

func (t *Yamux) AcceptStream(ctx context.Context) (net.Conn, error) {
	s, err := t.sess.AcceptStreamWithContext(ctx)
	if err != nil {
		return nil, mapErr("accept stream", err)
	}
	return s, nil
}

func (t *Yamux) SendDatagram(_ context.Context, _ []byte) error {
	return ErrDatagramsUnsupported
}

func (t *Yamux) RecvDatagram(_ context.Context) ([]byte, error) {
	return nil, ErrDatagramsUnsupported
}

func (t *Yamux) Close() error {
	return t.sess.Close()
}

func mapErr(op string, err error) error {
	if errors.Is(err, yamux.ErrSessionShutdown) {
		return fmt.Errorf("tunnel: %s: %w", op, ErrClosed)
	}
	return fmt.Errorf("tunnel: %s: %w", op, err)
}

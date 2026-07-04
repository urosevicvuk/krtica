package transport

import (
	"context"
	"errors"
	"fmt"
	"net"

	"github.com/hashicorp/yamux"
)

// Yamux carries a tunnel over a single reliable connection (TCP+TLS in
// production) by multiplexing it into logical streams. It has no datagram
// channel: SendDatagram and RecvDatagram always return
// ErrDatagramsUnsupported, so v1 UDP forwarding runs stream-framed
// instead (§8.2).
type Yamux struct {
	sess *yamux.Session
}

var _ Transport = (*Yamux)(nil)

// NewYamuxClient wraps conn as the dialing (agent) side of a tunnel.
// The caller retains responsibility for dial, TLS, and auth (§3.4); conn
// must be ready to carry traffic. Closing the transport closes conn.
func NewYamuxClient(conn net.Conn) (*Yamux, error) {
	sess, err := yamux.Client(conn, yamuxConfig())
	if err != nil {
		return nil, fmt.Errorf("transport: yamux client: %w", err)
	}
	return &Yamux{sess: sess}, nil
}

// NewYamuxServer wraps conn as the accepting (server) side of a tunnel.
func NewYamuxServer(conn net.Conn) (*Yamux, error) {
	sess, err := yamux.Server(conn, yamuxConfig())
	if err != nil {
		return nil, fmt.Errorf("transport: yamux server: %w", err)
	}
	return &Yamux{sess: sess}, nil
}

// yamuxConfig is shared by both sides so window sizes and keepalive
// behavior stay symmetric. Library defaults are fine for Phase 1; the
// global P1 caps arrive with the Phase 2 robustness work.
func yamuxConfig() *yamux.Config {
	return yamux.DefaultConfig()
}

// OpenStream opens a logical stream to the peer. ctx is checked before
// dispatch; yamux has no context-aware open, so the blocking wait for the
// peer's ack is bounded by the config's StreamOpenTimeout instead.
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

// AcceptStream blocks until the peer opens a stream, ctx is done, or the
// session closes.
func (t *Yamux) AcceptStream(ctx context.Context) (net.Conn, error) {
	s, err := t.sess.AcceptStreamWithContext(ctx)
	if err != nil {
		return nil, mapErr("accept stream", err)
	}
	return s, nil
}

// SendDatagram always fails: this carrier has no unreliable channel.
func (t *Yamux) SendDatagram(_ context.Context, _ []byte) error {
	return ErrDatagramsUnsupported
}

// RecvDatagram always fails: this carrier has no unreliable channel.
func (t *Yamux) RecvDatagram(_ context.Context) ([]byte, error) {
	return nil, ErrDatagramsUnsupported
}

// Close shuts down the session and all its streams. yamux makes repeated
// closes safe, which satisfies the interface's idempotence requirement.
func (t *Yamux) Close() error {
	return t.sess.Close()
}

// mapErr translates yamux errors into this package's sentinels so callers
// only ever branch on Transport's documented errors, never on yamux's.
func mapErr(op string, err error) error {
	if errors.Is(err, yamux.ErrSessionShutdown) {
		return fmt.Errorf("transport: %s: %w", op, ErrClosed)
	}
	return fmt.Errorf("transport: %s: %w", op, err)
}

// Package transport defines the carrier seam between krtica's tunnel
// transports and the layers above it (router, forwarder, control plane);
// see docs/DESIGN.md §4.3. Upper layers depend only on Transport — carrier
// libraries (yamux, QUIC) must not be imported outside this package, which
// is what keeps carriers swappable (Decision #3).
package transport

import (
	"context"
	"errors"
	"net"
)

var (
	// ErrClosed is returned by all methods after Close, and by in-flight
	// calls interrupted by it. Reconnect policy (§3.4) belongs to the
	// caller, not the transport.
	ErrClosed = errors.New("transport: closed")

	// ErrDatagramsUnsupported is returned by SendDatagram and RecvDatagram
	// on carriers with no unreliable channel (yamux-over-TCP, §8.2);
	// callers fall back to stream-framed UDP.
	ErrDatagramsUnsupported = errors.New("transport: datagrams unsupported by this carrier")
)

// Transport is one established, authenticated tunnel between agent and
// server, multiplexing logical streams (§5) and, on capable carriers,
// unreliable datagrams (§8.2). Constructing one is the concrete
// implementation's job and happens only after dial, TLS, and auth
// succeed (§3.4); both peers then hold a Transport.
//
// Implementations must be safe for concurrent use, must keep stream
// counts and buffered bytes bounded (P1) — OpenStream fails fast when
// global caps are hit rather than queueing — and must unblock all
// pending calls with ErrClosed when the transport closes.
type Transport interface {
	// OpenStream opens a logical stream to the peer, honoring ctx during
	// setup. It fails fast with ErrClosed on a dead tunnel.
	OpenStream(ctx context.Context) (net.Conn, error)

	// AcceptStream blocks until the peer opens a stream, ctx is done, or
	// the transport closes.
	AcceptStream(ctx context.Context) (net.Conn, error)

	// SendDatagram sends one unreliable, unordered message. Payloads over
	// the carrier's datagram MTU fail; they are never fragmented.
	SendDatagram(ctx context.Context, p []byte) error

	// RecvDatagram blocks for the next inbound datagram. The returned
	// slice is owned by the caller.
	RecvDatagram(ctx context.Context) ([]byte, error)

	// Close tears down the tunnel and all its streams, unblocking pending
	// calls. It is idempotent.
	Close() error
}

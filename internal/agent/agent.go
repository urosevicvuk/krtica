// Package agent implements the mole (§3): it dials the server, holds the
// tunnel open, and splices incoming streams to local targets.
package agent

import (
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"time"

	"github.com/urosevicvuk/krtica/internal/config"
	"github.com/urosevicvuk/krtica/internal/forward"
	"github.com/urosevicvuk/krtica/internal/proto"
	"github.com/urosevicvuk/krtica/internal/proto/pb"
	"github.com/urosevicvuk/krtica/internal/transport"
)

const (
	handshakeTimeout = 10 * time.Second
	dialTimeout      = 10 * time.Second
)

// Agent maintains one tunnel to the server and serves its streams.
type Agent struct {
	cfg     *config.Agent
	log     *slog.Logger
	targets map[string]string
}

// New builds an Agent from validated config.
func New(cfg *config.Agent, log *slog.Logger) *Agent {
	targets := make(map[string]string, len(cfg.Services))
	for _, svc := range cfg.Services {
		targets[svc.Name] = svc.Target
	}
	return &Agent{cfg: cfg, log: log, targets: targets}
}

// Run dials the server, authenticates, and serves tunnel streams until
// ctx is canceled or the tunnel drops. Reconnect-with-backoff wraps this
// in Phase 2 (§3.4); for now a dropped tunnel ends Run.
func (a *Agent) Run(ctx context.Context) error {
	dialer := &net.Dialer{Timeout: dialTimeout}
	// Phase 1 stopgap: the server's certificate is ephemeral self-signed
	// (see internal/server), so there is no chain to verify yet. The
	// tunnel is encrypted but the server is unauthenticated; enrollment
	// (pinning / mTLS / Noise, §20) closes this hole before real use.
	tlsCfg := &tls.Config{InsecureSkipVerify: true} //nolint:gosec // see above
	conn, err := tls.DialWithDialer(dialer, "tcp", a.cfg.Server, tlsCfg)
	if err != nil {
		return fmt.Errorf("agent: dial server: %w", err)
	}

	if err := a.handshake(conn); err != nil {
		_ = conn.Close()
		return fmt.Errorf("agent: handshake: %w", err)
	}

	tr, err := transport.NewYamuxClient(conn)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = tr.Close() }()
	a.log.Info("tunnel established", "server", a.cfg.Server)

	for {
		stream, err := tr.AcceptStream(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("agent: tunnel lost: %w", err)
		}
		go a.serveStream(stream)
	}
}

// handshake sends Hello and waits for the server's verdict, bounded by a
// deadline covering the whole exchange.
func (a *Agent) handshake(conn net.Conn) error {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return err
	}
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	services := make([]string, 0, len(a.targets))
	for name := range a.targets {
		services = append(services, name)
	}
	hello := &pb.Hello{
		ProtocolVersion: proto.ProtocolVersion,
		AgentName:       a.cfg.Name,
		Token:           a.cfg.Token,
		Services:        services,
	}
	if err := proto.WriteFrame(conn, hello); err != nil {
		return err
	}
	var ack pb.HelloAck
	if err := proto.ReadFrame(conn, &ack); err != nil {
		return err
	}
	if !ack.Ok {
		return fmt.Errorf("server rejected agent: %s", ack.Error)
	}
	return nil
}

// serveStream handles one incoming stream: read which service the server
// wants, dial the local target, splice.
func (a *Agent) serveStream(stream net.Conn) {
	if err := stream.SetReadDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		_ = stream.Close()
		return
	}
	var hdr pb.StreamHeader
	if err := proto.ReadFrame(stream, &hdr); err != nil {
		a.log.Warn("bad stream header", "err", err)
		_ = stream.Close()
		return
	}
	_ = stream.SetReadDeadline(time.Time{})

	target, ok := a.targets[hdr.Service]
	if !ok {
		a.log.Warn("unknown service requested", "service", hdr.Service)
		_ = stream.Close()
		return
	}
	local, err := net.DialTimeout("tcp", target, dialTimeout)
	if err != nil {
		a.log.Warn("local dial failed", "service", hdr.Service, "target", target, "err", err)
		_ = stream.Close()
		return
	}
	forward.Splice(stream, local)
}

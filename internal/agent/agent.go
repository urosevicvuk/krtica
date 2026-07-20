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
	"github.com/urosevicvuk/krtica/internal/tunnel"
	"github.com/urosevicvuk/krtica/internal/wire"
	"github.com/urosevicvuk/krtica/internal/wire/pb"
)

const (
	handshakeTimeout = 10 * time.Second
	dialTimeout      = 10 * time.Second
)

type Agent struct {
	cfg     *config.Agent
	log     *slog.Logger
	targets map[string]string
}

func New(cfg *config.Agent, log *slog.Logger) *Agent {
	targets := make(map[string]string, len(cfg.Services))
	for _, svc := range cfg.Services {
		targets[svc.Name] = svc.Target
	}
	return &Agent{cfg: cfg, log: log, targets: targets}
}

func (a *Agent) Run(ctx context.Context) error {
	dialer := &net.Dialer{Timeout: dialTimeout}

	tlsCfg := &tls.Config{InsecureSkipVerify: true}
	conn, err := tls.DialWithDialer(dialer, "tcp", a.cfg.Server, tlsCfg)
	if err != nil {
		return fmt.Errorf("agent: dial server: %w", err)
	}

	if err := a.handshake(conn); err != nil {
		_ = conn.Close()
		return fmt.Errorf("agent: handshake: %w", err)
	}

	tun, err := tunnel.NewYamuxClient(conn)
	if err != nil {
		_ = conn.Close()
		return err
	}
	defer func() { _ = tun.Close() }()
	a.log.Info("tunnel established", "server", a.cfg.Server)

	for {
		stream, err := tun.AcceptStream(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("agent: tunnel lost: %w", err)
		}
		go a.serveStream(stream)
	}
}

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
		ProtocolVersion: wire.ProtocolVersion,
		AgentName:       a.cfg.Name,
		Token:           a.cfg.Token,
		Services:        services,
	}
	if err := wire.WriteFrame(conn, hello); err != nil {
		return err
	}
	var ack pb.HelloAck
	if err := wire.ReadFrame(conn, &ack); err != nil {
		return err
	}
	if !ack.Ok {
		return fmt.Errorf("server rejected agent: %s", ack.Error)
	}
	return nil
}

func (a *Agent) serveStream(stream net.Conn) {
	if err := stream.SetReadDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		_ = stream.Close()
		return
	}
	var hdr pb.StreamHeader
	if err := wire.ReadFrame(stream, &hdr); err != nil {
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

package server

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/urosevicvuk/krtica/internal/config"
	"github.com/urosevicvuk/krtica/internal/forward"
	"github.com/urosevicvuk/krtica/internal/tunnel"
	"github.com/urosevicvuk/krtica/internal/wire"
	"github.com/urosevicvuk/krtica/internal/wire/pb"
)

const handshakeTimeout = 10 * time.Second

type Server struct {
	cfg *config.Server
	log *slog.Logger
	mu sync.Mutex
	backends map[string]tunnel.Tunnel
}

func New(cfg *config.Server, log *slog.Logger) *Server {
	return &Server{
		cfg:      cfg,
		log:      log,
		backends: make(map[string]tunnel.Tunnel),
	}
}

func (s *Server) Run(ctx context.Context) error {
	tlsCfg, err := selfSignedTLS()
	if err != nil {
		return err
	}
	agentLn, err := tls.Listen("tcp", s.cfg.AgentListen, tlsCfg)
	if err != nil {
		return fmt.Errorf("server: listen for agents: %w", err)
	}
	s.log.Info("listening for agents", "addr", agentLn.Addr().String())

	var wg sync.WaitGroup
	lns := []net.Listener{agentLn}

	for _, rt := range s.cfg.Routes {
		ln, err := net.Listen("tcp", rt.Listen)
		if err != nil {
			closeAll(lns)
			return fmt.Errorf("server: listen %s for route %q: %w", rt.Listen, rt.Name, err)
		}
		lns = append(lns, ln)
		s.log.Info("public listener up", "route", rt.Name, "addr", ln.Addr().String())

		wg.Go(func() {
			s.acceptPublic(ctx, rt, ln)
		})
	}

	wg.Go(func() {
		s.acceptAgents(ctx, agentLn)
	})

	<-ctx.Done()
	closeAll(lns)
	wg.Wait()
	return nil
}

func (s *Server) acceptAgents(ctx context.Context, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("agent accept failed", "err", err)
			return
		}
		go s.handleAgent(ctx, conn)
	}
}

func (s *Server) handleAgent(ctx context.Context, conn net.Conn) {
	log := s.log.With("remote", conn.RemoteAddr().String())

	hello, err := s.handshake(conn)
	if err != nil {
		log.Warn("agent rejected", "err", err)
		_ = conn.Close()
		return
	}
	log = log.With("agent", hello.AgentName)

	tun, err := tunnel.NewYamuxServer(conn)
	if err != nil {
		log.Error("mux setup failed", "err", err)
		_ = conn.Close()
		return
	}

	s.register(hello.Services, tun)
	log.Info("agent connected", "services", hello.Services)

	_, err = tun.AcceptStream(ctx)
	if err != nil && !errors.Is(err, tunnel.ErrClosed) && ctx.Err() == nil {
		log.Warn("tunnel closed", "err", err)
	}
	s.unregister(hello.Services, tun)
	_ = tun.Close()
	log.Info("agent disconnected")
}

func (s *Server) handshake(conn net.Conn) (*pb.Hello, error) {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return nil, err
	}
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	var hello pb.Hello
	if err := wire.ReadFrame(conn, &hello); err != nil {
		return nil, err
	}
	if hello.ProtocolVersion != wire.ProtocolVersion {
		_ = wire.WriteFrame(conn, &pb.HelloAck{Error: "unsupported protocol version"})
		return nil, fmt.Errorf("protocol version %d, want %d", hello.ProtocolVersion, wire.ProtocolVersion)
	}
	if subtle.ConstantTimeCompare([]byte(hello.Token), []byte(s.cfg.Token)) != 1 {
		_ = wire.WriteFrame(conn, &pb.HelloAck{Error: "invalid token"})
		return nil, errors.New("invalid token")
	}
	if err := wire.WriteFrame(conn, &pb.HelloAck{Ok: true}); err != nil {
		return nil, err
	}
	return &hello, nil
}

func (s *Server) register(services []string, tun tunnel.Tunnel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range services {
		s.backends[name] = tun
	}
}

func (s *Server) unregister(services []string, tun tunnel.Tunnel) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range services {
		if s.backends[name] == tun {
			delete(s.backends, name)
		}
	}
}

func (s *Server) lookup(service string) (tunnel.Tunnel, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tun, ok := s.backends[service]
	return tun, ok
}

func (s *Server) acceptPublic(ctx context.Context, rt config.Route, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("public accept failed", "route", rt.Name, "err", err)
			return
		}
		go s.servePublic(ctx, rt, conn)
	}
}

func (s *Server) servePublic(ctx context.Context, rt config.Route, conn net.Conn) {
	tun, ok := s.lookup(rt.Name)
	if !ok {
		s.log.Warn("no backend for route", "route", rt.Name)
		_ = conn.Close()
		return
	}
	stream, err := tun.OpenStream(ctx)
	if err != nil {
		s.log.Warn("open stream failed", "route", rt.Name, "err", err)
		_ = conn.Close()
		return
	}
	if err := wire.WriteFrame(stream, &pb.StreamHeader{Service: rt.Name}); err != nil {
		s.log.Warn("stream header failed", "route", rt.Name, "err", err)
		_ = stream.Close()
		_ = conn.Close()
		return
	}
	forward.Splice(conn, stream)
}

func closeAll(lns []net.Listener) {
	for _, ln := range lns {
		_ = ln.Close()
	}
}

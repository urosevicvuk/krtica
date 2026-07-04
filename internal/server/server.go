// Package server implements the molehill (§3): it accepts agent tunnels
// on one listener and public visitors on the others, routing each public
// connection down the right tunnel as a fresh stream.
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
	"github.com/urosevicvuk/krtica/internal/proto"
	"github.com/urosevicvuk/krtica/internal/proto/pb"
	"github.com/urosevicvuk/krtica/internal/transport"
)

const handshakeTimeout = 10 * time.Second

// Server routes public ingress to agent tunnels. One Server owns all
// listeners; Run blocks until ctx is canceled.
type Server struct {
	cfg *config.Server
	log *slog.Logger

	mu sync.Mutex
	// agents maps advertised service name → the transport of the agent
	// that advertised it. Phase 1: last agent to advertise wins; edge LB
	// across duplicates arrives in Phase 3 (§7).
	agents map[string]transport.Transport
}

// New builds a Server from validated config.
func New(cfg *config.Server, log *slog.Logger) *Server {
	return &Server{
		cfg:    cfg,
		log:    log,
		agents: make(map[string]transport.Transport),
	}
}

// Run listens for agents and public visitors until ctx is canceled.
func (s *Server) Run(ctx context.Context) error {
	tlsCfg, err := selfSignedTLS()
	if err != nil {
		return err
	}
	agentLn, err := tls.Listen("tcp", s.cfg.Listen, tlsCfg)
	if err != nil {
		return fmt.Errorf("server: listen for agents: %w", err)
	}
	s.log.Info("listening for agents", "addr", agentLn.Addr().String())

	var wg sync.WaitGroup
	lns := []net.Listener{agentLn}

	for _, tn := range s.cfg.Tunnels {
		ln, err := net.Listen("tcp", tn.Listen)
		if err != nil {
			closeAll(lns)
			return fmt.Errorf("server: listen %s for tunnel %q: %w", tn.Listen, tn.Name, err)
		}
		lns = append(lns, ln)
		s.log.Info("public listener up", "tunnel", tn.Name, "addr", ln.Addr().String())

		wg.Add(1)
		go func(tn config.Tunnel, ln net.Listener) {
			defer wg.Done()
			s.acceptPublic(ctx, tn, ln)
		}(tn, ln)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		s.acceptAgents(ctx, agentLn)
	}()

	<-ctx.Done()
	closeAll(lns)
	wg.Wait()
	return nil
}

// acceptAgents runs the agent-listener loop: one goroutine per tunnel
// handshake so a stalled agent cannot block new ones.
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

// handleAgent authenticates one agent connection and, on success,
// registers its advertised services and keeps the tunnel until it drops.
func (s *Server) handleAgent(ctx context.Context, conn net.Conn) {
	log := s.log.With("remote", conn.RemoteAddr().String())

	hello, err := s.handshake(conn)
	if err != nil {
		log.Warn("agent rejected", "err", err)
		_ = conn.Close()
		return
	}
	log = log.With("agent", hello.AgentName)

	tr, err := transport.NewYamuxServer(conn)
	if err != nil {
		log.Error("mux setup failed", "err", err)
		_ = conn.Close()
		return
	}

	s.register(hello.Services, tr)
	log.Info("agent connected", "services", hello.Services)

	// Block until the tunnel dies: the agent opens no streams toward us
	// in Phase 1, so the first Accept result signals session end.
	_, err = tr.AcceptStream(ctx)
	if err != nil && !errors.Is(err, transport.ErrClosed) && ctx.Err() == nil {
		log.Warn("tunnel closed", "err", err)
	}
	s.unregister(hello.Services, tr)
	_ = tr.Close()
	log.Info("agent disconnected")
}

// handshake enforces protocol version and token before any multiplexing
// starts. The deadline covers the whole exchange so a silent peer cannot
// hold a goroutine forever (P1).
func (s *Server) handshake(conn net.Conn) (*pb.Hello, error) {
	if err := conn.SetDeadline(time.Now().Add(handshakeTimeout)); err != nil {
		return nil, err
	}
	defer func() { _ = conn.SetDeadline(time.Time{}) }()

	var hello pb.Hello
	if err := proto.ReadFrame(conn, &hello); err != nil {
		return nil, err
	}
	if hello.ProtocolVersion != proto.ProtocolVersion {
		_ = proto.WriteFrame(conn, &pb.HelloAck{Error: "unsupported protocol version"})
		return nil, fmt.Errorf("protocol version %d, want %d", hello.ProtocolVersion, proto.ProtocolVersion)
	}
	if subtle.ConstantTimeCompare([]byte(hello.Token), []byte(s.cfg.Token)) != 1 {
		_ = proto.WriteFrame(conn, &pb.HelloAck{Error: "invalid token"})
		return nil, errors.New("invalid token")
	}
	if err := proto.WriteFrame(conn, &pb.HelloAck{Ok: true}); err != nil {
		return nil, err
	}
	return &hello, nil
}

func (s *Server) register(services []string, tr transport.Transport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range services {
		s.agents[name] = tr
	}
}

// unregister removes services only if they still point at tr, so a
// reconnected agent's fresh registration is never torn down by the old
// tunnel's cleanup.
func (s *Server) unregister(services []string, tr transport.Transport) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, name := range services {
		if s.agents[name] == tr {
			delete(s.agents, name)
		}
	}
}

func (s *Server) lookup(service string) (transport.Transport, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	tr, ok := s.agents[service]
	return tr, ok
}

// acceptPublic accepts visitors on one tunnel's public listener.
func (s *Server) acceptPublic(ctx context.Context, tn config.Tunnel, ln net.Listener) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.Error("public accept failed", "tunnel", tn.Name, "err", err)
			return
		}
		go s.servePublic(ctx, tn, conn)
	}
}

// servePublic routes one public connection: find the agent advertising
// the service, open a stream, send the header, splice.
func (s *Server) servePublic(ctx context.Context, tn config.Tunnel, conn net.Conn) {
	tr, ok := s.lookup(tn.Name)
	if !ok {
		s.log.Warn("no agent for tunnel", "tunnel", tn.Name)
		_ = conn.Close()
		return
	}
	stream, err := tr.OpenStream(ctx)
	if err != nil {
		s.log.Warn("open stream failed", "tunnel", tn.Name, "err", err)
		_ = conn.Close()
		return
	}
	if err := proto.WriteFrame(stream, &pb.StreamHeader{Service: tn.Name}); err != nil {
		s.log.Warn("stream header failed", "tunnel", tn.Name, "err", err)
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

package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"testing"
	"time"

	"github.com/urosevicvuk/krtica/internal/agent"
	"github.com/urosevicvuk/krtica/internal/config"
	"github.com/urosevicvuk/krtica/internal/server"
)

func freePort(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freePort: %v", err)
	}
	port := ln.Addr().(*net.TCPAddr).Port
	_ = ln.Close()
	return port
}

func echoServer(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echoServer: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer func() { _ = c.Close() }()
				b, _ := io.ReadAll(c)
				_, _ = fmt.Fprintf(c, "echo:%s", b)
			}(conn)
		}
	}()
	return ln.Addr().String()
}

func dialRetry(t *testing.T, addr string, timeout time.Duration) net.Conn {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			return conn
		}
		if time.Now().After(deadline) {
			t.Fatalf("dialRetry %s: %v", addr, err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestTCPTunnelEndToEnd(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	target := echoServer(t)
	agentPort := freePort(t)
	publicPort := freePort(t)

	srvCfg := &config.Server{
		AgentListen: fmt.Sprintf("127.0.0.1:%d", agentPort),
		Token:       "test-token",
		Routes: []config.Route{
			{Name: "echo", Listen: fmt.Sprintf("127.0.0.1:%d", publicPort)},
		},
	}
	agCfg := &config.Agent{
		Name:   "test-agent",
		Server: fmt.Sprintf("127.0.0.1:%d", agentPort),
		Token:  "test-token",
		Services: []config.Service{
			{Name: "echo", Target: target},
		},
	}

	srvDone := make(chan error, 1)
	go func() { srvDone <- server.New(srvCfg, log).Run(ctx) }()

	agDone := make(chan error, 1)
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for {
			err := agent.New(agCfg, log).Run(ctx)
			if err == nil || time.Now().After(deadline) {
				agDone <- err
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()

	deadline := time.Now().Add(5 * time.Second)
	var got []byte
	for {
		conn := dialRetry(t, fmt.Sprintf("127.0.0.1:%d", publicPort), 5*time.Second)
		msg := []byte("hello through the tunnel")
		if _, err := conn.Write(msg); err == nil {
			if tc, ok := conn.(*net.TCPConn); ok {
				_ = tc.CloseWrite()
			}
			got, _ = io.ReadAll(conn)
		}
		_ = conn.Close()
		if bytes.Equal(got, []byte("echo:hello through the tunnel")) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("round trip = %q, want %q", got, "echo:hello through the tunnel")
		}
		time.Sleep(100 * time.Millisecond)
	}

	cancel()
	for _, ch := range []chan error{srvDone, agDone} {
		select {
		case <-ch:
		case <-time.After(5 * time.Second):
			t.Fatal("shutdown timed out")
		}
	}
}

func TestBadTokenRejected(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	agentPort := freePort(t)
	srvCfg := &config.Server{
		AgentListen: fmt.Sprintf("127.0.0.1:%d", agentPort),
		Token:       "right-token",
	}
	go func() { _ = server.New(srvCfg, log).Run(ctx) }()

	agCfg := &config.Agent{
		Name:   "intruder",
		Server: fmt.Sprintf("127.0.0.1:%d", agentPort),
		Token:  "wrong-token",
		Services: []config.Service{
			{Name: "echo", Target: "127.0.0.1:1"},
		},
	}

	deadline := time.Now().Add(5 * time.Second)
	for {
		err := agent.New(agCfg, log).Run(ctx)
		if err != nil && !isConnRefused(err) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("agent with bad token was not rejected (last err: %v)", err)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func isConnRefused(err error) bool {
	var opErr *net.OpError
	return errors.As(err, &opErr) && opErr.Op == "dial"
}

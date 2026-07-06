package tunnel

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"
)

// pipeTunnels returns a connected client/server pair over an in-memory
// pipe, torn down automatically when the test finishes.
func pipeTunnels(t *testing.T) (client, server *Yamux) {
	t.Helper()
	c1, c2 := net.Pipe()
	client, err := NewYamuxClient(c1)
	if err != nil {
		t.Fatalf("NewYamuxClient: %v", err)
	}
	server, err = NewYamuxServer(c2)
	if err != nil {
		t.Fatalf("NewYamuxServer: %v", err)
	}
	t.Cleanup(func() {
		_ = client.Close()
		_ = server.Close()
	})
	return client, server
}

func TestYamuxStreamRoundTrip(t *testing.T) {
	client, server := pipeTunnels(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Accept concurrently: it cannot return until the open side's SYN
	// frame crosses the pipe.
	type accept struct {
		conn net.Conn
		err  error
	}
	accepted := make(chan accept, 1)
	go func() {
		c, err := server.AcceptStream(ctx)
		accepted <- accept{c, err}
	}()

	out, err := client.OpenStream(ctx)
	if err != nil {
		t.Fatalf("OpenStream: %v", err)
	}

	msg := []byte("hello from the mole")
	if _, err := out.Write(msg); err != nil {
		t.Fatalf("Write: %v", err)
	}

	in := <-accepted
	if in.err != nil {
		t.Fatalf("AcceptStream: %v", in.err)
	}
	got := make([]byte, len(msg))
	if _, err := io.ReadFull(in.conn, got); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if !bytes.Equal(got, msg) {
		t.Fatalf("round trip = %q, want %q", got, msg)
	}
}

func TestYamuxAcceptHonorsContext(t *testing.T) {
	_, server := pipeTunnels(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := server.AcceptStream(ctx)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("AcceptStream with expired ctx = %v, want DeadlineExceeded", err)
	}
}

func TestYamuxClosedSemantics(t *testing.T) {
	client, _ := pipeTunnels(t)
	ctx := context.Background()

	if err := client.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := client.Close(); err != nil {
		t.Fatalf("second Close: %v, want nil (idempotent)", err)
	}
	if _, err := client.OpenStream(ctx); !errors.Is(err, ErrClosed) {
		t.Fatalf("OpenStream after Close = %v, want ErrClosed", err)
	}
	if _, err := client.AcceptStream(ctx); !errors.Is(err, ErrClosed) {
		t.Fatalf("AcceptStream after Close = %v, want ErrClosed", err)
	}
}

func TestYamuxDatagramsUnsupported(t *testing.T) {
	client, _ := pipeTunnels(t)
	ctx := context.Background()

	if err := client.SendDatagram(ctx, []byte("x")); !errors.Is(err, ErrDatagramsUnsupported) {
		t.Fatalf("SendDatagram = %v, want ErrDatagramsUnsupported", err)
	}
	if _, err := client.RecvDatagram(ctx); !errors.Is(err, ErrDatagramsUnsupported) {
		t.Fatalf("RecvDatagram = %v, want ErrDatagramsUnsupported", err)
	}
}

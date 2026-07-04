// Package proto carries krtica's control-plane messages: protobuf types
// generated into pb (Decision #18) plus the length-prefixed framing that
// delimits them on a byte stream.
package proto

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

// ProtocolVersion is carried in Hello and bumped on any breaking change
// to the control messages or framing.
const ProtocolVersion = 1

// MaxFrameSize bounds a single control frame. Control messages are tiny;
// anything larger indicates a corrupt or hostile peer (P1: no unbounded
// allocation driven by remote input).
const MaxFrameSize = 1 << 20

// ErrFrameTooLarge is returned when a peer announces a frame above
// MaxFrameSize.
var ErrFrameTooLarge = errors.New("proto: frame exceeds maximum size")

// WriteFrame marshals msg and writes it as one length-prefixed frame
// (4-byte big-endian length, then the protobuf bytes).
func WriteFrame(w io.Writer, msg proto.Message) error {
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("proto: marshal: %w", err)
	}
	if len(b) > MaxFrameSize {
		return ErrFrameTooLarge
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("proto: write frame header: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("proto: write frame body: %w", err)
	}
	return nil
}

// ReadFrame reads one length-prefixed frame from r and unmarshals it into
// msg, rejecting frames above MaxFrameSize before allocating.
func ReadFrame(r io.Reader, msg proto.Message) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return fmt.Errorf("proto: read frame header: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxFrameSize {
		return ErrFrameTooLarge
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return fmt.Errorf("proto: read frame body: %w", err)
	}
	if err := proto.Unmarshal(b, msg); err != nil {
		return fmt.Errorf("proto: unmarshal: %w", err)
	}
	return nil
}

package wire

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"

	"google.golang.org/protobuf/proto"
)

const ProtocolVersion = 1

const MaxFrameSize = 1 << 20

var ErrFrameTooLarge = errors.New("wire: frame exceeds maximum size")

func WriteFrame(w io.Writer, msg proto.Message) error {
	b, err := proto.Marshal(msg)
	if err != nil {
		return fmt.Errorf("wire: marshal: %w", err)
	}
	if len(b) > MaxFrameSize {
		return ErrFrameTooLarge
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(hdr[:]); err != nil {
		return fmt.Errorf("wire: write frame header: %w", err)
	}
	if _, err := w.Write(b); err != nil {
		return fmt.Errorf("wire: write frame body: %w", err)
	}
	return nil
}

func ReadFrame(r io.Reader, msg proto.Message) error {
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		return fmt.Errorf("wire: read frame header: %w", err)
	}
	n := binary.BigEndian.Uint32(hdr[:])
	if n > MaxFrameSize {
		return ErrFrameTooLarge
	}
	b := make([]byte, n)
	if _, err := io.ReadFull(r, b); err != nil {
		return fmt.Errorf("wire: read frame body: %w", err)
	}
	if err := proto.Unmarshal(b, msg); err != nil {
		return fmt.Errorf("wire: unmarshal: %w", err)
	}
	return nil
}

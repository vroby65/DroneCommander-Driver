package tello

import (
	"bytes"
	"errors"
	"testing"
)

type packetSequence struct {
	packets [][]byte
}

func (s *packetSequence) Read(destination []byte) (int, error) {
	if len(s.packets) == 0 {
		return 0, errors.New("no more packets")
	}
	packet := s.packets[0]
	s.packets = s.packets[1:]
	return copy(destination, packet), nil
}

func TestVideoFrameReaderReassemblesTelloPackets(t *testing.T) {
	first := bytes.Repeat([]byte{1}, telloVideoPacketSize)
	second := bytes.Repeat([]byte{2}, 317)
	reader := &videoFrameReader{source: &packetSequence{packets: [][]byte{first, second}}}
	frame := make([]byte, 4096)

	n, err := reader.Read(frame)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(first)+len(second) {
		t.Fatalf("frame size = %d, want %d", n, len(first)+len(second))
	}
	if !bytes.Equal(frame[:n], append(first, second...)) {
		t.Fatal("reassembled frame differs from UDP packets")
	}
}

func TestVideoFrameReaderRejectsOversizedFrameAtItsBoundary(t *testing.T) {
	reader := &videoFrameReader{source: &packetSequence{packets: [][]byte{
		bytes.Repeat([]byte{1}, telloVideoPacketSize),
		bytes.Repeat([]byte{2}, 100),
		bytes.Repeat([]byte{3}, 20),
	}}}
	frame := make([]byte, 1500)

	if _, err := reader.Read(frame); !errors.Is(err, errVideoFrameTooLarge) {
		t.Fatalf("oversized frame error = %v", err)
	}
	next := make([]byte, 100)
	n, err := reader.Read(next)
	if err != nil {
		t.Fatal(err)
	}
	if n != 20 || next[0] != 3 {
		t.Fatalf("reader did not resume at next frame: n=%d first=%d", n, next[0])
	}
}

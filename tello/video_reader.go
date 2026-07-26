package tello

import (
	"errors"
	"io"
)

const telloVideoPacketSize = 1460

var errVideoFrameTooLarge = errors.New("fotogramma video Tello troppo grande")

// videoFrameReader reassembles the UDP chunks used by the Tello into complete
// H.264 access units. A datagram shorter than 1460 bytes terminates a frame.
// Keeping that boundary is important for decoders that do not accept partial
// frames.
type videoFrameReader struct {
	source io.Reader
}

func (r *videoFrameReader) Read(destination []byte) (int, error) {
	packet := make([]byte, 2048)
	frameSize := 0
	overflow := false

	for {
		n, err := r.source.Read(packet)
		if err != nil {
			return 0, err
		}
		if n == 0 {
			continue
		}
		if frameSize+n > len(destination) {
			overflow = true
		} else if !overflow {
			copy(destination[frameSize:], packet[:n])
			frameSize += n
		}
		if n == telloVideoPacketSize {
			continue
		}
		if overflow {
			return 0, errVideoFrameTooLarge
		}
		return frameSize, nil
	}
}

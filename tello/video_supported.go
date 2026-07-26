//go:build !android && !ios && ((linux && (amd64 || arm || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

package tello

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"io"
	"net"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"

	openh264 "github.com/Azunyan1111/openh264-go"
)

const maxJPEGFrameSize = 8 << 20

var errJPEGFrameTooLarge = errors.New("fotogramma JPEG FFmpeg troppo grande")

// VideoReceiver receives the Tello H.264 stream and exposes decoded frames.
// FFmpeg is preferred when installed; OpenH264 remains available as an
// embedded fallback on supported desktop platforms.
type VideoReceiver struct {
	conn      *net.UDPConn
	decoder   *openh264.Decoder
	onFrame   func(image.Image)
	done      chan struct{}
	closeOnce sync.Once
	closed    atomic.Bool

	ffmpeg       *exec.Cmd
	ffmpegCancel context.CancelFunc
	ffmpegInput  io.WriteCloser
	ffmpegLog    bytes.Buffer

	errMu sync.Mutex
	err   error
}

// StartVideoReceiver binds the UDP socket before streamon is sent to the
// drone. This prevents losing the first SPS/PPS packets needed by the decoder.
func StartVideoReceiver(address string, onFrame func(image.Image)) (*VideoReceiver, error) {
	if address == "" {
		address = ":11111"
	}
	local, err := net.ResolveUDPAddr("udp4", address)
	if err != nil {
		return nil, fmt.Errorf("indirizzo video non valido: %w", err)
	}
	conn, err := net.ListenUDP("udp4", local)
	if err != nil {
		return nil, fmt.Errorf("porta video %s non disponibile: %w", address, err)
	}
	// A larger kernel buffer reduces corruption when the UI is briefly busy.
	_ = conn.SetReadBuffer(4 << 20)

	if onFrame == nil {
		onFrame = func(image.Image) {}
	}
	receiver := &VideoReceiver{
		conn: conn, onFrame: onFrame, done: make(chan struct{}),
	}
	// FFmpeg is the most tolerant decoder for the real Tello stream, which can
	// contain incomplete frames after a lost UDP datagram. Keep the embedded
	// OpenH264 path as a dependency-free fallback.
	if err := receiver.startFFmpeg(); err == nil {
		return receiver, nil
	}

	decoder, err := openh264.NewDecoder(&videoFrameReader{source: conn})
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("inizializzazione decoder H.264: %w", err)
	}
	receiver.decoder = decoder
	go receiver.readFrames()
	return receiver, nil
}

func (r *VideoReceiver) startFFmpeg() error {
	path, err := exec.LookPath("ffmpeg")
	if err != nil {
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, path,
		"-hide_banner",
		"-loglevel", "error",
		"-probesize", "1048576",
		"-analyzeduration", "1000000",
		"-fflags", "discardcorrupt",
		"-err_detect", "ignore_err",
		"-flags", "low_delay",
		"-f", "h264",
		"-framerate", "30",
		"-i", "pipe:0",
		"-an",
		"-f", "image2pipe",
		"-c:v", "mjpeg",
		"-q:v", "5",
		"-flush_packets", "1",
		"pipe:1",
	)
	input, err := command.StdinPipe()
	if err != nil {
		cancel()
		return err
	}
	output, err := command.StdoutPipe()
	if err != nil {
		_ = input.Close()
		cancel()
		return err
	}
	command.Stderr = &r.ffmpegLog
	if err := command.Start(); err != nil {
		_ = input.Close()
		cancel()
		return err
	}

	r.ffmpeg = command
	r.ffmpegCancel = cancel
	r.ffmpegInput = input
	go r.feedFFmpeg(input)
	go r.readFFmpegFrames(output)
	go r.waitFFmpeg()
	return nil
}

func (r *VideoReceiver) feedFFmpeg(input io.WriteCloser) {
	packet := make([]byte, 64*1024)
	for {
		n, _, err := r.conn.ReadFromUDP(packet)
		if err != nil {
			_ = input.Close()
			if !r.closed.Load() && r.ffmpegCancel != nil {
				r.setError(fmt.Errorf("ricezione UDP video: %w", err))
				r.ffmpegCancel()
			}
			return
		}
		if n == 0 {
			continue
		}
		if _, err := input.Write(packet[:n]); err != nil {
			_ = input.Close()
			if !r.closed.Load() && r.ffmpegCancel != nil {
				r.setError(fmt.Errorf("inoltro video a FFmpeg: %w", err))
				r.ffmpegCancel()
			}
			return
		}
	}
}

func (r *VideoReceiver) readFFmpegFrames(output io.Reader) {
	reader := bufio.NewReaderSize(output, 1024*1024)
	for {
		frame, err := decodeNextJPEGFrame(reader)
		if err != nil {
			if r.closed.Load() {
				return
			}
			if errors.Is(err, io.EOF) {
				if r.ffmpegCancel != nil {
					r.setError(fmt.Errorf("uscita video FFmpeg terminata: %w", err))
					r.ffmpegCancel()
				}
				return
			}
			// A damaged JPEG must not terminate the whole camera stream. The
			// next call scans for a fresh SOI marker and resumes at the next
			// complete frame.
			continue
		}
		r.onFrame(frame)
	}
}

func decodeNextJPEGFrame(reader *bufio.Reader) (image.Image, error) {
	encoded, err := readNextJPEGFrame(reader)
	if err != nil {
		return nil, err
	}
	return jpeg.Decode(bytes.NewReader(encoded))
}

func readNextJPEGFrame(reader *bufio.Reader) ([]byte, error) {
	previous := byte(0)
	for {
		current, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		if previous == 0xff && current == 0xd8 {
			break
		}
		previous = current
	}

	frame := make([]byte, 0, 64*1024)
	frame = append(frame, 0xff, 0xd8)
	previous = 0
	for len(frame) < maxJPEGFrameSize {
		current, err := reader.ReadByte()
		if err != nil {
			return nil, err
		}
		frame = append(frame, current)
		if previous == 0xff && current == 0xd9 {
			return frame, nil
		}
		previous = current
	}
	return nil, errJPEGFrameTooLarge
}

func (r *VideoReceiver) waitFFmpeg() {
	err := r.ffmpeg.Wait()
	if !r.closed.Load() {
		detail := strings.TrimSpace(r.ffmpegLog.String())
		switch {
		case detail != "":
			r.setError(fmt.Errorf("decoder video FFmpeg: %s", detail))
		case err != nil:
			r.setError(fmt.Errorf("decoder video FFmpeg: %w", err))
		default:
			r.setError(errors.New("decoder video FFmpeg terminato"))
		}
	}
	_ = r.conn.Close()
	close(r.done)
}

func (r *VideoReceiver) readFrames() {
	defer close(r.done)
	defer r.decoder.Close()

	for {
		frame, err := r.decoder.Read()
		if err != nil {
			if r.closed.Load() {
				return
			}
			var networkError *net.OpError
			if errors.As(err, &networkError) {
				r.setError(fmt.Errorf("ricezione video Tello: %w", err))
				return
			}
			// A lost UDP packet can invalidate one H.264 frame. Keep the
			// decoder alive so it can recover at the next complete key frame.
			continue
		}
		if frame != nil {
			r.onFrame(frame)
		}
	}
}

func (r *VideoReceiver) setError(err error) {
	r.errMu.Lock()
	if r.err == nil {
		r.err = err
	}
	r.errMu.Unlock()
}

// Done is closed when reception terminates.
func (r *VideoReceiver) Done() <-chan struct{} { return r.done }

// Err reports an unexpected receiver failure. A normal Close has no error.
func (r *VideoReceiver) Err() error {
	r.errMu.Lock()
	defer r.errMu.Unlock()
	return r.err
}

// Close stops the UDP reader and releases the active H.264 decoder.
func (r *VideoReceiver) Close() error {
	r.closeOnce.Do(func() {
		r.closed.Store(true)
		_ = r.conn.Close()
		if r.ffmpegInput != nil {
			_ = r.ffmpegInput.Close()
		}
		if r.ffmpegCancel != nil {
			r.ffmpegCancel()
		}
	})
	<-r.done
	return nil
}

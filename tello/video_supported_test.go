//go:build !android && !ios && ((linux && (amd64 || arm || arm64)) || (darwin && (amd64 || arm64)) || (windows && amd64))

package tello

import (
	"bufio"
	"bytes"
	"image"
	"image/jpeg"
	"net"
	"os/exec"
	"testing"
	"time"

	openh264 "github.com/Azunyan1111/openh264-go"
)

func TestFFmpegJPEGReaderResynchronizesAfterJunkAndCorruptFrame(t *testing.T) {
	encode := func(width, height int) []byte {
		t.Helper()
		var output bytes.Buffer
		if err := jpeg.Encode(&output, image.NewRGBA(image.Rect(0, 0, width, height)), nil); err != nil {
			t.Fatal(err)
		}
		return output.Bytes()
	}

	var stream bytes.Buffer
	stream.Write([]byte("junk before frame"))
	stream.Write(encode(16, 12))
	stream.Write([]byte{0x00, 0x01, 0x02})
	stream.Write([]byte{0xff, 0xd8, 0x01, 0x02, 0xff, 0xd9})
	stream.Write([]byte("padding"))
	stream.Write(encode(32, 24))
	reader := bufio.NewReader(&stream)

	first, err := decodeNextJPEGFrame(reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := first.Bounds().Size(); got.X != 16 || got.Y != 12 {
		t.Fatalf("first JPEG size = %dx%d, want 16x12", got.X, got.Y)
	}
	if _, err := decodeNextJPEGFrame(reader); err == nil {
		t.Fatal("corrupt JPEG was unexpectedly accepted")
	}
	second, err := decodeNextJPEGFrame(reader)
	if err != nil {
		t.Fatal(err)
	}
	if got := second.Bounds().Size(); got.X != 32 || got.Y != 24 {
		t.Fatalf("second JPEG size = %dx%d, want 32x24", got.X, got.Y)
	}
}

func TestEmbeddedVideoReceiverDecodesPacketizedH264Frame(t *testing.T) {
	// Exercise the embedded fallback independently of software installed on
	// the build machine.
	t.Setenv("PATH", t.TempDir())

	frames := make(chan image.Image, 1)
	receiver, err := StartVideoReceiver("127.0.0.1:0", func(frame image.Image) {
		select {
		case frames <- frame:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()

	params := openh264.NewEncoderParams()
	params.Width = 160
	params.Height = 120
	params.IntraPeriod = 1
	encoder, err := openh264.NewEncoder(params)
	if err != nil {
		t.Fatal(err)
	}
	defer encoder.Close()
	source := image.NewYCbCr(image.Rect(0, 0, params.Width, params.Height), image.YCbCrSubsampleRatio420)
	for index := range source.Y {
		source.Y[index] = 96
	}
	for index := range source.Cb {
		source.Cb[index] = 128
		source.Cr[index] = 128
	}
	// Send several access units: production decoders are allowed to buffer a
	// small amount of input while discovering the stream parameters.
	for frameIndex := 0; frameIndex < 8; frameIndex++ {
		encoded, err := encoder.Encode(source)
		if err != nil {
			t.Fatal(err)
		}
		sendVideoBytes(t, receiver, encoded)
	}

	assertVideoFrame(t, frames, params.Width, params.Height)
}

func TestFFmpegVideoReceiverDecodesLiveH264Stream(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		t.Skip("FFmpeg not installed")
	}
	command := exec.Command(ffmpeg,
		"-hide_banner", "-loglevel", "error",
		"-f", "lavfi", "-i", "color=c=gray:size=160x120:rate=30",
		"-frames:v", "60",
		"-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
		"-f", "h264", "pipe:1",
	)
	encoded, err := command.Output()
	if err != nil {
		t.Skipf("FFmpeg H.264 encoder unavailable: %v", err)
	}

	frames := make(chan image.Image, 1)
	receiver, err := StartVideoReceiver("127.0.0.1:0", func(frame image.Image) {
		select {
		case frames <- frame:
		default:
		}
	})
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Close()

	sendVideoBytes(t, receiver, encoded)
	assertVideoFrame(t, frames, 160, 120)
}

func sendVideoBytes(t *testing.T, receiver *VideoReceiver, encoded []byte) {
	t.Helper()
	conn, err := net.DialUDP("udp4", nil, receiver.conn.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	for len(encoded) > 0 {
		size := min(len(encoded), telloVideoPacketSize)
		if _, err := conn.Write(encoded[:size]); err != nil {
			t.Fatal(err)
		}
		encoded = encoded[size:]
	}
}

func assertVideoFrame(t *testing.T, frames <-chan image.Image, width, height int) {
	t.Helper()
	select {
	case frame := <-frames:
		if got := frame.Bounds().Size(); got.X != width || got.Y != height {
			t.Fatalf("decoded frame = %dx%d, want %dx%d", got.X, got.Y, width, height)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a decoded video frame")
	}
}

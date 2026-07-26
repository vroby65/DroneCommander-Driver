package session

import (
	"image"
	"image/color"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestFFmpegRecordingProducesMP4(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg is not installed")
	}
	recording, err := newFFmpegRecording(t.TempDir(), time.Date(2026, 7, 26, 12, 34, 56, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 12; index++ {
		frame := image.NewRGBA(image.Rect(0, 0, 64, 48))
		frame.Set(0, 0, color.RGBA{R: uint8(index * 20), G: 80, B: 160, A: 255})
		recording.AddFrame(frame)
	}
	path, err := recording.Save()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(path) != ".mp4" {
		t.Fatalf("recording path = %q", path)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() == 0 {
		t.Fatal("saved MP4 is empty")
	}
}

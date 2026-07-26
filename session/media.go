package session

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	cameraFrameWait    = 3 * time.Second
	recordingFrameRate = 30
)

func (s *Session) takePhoto(ctx context.Context) (string, error) {
	frame, err := s.mediaFrame(ctx)
	if err != nil {
		return "", err
	}
	s.mu.Lock()
	directory := s.options.MediaDirectory
	s.mu.Unlock()
	if directory == "" {
		return "", errors.New("cartella multimediale non configurata")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", fmt.Errorf("creazione cartella multimediale: %w", err)
	}

	finalPath := availableMediaPath(directory, "drone-photo", ".png", time.Now())
	temp, err := os.CreateTemp(directory, ".drone-photo-*.tmp")
	if err != nil {
		return "", fmt.Errorf("creazione foto temporanea: %w", err)
	}
	tempPath := temp.Name()
	defer func() { _ = os.Remove(tempPath) }()

	if err := png.Encode(temp, frame); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("codifica foto PNG: %w", err)
	}
	if err := temp.Chmod(0o644); err != nil {
		_ = temp.Close()
		return "", fmt.Errorf("permessi foto: %w", err)
	}
	if err := temp.Close(); err != nil {
		return "", fmt.Errorf("chiusura foto: %w", err)
	}
	if err := os.Rename(tempPath, finalPath); err != nil {
		return "", fmt.Errorf("salvataggio foto: %w", err)
	}
	return finalPath, nil
}

func (s *Session) startRecording(ctx context.Context) error {
	frame, err := s.mediaFrame(ctx)
	if err != nil {
		return err
	}

	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return errors.New("la registrazione può essere avviata solo da un programma in esecuzione")
	}
	if s.recording != nil {
		s.mu.Unlock()
		return errors.New("una registrazione è già in corso")
	}
	runID := s.runID
	factory := s.recordingFactory
	directory := s.options.MediaDirectory
	s.mu.Unlock()

	if directory == "" {
		return errors.New("cartella multimediale non configurata")
	}
	recording, err := factory(directory, time.Now())
	if err != nil {
		return err
	}
	// Seed the encoder before publishing it to the camera callback. This keeps
	// the first frame ordered before all subsequently received frames.
	recording.AddFrame(frame)

	s.mu.Lock()
	if !s.running || s.runID != runID {
		s.mu.Unlock()
		_ = recording.Cancel()
		return context.Canceled
	}
	if !s.cameraEnabled || s.cameraFrame == nil {
		s.mu.Unlock()
		_ = recording.Cancel()
		return errors.New("lo stream della telecamera non è più disponibile")
	}
	if s.recording != nil {
		s.mu.Unlock()
		_ = recording.Cancel()
		return errors.New("una registrazione è già in corso")
	}
	s.recording = recording
	s.recordingRunID = runID
	s.mu.Unlock()
	return nil
}

func (s *Session) saveRecording(ctx context.Context) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	s.mu.Lock()
	recording := s.recording
	if recording == nil {
		s.mu.Unlock()
		return "", errors.New("nessuna registrazione in corso")
	}
	s.recording = nil
	s.recordingRunID = 0
	s.mu.Unlock()
	return recording.Save()
}

func (s *Session) mediaFrame(ctx context.Context) (image.Image, error) {
	deadline := time.NewTimer(cameraFrameWait)
	defer deadline.Stop()
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()

	for {
		s.mu.Lock()
		connected := s.connected
		simulated := s.simulated
		enabled := s.cameraEnabled
		frame := s.cameraFrame
		s.mu.Unlock()

		switch {
		case !connected:
			return nil, errors.New("Tello non connesso")
		case simulated:
			return nil, errors.New("la telecamera non è disponibile in simulazione")
		case !enabled:
			return nil, errors.New("attiva manualmente la telecamera prima di avviare il programma")
		case frame != nil:
			return frame, nil
		}

		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-deadline.C:
			return nil, errors.New("nessun fotogramma ricevuto dalla telecamera")
		case <-ticker.C:
		}
	}
}

func (s *Session) detachRecordingLocked() videoRecording {
	recording := s.recording
	s.recording = nil
	s.recordingRunID = 0
	return recording
}

func (s *Session) discardRunRecording(runID uint64) {
	s.mu.Lock()
	var recording videoRecording
	if s.recording != nil && s.recordingRunID == runID {
		recording = s.detachRecordingLocked()
	}
	s.mu.Unlock()
	if recording == nil {
		return
	}
	_ = recording.Cancel()
	s.addLog("Registrazione incompleta scartata: manca il blocco Salva registrazione.")
}

func cancelDetachedRecording(recording videoRecording) {
	if recording != nil {
		_ = recording.Cancel()
	}
}

func availableMediaPath(directory, prefix, extension string, timestamp time.Time) string {
	base := prefix + "-" + timestamp.Format("20060102-150405.000")
	candidate := filepath.Join(directory, base+extension)
	for suffix := 2; ; suffix++ {
		if _, err := os.Stat(candidate); err != nil {
			// Unknown filesystem errors are reported later by Rename. Returning
			// here also avoids an unbounded loop on an inaccessible directory.
			return candidate
		}
		candidate = filepath.Join(directory, fmt.Sprintf("%s-%d%s", base, suffix, extension))
	}
}

type ffmpegRecording struct {
	mu        sync.Mutex
	frames    chan image.Image
	closed    bool
	cancel    context.CancelFunc
	done      chan error
	tempPath  string
	finalPath string

	finishOnce sync.Once
	finishErr  error
	saved      bool
}

func newFFmpegRecording(directory string, timestamp time.Time) (videoRecording, error) {
	ffmpegPath, err := exec.LookPath("ffmpeg")
	if err != nil {
		return nil, errors.New("FFmpeg non trovato: installalo per registrare i video")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return nil, fmt.Errorf("creazione cartella multimediale: %w", err)
	}

	temp, err := os.CreateTemp(directory, ".drone-video-*.tmp.mp4")
	if err != nil {
		return nil, fmt.Errorf("creazione video temporaneo: %w", err)
	}
	tempPath := temp.Name()
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("chiusura video temporaneo: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	command := exec.CommandContext(ctx, ffmpegPath,
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "image2pipe",
		"-framerate", fmt.Sprint(recordingFrameRate),
		"-vcodec", "mjpeg",
		"-i", "pipe:0",
		"-an",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:v", "mpeg4",
		"-q:v", "4",
		"-pix_fmt", "yuv420p",
		"-movflags", "+faststart",
		tempPath,
	)
	input, err := command.StdinPipe()
	if err != nil {
		cancel()
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("ingresso FFmpeg: %w", err)
	}
	var ffmpegLog bytes.Buffer
	command.Stderr = &ffmpegLog
	if err := command.Start(); err != nil {
		_ = input.Close()
		cancel()
		_ = os.Remove(tempPath)
		return nil, fmt.Errorf("avvio FFmpeg: %w", err)
	}

	recording := &ffmpegRecording{
		frames:    make(chan image.Image, 3),
		cancel:    cancel,
		done:      make(chan error, 1),
		tempPath:  tempPath,
		finalPath: availableMediaPath(directory, "drone-video", ".mp4", timestamp),
	}
	go recording.encode(command, input, &ffmpegLog)
	return recording, nil
}

func (r *ffmpegRecording) AddFrame(frame image.Image) {
	if frame == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		return
	}
	// The preview must never stall because the encoder is slower than the live
	// stream. Dropping an occasional queued frame is preferable to blocking the
	// camera decoder.
	select {
	case r.frames <- frame:
	default:
	}
}

func (r *ffmpegRecording) Save() (string, error) {
	r.finish(true)
	if !r.saved {
		if r.finishErr != nil {
			return "", r.finishErr
		}
		return "", errors.New("la registrazione è stata annullata")
	}
	return r.finalPath, r.finishErr
}

func (r *ffmpegRecording) Cancel() error {
	r.finish(false)
	return r.finishErr
}

func (r *ffmpegRecording) finish(save bool) {
	r.finishOnce.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.frames)
		if !save {
			r.cancel()
		}
		r.mu.Unlock()

		if save {
			timer := time.NewTimer(10 * time.Second)
			select {
			case r.finishErr = <-r.done:
				timer.Stop()
			case <-timer.C:
				r.cancel()
				<-r.done
				r.finishErr = errors.New("FFmpeg non ha terminato il video entro 10 secondi")
			}
		} else {
			<-r.done
		}

		if save && r.finishErr == nil {
			if err := os.Rename(r.tempPath, r.finalPath); err != nil {
				r.finishErr = fmt.Errorf("salvataggio registrazione: %w", err)
			} else {
				r.saved = true
			}
		}
		if !r.saved {
			if err := os.Remove(r.tempPath); err != nil && !errors.Is(err, os.ErrNotExist) && r.finishErr == nil {
				r.finishErr = fmt.Errorf("rimozione registrazione temporanea: %w", err)
			}
		}
	})
}

func (r *ffmpegRecording) encode(command *exec.Cmd, input io.WriteCloser, ffmpegLog *bytes.Buffer) {
	var encodeErr error
	for frame := range r.frames {
		if err := jpeg.Encode(input, frame, &jpeg.Options{Quality: 82}); err != nil {
			encodeErr = fmt.Errorf("invio fotogramma a FFmpeg: %w", err)
			r.cancel()
			break
		}
	}
	_ = input.Close()
	waitErr := command.Wait()

	switch {
	case encodeErr != nil:
		r.done <- encodeErr
	case waitErr != nil:
		detail := strings.TrimSpace(ffmpegLog.String())
		if detail != "" {
			r.done <- fmt.Errorf("registrazione FFmpeg: %s", detail)
		} else {
			r.done <- fmt.Errorf("registrazione FFmpeg: %w", waitErr)
		}
	default:
		r.done <- nil
	}
}

package session

import (
	"context"
	"errors"
	"image"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vroby65/DroneCommander-Driver/tello"
)

type landingCommander struct {
	commands []string
}

type fakeCameraStream struct {
	done   chan struct{}
	closed bool
	err    error
}

type fakeVideoRecording struct {
	mu       sync.Mutex
	frames   int
	saved    bool
	canceled bool
	path     string
}

func (r *fakeVideoRecording) AddFrame(image.Image) {
	r.mu.Lock()
	r.frames++
	r.mu.Unlock()
}
func (r *fakeVideoRecording) Save() (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.saved = true
	return r.path, nil
}
func (r *fakeVideoRecording) Cancel() error {
	r.mu.Lock()
	r.canceled = true
	r.mu.Unlock()
	return nil
}

func (s *fakeCameraStream) Done() <-chan struct{} { return s.done }
func (s *fakeCameraStream) Err() error            { return s.err }
func (s *fakeCameraStream) Close() error {
	if !s.closed {
		s.closed = true
		close(s.done)
	}
	return nil
}

type changingBatteryCommander struct {
	mu        sync.Mutex
	batteries []int
	reads     int
	state     tello.Telemetry
}

func (c *changingBatteryCommander) Connect(context.Context) error { return nil }
func (c *changingBatteryCommander) Command(_ context.Context, command string) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	switch command {
	case "battery?":
		index := min(c.reads, len(c.batteries)-1)
		battery := c.batteries[index]
		c.reads++
		c.state.Battery = battery
		c.state.UpdatedAt = time.Now()
		return strconv.Itoa(battery), nil
	case "takeoff":
		c.state.Height = 80
	case "land", "emergency":
		c.state.Height = 0
	}
	return "ok", nil
}
func (c *changingBatteryCommander) Immediate(command string) error {
	_, err := c.Command(context.Background(), command)
	return err
}
func (c *changingBatteryCommander) Snapshot() tello.Telemetry {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state
}
func (c *changingBatteryCommander) Close() error { return nil }

func (c *landingCommander) Connect(context.Context) error { return nil }
func (c *landingCommander) Command(_ context.Context, command string) (string, error) {
	c.commands = append(c.commands, "confirmed:"+command)
	return "ok", nil
}
func (c *landingCommander) Immediate(command string) error {
	c.commands = append(c.commands, "immediate:"+command)
	return nil
}
func (c *landingCommander) Snapshot() tello.Telemetry { return tello.Telemetry{} }
func (c *landingCommander) Close() error              { return nil }

func TestSimulationWorkflowUsesCentimeters(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	xml := `<xml><block type="start_block"><next><block type="take_off"><next><block type="walk"><value name="DIST"><block type="math_number"><field name="NUM">20</field></block></value><next><block type="land"></block></next></block></next></block></next></block></xml>`
	if err := s.LoadProgram("indoor.xml", []byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := s.Connect(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(RunConfig{MinimumBattery: 20, AutoLand: true}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for s.Snapshot().Running {
		if time.Now().After(deadline) {
			t.Fatal("program did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	snapshot := s.Snapshot()
	if snapshot.LastError != "" {
		t.Fatal(snapshot.LastError)
	}
	var messages strings.Builder
	for _, entry := range snapshot.Logs {
		messages.WriteString(entry.Message)
		messages.WriteByte('\n')
	}
	if !strings.Contains(messages.String(), "→ forward 20") {
		t.Fatalf("logs do not contain centimeter command:\n%s", messages.String())
	}
}

func TestCollisionCheckActivationIsLogged(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	if err := s.LoadProgram("empty.xml", []byte(`<xml><block type="start_block"></block></xml>`)); err != nil {
		t.Fatal(err)
	}
	if err := s.Connect(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(RunConfig{CollisionCheck: true}); err != nil {
		t.Fatal(err)
	}
	waitUntilStopped(t, s)

	var messages strings.Builder
	for _, entry := range s.Snapshot().Logs {
		messages.WriteString(entry.Message)
		messages.WriteByte('\n')
	}
	if !strings.Contains(messages.String(), "Controllo collisioni attivato.") {
		t.Fatalf("collision check activation was not logged:\n%s", messages.String())
	}
}

func TestSubTwentyCentimeterMovementFails(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	xml := `<xml><block type="start_block"><next><block type="take_off"><next><block type="walk"><value name="DIST"><block type="math_number"><field name="NUM">1</field></block></value></block></next></block></next></block></xml>`
	if err := s.LoadProgram("too-small.xml", []byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := s.Connect(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(RunConfig{MinimumBattery: 20, AutoLand: false}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	for s.Snapshot().Running {
		if time.Now().After(deadline) {
			t.Fatal("program did not finish")
		}
		time.Sleep(time.Millisecond)
	}
	if !strings.Contains(s.Snapshot().LastError, "minimo Tello") {
		t.Fatalf("unexpected error: %q", s.Snapshot().LastError)
	}
	if height := s.Snapshot().Telemetry.Height; height != 0 {
		t.Fatalf("safety landing did not run after the error: height = %d", height)
	}
}

func TestManualLandIsImmediateAndConfirmed(t *testing.T) {
	s := New(Options{})
	device := &landingCommander{}
	s.connected = true
	s.device = device
	if err := s.Safety("land"); err != nil {
		t.Fatal(err)
	}
	want := "immediate:land|confirmed:land"
	if got := strings.Join(device.commands, "|"); got != want {
		t.Fatalf("landing commands = %q, want %q", got, want)
	}
}

func TestCameraSendsStreamCommandsAndPublishesFrames(t *testing.T) {
	s := New(Options{VideoAddress: "127.0.0.1:11111"})
	device := &landingCommander{}
	stream := &fakeCameraStream{done: make(chan struct{})}
	var factoryAddress string
	s.cameraFactory = func(address string, onFrame func(image.Image)) (cameraStream, error) {
		factoryAddress = address
		onFrame(image.NewRGBA(image.Rect(0, 0, 2, 2)))
		return stream, nil
	}
	s.connected = true
	s.device = device

	if err := s.SetCamera(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	snapshot := s.Snapshot()
	if !snapshot.CameraEnabled || !snapshot.CameraRequested || snapshot.CameraChanging {
		t.Fatalf("camera state after enable = enabled:%t requested:%t changing:%t", snapshot.CameraEnabled, snapshot.CameraRequested, snapshot.CameraChanging)
	}
	if snapshot.CameraFrame == nil || snapshot.CameraFrameID == 0 {
		t.Fatal("decoded camera frame was not published")
	}
	if factoryAddress != "127.0.0.1:11111" {
		t.Fatalf("video bind address = %q", factoryAddress)
	}

	if err := s.SetCamera(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	snapshot = s.Snapshot()
	if snapshot.CameraEnabled || snapshot.CameraRequested || snapshot.CameraChanging || snapshot.CameraFrame != nil {
		t.Fatalf("camera state after disable = %+v", snapshot)
	}
	if !stream.closed {
		t.Fatal("camera receiver was not closed")
	}
	if got := strings.Join(device.commands, "|"); got != "confirmed:streamon|confirmed:streamoff" {
		t.Fatalf("camera commands = %q", got)
	}
}

func TestCameraIsUnavailableInSimulation(t *testing.T) {
	s := New(Options{})
	if err := s.Connect(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.SetCamera(context.Background(), true); err == nil || !strings.Contains(err.Error(), "simulazione") {
		t.Fatalf("camera in simulation error = %v", err)
	}
}

func TestPhotoAndRecordingUseCameraFramesWithoutStreamCommands(t *testing.T) {
	mediaDirectory := t.TempDir()
	s := New(Options{MediaDirectory: mediaDirectory})
	device := &landingCommander{}
	frame := image.NewRGBA(image.Rect(0, 0, 16, 12))
	recording := &fakeVideoRecording{path: filepath.Join(mediaDirectory, "video.mp4")}
	s.recordingFactory = func(directory string, _ time.Time) (videoRecording, error) {
		if directory != mediaDirectory {
			t.Fatalf("recording directory = %q, want %q", directory, mediaDirectory)
		}
		return recording, nil
	}
	s.connected = true
	s.device = device
	s.running = true
	s.runID = 7
	s.cameraEnabled = true
	s.cameraRequested = true
	s.cameraGeneration = 3
	s.cameraFrame = frame

	photoPath, err := s.takePhoto(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Ext(photoPath) != ".png" {
		t.Fatalf("photo path = %q", photoPath)
	}
	if info, err := os.Stat(photoPath); err != nil || info.Size() == 0 {
		t.Fatalf("saved photo is invalid: info=%v err=%v", info, err)
	}

	if err := s.startRecording(context.Background()); err != nil {
		t.Fatal(err)
	}
	s.acceptCameraFrame(3, image.NewRGBA(image.Rect(0, 0, 16, 12)))
	videoPath, err := s.saveRecording(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if videoPath != recording.path {
		t.Fatalf("video path = %q, want %q", videoPath, recording.path)
	}
	recording.mu.Lock()
	frames, saved, canceled := recording.frames, recording.saved, recording.canceled
	recording.mu.Unlock()
	if frames != 2 || !saved || canceled {
		t.Fatalf("recording state = frames:%d saved:%t canceled:%t", frames, saved, canceled)
	}
	if len(device.commands) != 0 {
		t.Fatalf("media actions toggled the Tello stream: %#v", device.commands)
	}
}

func TestMediaDirectoryCanOnlyChangeBeforeProgramStart(t *testing.T) {
	first := t.TempDir()
	second := t.TempDir()
	s := New(Options{MediaDirectory: first})

	if err := s.SetMediaDirectory(second); err != nil {
		t.Fatal(err)
	}
	if got := s.Snapshot().MediaDirectory; got != second {
		t.Fatalf("media directory = %q, want %q", got, second)
	}

	s.running = true
	if err := s.SetMediaDirectory(first); err == nil || !strings.Contains(err.Error(), "prima di avviare") {
		t.Fatalf("unexpected running directory change error: %v", err)
	}
	if got := s.Snapshot().MediaDirectory; got != second {
		t.Fatalf("running directory change altered destination to %q", got)
	}
}

func TestRecordingRequiresManuallyEnabledCamera(t *testing.T) {
	s := New(Options{MediaDirectory: t.TempDir()})
	s.connected = true
	s.device = &landingCommander{}
	s.running = true
	s.runID = 1
	err := s.startRecording(context.Background())
	if err == nil || !strings.Contains(err.Error(), "attiva manualmente") {
		t.Fatalf("unexpected recording error: %v", err)
	}
	if len(s.device.(*landingCommander).commands) != 0 {
		t.Fatal("recording attempted to toggle the camera automatically")
	}
}

func TestUnfinishedRecordingIsCanceledAtProgramEnd(t *testing.T) {
	s := New(Options{})
	recording := &fakeVideoRecording{}
	s.recording = recording
	s.recordingRunID = 9

	s.discardRunRecording(9)

	recording.mu.Lock()
	canceled := recording.canceled
	recording.mu.Unlock()
	if !canceled || s.recording != nil {
		t.Fatalf("unfinished recording was not canceled: canceled=%t active=%v", canceled, s.recording)
	}
}

func TestCameraBlocksExecuteEndToEndThroughSession(t *testing.T) {
	mediaDirectory := t.TempDir()
	s := New(Options{MediaDirectory: mediaDirectory})
	xml := `<xml><block type="start_block"><next>
	  <block type="take_photo"><next>
	  <block type="start_recording"><next>
	  <block type="save_recording"></block>
	  </next></block></next></block>
	</next></block></xml>`
	if err := s.LoadProgram("media.xml", []byte(xml)); err != nil {
		t.Fatal(err)
	}

	device := &landingCommander{}
	recording := &fakeVideoRecording{path: filepath.Join(mediaDirectory, "saved.mp4")}
	s.recordingFactory = func(string, time.Time) (videoRecording, error) {
		return recording, nil
	}
	s.connected = true
	s.device = device
	s.cameraEnabled = true
	s.cameraRequested = true
	s.cameraFrame = image.NewRGBA(image.Rect(0, 0, 16, 12))

	if err := s.Start(RunConfig{}); err != nil {
		t.Fatal(err)
	}
	waitUntilStopped(t, s)
	snapshot := s.Snapshot()
	if snapshot.LastError != "" {
		t.Fatal(snapshot.LastError)
	}
	var messages strings.Builder
	for _, entry := range snapshot.Logs {
		messages.WriteString(entry.Message)
		messages.WriteByte('\n')
	}
	for _, expected := range []string{"Foto salvata:", "Registrazione video avviata.", "Registrazione video salvata:"} {
		if !strings.Contains(messages.String(), expected) {
			t.Fatalf("missing %q in media logs:\n%s", expected, messages.String())
		}
	}
	if joined := strings.Join(device.commands, "|"); strings.Contains(joined, "streamon") || strings.Contains(joined, "streamoff") {
		t.Fatalf("camera blocks toggled the stream: %q", joined)
	}
}

func TestCameraFailureNeverTogglesStreamAutomatically(t *testing.T) {
	s := New(Options{VideoAddress: "127.0.0.1:11111"})
	device := &landingCommander{}
	stream := &fakeCameraStream{done: make(chan struct{})}
	factoryCalls := 0
	s.cameraFactory = func(_ string, onFrame func(image.Image)) (cameraStream, error) {
		factoryCalls++
		onFrame(image.NewRGBA(image.Rect(0, 0, 2, 2)))
		return stream, nil
	}
	s.connected = true
	s.device = device

	if err := s.SetCamera(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	stream.err = errors.New("decoder interrotto")
	stream.closed = true
	close(stream.done)

	deadline := time.Now().Add(time.Second)
	for s.Snapshot().CameraError == "" {
		if time.Now().After(deadline) {
			t.Fatal("camera failure was not published")
		}
		time.Sleep(time.Millisecond)
	}
	if !s.Snapshot().CameraRequested {
		t.Fatal("camera checkbox state changed without a user action")
	}
	if factoryCalls != 1 {
		t.Fatalf("camera factory calls = %d, want exactly 1", factoryCalls)
	}
	if got := strings.Join(device.commands, "|"); got != "confirmed:streamon" {
		t.Fatalf("commands after decoder failure = %q, want no automatic toggle", got)
	}
	if err := s.SetCamera(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(device.commands, "|"); got != "confirmed:streamon|confirmed:streamoff" {
		t.Fatalf("commands after manual disable = %q", got)
	}
}

func TestBatteryIsReadBeforeAndAfterEveryFlight(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	if err := s.LoadProgram("battery.xml", []byte(`<xml><block type="start_block"><next><block type="take_off"><next><block type="land"></block></next></block></next></block></xml>`)); err != nil {
		t.Fatal(err)
	}
	device := &changingBatteryCommander{batteries: []int{94, 92, 89, 87}}
	s.mu.Lock()
	s.device = device
	s.connected = true
	s.mu.Unlock()

	for flight := 1; flight <= 2; flight++ {
		if err := s.Start(RunConfig{MinimumBattery: 20}); err != nil {
			t.Fatalf("flight %d: %v", flight, err)
		}
		waitUntilStopped(t, s)
		if err := s.Snapshot().LastError; err != "" {
			t.Fatalf("flight %d: %s", flight, err)
		}
	}

	device.mu.Lock()
	reads := device.reads
	device.mu.Unlock()
	if reads != 4 {
		t.Fatalf("battery reads = %d, want 4 (before and after each flight)", reads)
	}
	snapshot := s.Snapshot()
	if snapshot.Telemetry.Battery != 87 {
		t.Fatalf("displayed battery = %d%%, want latest value 87%%", snapshot.Telemetry.Battery)
	}
	var messages strings.Builder
	for _, entry := range snapshot.Logs {
		messages.WriteString(entry.Message)
		messages.WriteByte('\n')
	}
	if count := strings.Count(messages.String(), "Batteria dopo il volo:"); count != 2 {
		t.Fatalf("final battery log count = %d, want 2\n%s", count, messages.String())
	}
}

func TestFlightLogIsPersisted(t *testing.T) {
	path := filepath.Join(t.TempDir(), "logs", "flight.log")
	s := New(Options{LogPath: path})
	if err := s.LoadProgram("saved.xml", []byte(`<xml><block type="start_block"></block></xml>`)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Caricato saved.xml") {
		t.Fatalf("persistent log does not contain load entry:\n%s", data)
	}
	if err := s.ClearLogs(); err != nil {
		t.Fatal(err)
	}
	if got := len(s.Snapshot().Logs); got != 0 {
		t.Fatalf("in-memory log entries after clear = %d, want 0", got)
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 0 {
		t.Fatalf("persistent log after clear is not empty:\n%s", data)
	}
}

func TestRepeatedMovementsAndAngleChangesExecuteEndToEnd(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	xml := `<xml><block type="start_block"><next><block type="take_off"><next>
      <block type="controls_repeat_ext">
        <value name="TIMES"><block type="math_number"><field name="NUM">4</field></block></value>
        <statement name="DO"><block type="walk">
          <value name="DIST"><block type="math_number"><field name="NUM">30</field></block></value><next>
          <block type="change_angle"><value name="ANGLE"><block type="math_number"><field name="NUM">90</field></block></value></block>
        </next></block></statement><next><block type="land"></block></next>
      </block></next></block></next></block></xml>`
	if err := s.LoadProgram("four-turns.xml", []byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := s.Connect(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(RunConfig{MinimumBattery: 20}); err != nil {
		t.Fatal(err)
	}
	waitUntilStopped(t, s)
	snapshot := s.Snapshot()
	if snapshot.LastError != "" {
		t.Fatal(snapshot.LastError)
	}
	var messages strings.Builder
	for _, entry := range snapshot.Logs {
		messages.WriteString(entry.Message)
		messages.WriteByte('\n')
	}
	if count := strings.Count(messages.String(), "→ forward 30"); count != 4 {
		t.Fatalf("forward command count = %d, want 4\n%s", count, messages.String())
	}
	if count := strings.Count(messages.String(), "→ cw 90"); count != 4 {
		t.Fatalf("angle command count = %d, want 4\n%s", count, messages.String())
	}
	if snapshot.Telemetry.Yaw != 0 {
		t.Fatalf("yaw after a full turn = %d degrees, want 0", snapshot.Telemetry.Yaw)
	}
}

func TestLatestDroneCommandsExecuteFromXML(t *testing.T) {
	s := New(Options{})
	defer s.Close()
	xml := `<xml><block type="start_block"><next><block type="take_off"><next>
      <block type="set_speed"><value name="SPEED"><block type="math_number"><field name="NUM">6</field></block></value><next>
      <block type="curve">
        <value name="X"><block type="math_number"><field name="NUM">50</field></block></value>
        <value name="Y"><block type="math_number"><field name="NUM">0</field></block></value>
        <value name="Z"><block type="math_number"><field name="NUM">50</field></block></value>
        <value name="XD"><block type="math_number"><field name="NUM">50</field></block></value>
        <value name="YD"><block type="math_number"><field name="NUM">0</field></block></value>
        <value name="ZD"><block type="math_number"><field name="NUM">-50</field></block></value><next>
      <block type="curve_abs">
        <value name="X"><block type="math_number"><field name="NUM">100</field></block></value>
        <value name="Y"><block type="math_number"><field name="NUM">80</field></block></value>
        <value name="Z"><block type="math_number"><field name="NUM">100</field></block></value>
        <value name="XD"><block type="math_number"><field name="NUM">0</field></block></value>
        <value name="YD"><block type="math_number"><field name="NUM">80</field></block></value>
        <value name="ZD"><block type="math_number"><field name="NUM">100</field></block></value><next>
      <block type="smoke"><value name="SMOKE"><block type="logic_boolean"><field name="BOOL">TRUE</field></block></value><next>
      <block type="wait"><value name="DIST"><block type="math_number"><field name="NUM">0</field></block></value><next>
      <block type="return_to_base"><next><block type="land"></block></next></block>
      </next></block></next></block></next></block></next></block>
      </next></block></next></block></next></block></xml>`
	if err := s.LoadProgram("latest.xml", []byte(xml)); err != nil {
		t.Fatal(err)
	}
	if err := s.Connect(context.Background(), true); err != nil {
		t.Fatal(err)
	}
	if err := s.Start(RunConfig{MinimumBattery: 20}); err != nil {
		t.Fatal(err)
	}
	waitUntilStopped(t, s)
	snapshot := s.Snapshot()
	if snapshot.LastError != "" {
		t.Fatal(snapshot.LastError)
	}
	var messages strings.Builder
	for _, entry := range snapshot.Logs {
		messages.WriteString(entry.Message)
		messages.WriteByte('\n')
	}
	for _, expected := range []string{
		"→ speed 60",
		"→ curve 50 -50 0 0 -100 0 60",
		"→ curve 100 0 0 100 100 0 60",
		"Scia di fumo ignorata",
	} {
		if !strings.Contains(messages.String(), expected) {
			t.Errorf("missing %q in logs:\n%s", expected, messages.String())
		}
	}
}

func waitUntilStopped(t *testing.T, s *Session) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for s.Snapshot().Running {
		if time.Now().After(deadline) {
			t.Fatal("program did not finish")
		}
		time.Sleep(time.Millisecond)
	}
}

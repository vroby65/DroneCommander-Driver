package session

import (
	"context"
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

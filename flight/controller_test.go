package flight

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vroby65/DroneCommander-Driver/program"
	"github.com/vroby65/DroneCommander-Driver/tello"
)

type fakeCommander struct {
	commands []string
	state    tello.Telemetry
}

type telemetryCommander struct {
	mu    sync.Mutex
	state tello.Telemetry
}

func (f *telemetryCommander) Connect(context.Context) error { return nil }
func (f *telemetryCommander) Command(context.Context, string) (string, error) {
	return "ok", nil
}
func (f *telemetryCommander) Immediate(string) error { return nil }
func (f *telemetryCommander) Snapshot() tello.Telemetry {
	f.mu.Lock()
	defer f.mu.Unlock()
	state := f.state
	state.Values = make(map[string]float64, len(f.state.Values))
	for name, value := range f.state.Values {
		state.Values[name] = value
	}
	return state
}
func (f *telemetryCommander) Close() error { return nil }
func (f *telemetryCommander) setAcceleration(x, y, z float64, pitch, roll int) {
	f.mu.Lock()
	f.state.Values = map[string]float64{"agx": x, "agy": y, "agz": z}
	f.state.Pitch = pitch
	f.state.Roll = roll
	f.state.UpdatedAt = time.Now()
	f.mu.Unlock()
}

func (f *fakeCommander) Connect(context.Context) error { return nil }
func (f *fakeCommander) Command(_ context.Context, command string) (string, error) {
	f.commands = append(f.commands, command)
	if command == "battery?" {
		return "75", nil
	}
	return "ok", nil
}
func (f *fakeCommander) Immediate(command string) error {
	f.commands = append(f.commands, command)
	return nil
}
func (f *fakeCommander) Snapshot() tello.Telemetry { return f.state }
func (f *fakeCommander) Close() error              { return nil }

func TestControllerUsesXMLValuesAsCentimeters(t *testing.T) {
	device := &fakeCommander{state: tello.Telemetry{Height: 80}}
	controller := NewController(device, Config{MinimumBattery: 20})
	ctx := context.Background()
	for _, action := range []struct {
		kind string
		args map[string]program.Value
	}{{"take_off", nil}, {"walk", map[string]program.Value{"DIST": float64(25)}}, {"slide", map[string]program.Value{"SLIDE": float64(-40)}}, {"change_angle", map[string]program.Value{"ANGLE": float64(90)}}, {"land", nil}} {
		if err := controller.Action(ctx, action.kind, action.args); err != nil {
			t.Fatalf("%s: %v", action.kind, err)
		}
	}
	want := []string{"battery?", "takeoff", "speed 30", "forward 25", "left 40", "cw 90", "land"}
	if strings.Join(device.commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %#v, want %#v", device.commands, want)
	}
}

func TestControllerRejectsSubMinimumMovement(t *testing.T) {
	device := &fakeCommander{state: tello.Telemetry{Height: 80}}
	controller := NewController(device, Config{})
	controller.flying = true
	err := controller.Action(context.Background(), "walk", map[string]program.Value{"DIST": float64(19)})
	if err == nil || !strings.Contains(err.Error(), "minimo Tello") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestControllerExecutesCameraCallbacksWithoutDroneCommands(t *testing.T) {
	device := &fakeCommander{}
	var calls []string
	controller := NewController(device, Config{
		TakePhoto: func(context.Context) (string, error) {
			calls = append(calls, "photo")
			return "/media/photo.png", nil
		},
		StartRecording: func(context.Context) error {
			calls = append(calls, "start")
			return nil
		},
		SaveRecording: func(context.Context) (string, error) {
			calls = append(calls, "save")
			return "/media/video.mp4", nil
		},
	})
	for _, kind := range []string{"take_photo", "start_recording", "save_recording"} {
		if err := controller.Action(context.Background(), kind, nil); err != nil {
			t.Fatalf("%s: %v", kind, err)
		}
	}
	if got := strings.Join(calls, "|"); got != "photo|start|save" {
		t.Fatalf("camera callback order = %q", got)
	}
	if len(device.commands) != 0 {
		t.Fatalf("camera blocks sent Tello SDK commands: %#v", device.commands)
	}
}

func TestCameraCallbackFailureIsLoggedAndDoesNotAbortFlight(t *testing.T) {
	device := &fakeCommander{}
	var logs []string
	controller := NewController(device, Config{
		TakePhoto: func(context.Context) (string, error) {
			return "", context.DeadlineExceeded
		},
		Log: func(message string) { logs = append(logs, message) },
	})
	if err := controller.Action(context.Background(), "take_photo", nil); err != nil {
		t.Fatalf("camera failure aborted the program: %v", err)
	}
	if joined := strings.Join(logs, "\n"); !strings.Contains(joined, "Foto non riuscita") {
		t.Fatalf("camera failure was not logged:\n%s", joined)
	}
}

func TestVectorCoordinates(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 80
	err := controller.Action(context.Background(), "move_by", map[string]program.Value{"X": float64(40), "Y": float64(20), "Z": float64(60)})
	if err != nil {
		t.Fatal(err)
	}
	if got := device.commands[0]; got != "go 60 -40 20 30" {
		t.Fatalf("command = %q", got)
	}
}

func TestOneHundredXMLUnitsAreOneHundredCentimeters(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 80
	if err := controller.Action(context.Background(), "walk", map[string]program.Value{"DIST": float64(100)}); err != nil {
		t.Fatal(err)
	}
	if got := device.commands[0]; got != "forward 100" {
		t.Fatalf("command = %q", got)
	}
}

func TestIndoorAltitudeBelowTwentyCentimetersIsRejected(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 80
	err := controller.Action(context.Background(), "set_altitude", map[string]program.Value{"ALTITUDE": float64(10)})
	if err == nil || !strings.Contains(err.Error(), "minimo di sicurezza") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestSetAndChangeAltitudeUseAbsoluteAndRelativeCentimeters(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 80

	if err := controller.Action(context.Background(), "set_altitude", map[string]program.Value{"ALTITUDE": float64(140)}); err != nil {
		t.Fatal(err)
	}
	if err := controller.Action(context.Background(), "change_altitude", map[string]program.Value{"ALTITUDE": float64(-20)}); err != nil {
		t.Fatal(err)
	}

	want := []string{"up 60", "down 20"}
	if strings.Join(device.commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %#v, want %#v", device.commands, want)
	}
	if got := controller.Result().Y; got != 120 {
		t.Fatalf("final altitude = %v cm, want 120 cm", got)
	}
}

func TestSetAngleStartsFromLiveDroneYawOnEveryFlight(t *testing.T) {
	device := &fakeCommander{state: tello.Telemetry{Height: 80, Yaw: 90}}
	controller := NewController(device, Config{})
	if err := controller.Action(context.Background(), "take_off", nil); err != nil {
		t.Fatal(err)
	}
	if err := controller.Action(context.Background(), "set_angle", map[string]program.Value{"ANGLE": float64(0)}); err != nil {
		t.Fatal(err)
	}

	want := []string{"battery?", "takeoff", "speed 30", "ccw 90"}
	if strings.Join(device.commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %#v, want %#v", device.commands, want)
	}
	if got := controller.Result().Heading; got != 0 {
		t.Fatalf("heading = %v degrees, want 0", got)
	}
}

func TestTelemetryDistanceDoesNotBlockMovement(t *testing.T) {
	device := &fakeCommander{state: tello.Telemetry{Height: 80, Values: map[string]float64{"tof": 5}}}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 80
	if err := controller.Action(context.Background(), "walk", map[string]program.Value{"DIST": float64(30)}); err != nil {
		t.Fatal(err)
	}
	if got := device.commands[0]; got != "forward 30" {
		t.Fatalf("command = %q, want forward 30", got)
	}
}

func TestCollisionCheckBlocksMovementAndLogsIntervention(t *testing.T) {
	device := &fakeCommander{state: tello.Telemetry{Height: 80, Values: map[string]float64{"tof": 5}}}
	var logs []string
	controller := NewController(device, Config{
		CollisionCheck: true,
		Log:            func(message string) { logs = append(logs, message) },
	})
	controller.flying = true
	controller.y = 80

	err := controller.Action(context.Background(), "walk", map[string]program.Value{"DIST": float64(30)})
	if err == nil || !strings.Contains(err.Error(), "ostacolo rilevato a 5 cm") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(device.commands) != 0 {
		t.Fatalf("movement was sent despite collision check: %#v", device.commands)
	}
	if joined := strings.Join(logs, "\n"); !strings.Contains(joined, "CONTROLLO COLLISIONI") {
		t.Fatalf("collision intervention was not logged:\n%s", joined)
	}
}

func TestImpactMonitorDetectsAccelerationSpikeWhileAirborne(t *testing.T) {
	device := &telemetryCommander{}
	device.setAcceleration(0, 0, -1000, 0, 0)
	controller := NewController(device, Config{CollisionCheck: true})
	controller.impactArmed.Store(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	detected := make(chan Impact, 1)
	go func() {
		if impact, ok := controller.MonitorImpact(ctx); ok {
			detected <- impact
		}
	}()

	// Let the monitor acquire a baseline packet, then publish a distinct 1.6 g
	// discontinuity like the impulse generated by an impact.
	time.Sleep(3 * collisionMonitorInterval)
	device.setAcceleration(1600, 0, -1000, 8, -5)

	select {
	case impact := <-detected:
		if impact.AccelerationDeltaMG < impactAccelerationDeltaMG {
			t.Fatalf("impact delta = %.0f mg, threshold = %.0f mg", impact.AccelerationDeltaMG, impactAccelerationDeltaMG)
		}
		if controller.impactArmed.Load() {
			t.Fatal("impact monitor remained armed after the first intervention")
		}
	case <-time.After(time.Second):
		t.Fatal("impact acceleration was not detected")
	}
}

func TestImpactMonitorIgnoresNormalFlightAcceleration(t *testing.T) {
	device := &telemetryCommander{}
	device.setAcceleration(0, 0, -1000, 0, 0)
	controller := NewController(device, Config{CollisionCheck: true})
	controller.impactArmed.Store(true)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan bool, 1)
	go func() {
		_, detected := controller.MonitorImpact(ctx)
		done <- detected
	}()
	time.Sleep(3 * collisionMonitorInterval)
	device.setAcceleration(250, -150, -950, 18, -12)
	time.Sleep(3 * collisionMonitorInterval)
	cancel()

	if detected := <-done; detected {
		t.Fatal("normal flight acceleration was classified as an impact")
	}
}

func TestReturnToBaseIgnoresClosedRouteFloatingPointResidue(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 80
	controller.x = 1e-12
	controller.z = -1e-12

	if err := controller.Action(context.Background(), "return_to_base", nil); err != nil {
		t.Fatal(err)
	}
	if len(device.commands) != 0 {
		t.Fatalf("unexpected commands for negligible residue: %#v", device.commands)
	}
}

func TestEveryFlightStepIsAnalyzedInTheLog(t *testing.T) {
	device := &fakeCommander{state: tello.Telemetry{Battery: 29, Height: 80, Pitch: 21, UpdatedAt: time.Now()}}
	var logs []string
	controller := NewController(device, Config{Log: func(message string) { logs = append(logs, message) }})
	controller.flying = true
	controller.y = 80
	if err := controller.Action(context.Background(), "walk", map[string]program.Value{"DIST": float64(30)}); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(logs, "\n")
	for _, expected := range []string{"STEP walk", "batteria bassa 29%", "assetto marcato pitch=21"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in step analysis:\n%s", expected, joined)
		}
	}
}

func TestSpeedUsesDroneCommanderZeroToTenRange(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	if err := controller.Action(context.Background(), "set_speed", map[string]program.Value{"SPEED": float64(6)}); err != nil {
		t.Fatal(err)
	}
	if got := device.commands[0]; got != "speed 60" {
		t.Fatalf("command = %q, want speed 60", got)
	}
}

func TestRelativeCurveUsesNativeTelloCurve(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 80
	arguments := map[string]program.Value{
		"X": float64(50), "Y": float64(0), "Z": float64(50),
		"XD": float64(50), "YD": float64(0), "ZD": float64(-50),
	}
	if err := controller.Action(context.Background(), "curve", arguments); err != nil {
		t.Fatal(err)
	}
	if got := device.commands[0]; got != "curve 50 -50 0 0 -100 0 30" {
		t.Fatalf("command = %q", got)
	}
}

func TestVerticalCircleUsesCorrectDirectionForAllFourQuarters(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 100

	quarters := []map[string]program.Value{
		{"X": float64(35), "Y": float64(15), "Z": float64(0), "XD": float64(15), "YD": float64(35), "ZD": float64(0)},
		{"X": float64(-15), "Y": float64(35), "Z": float64(0), "XD": float64(-35), "YD": float64(15), "ZD": float64(0)},
		{"X": float64(-35), "Y": float64(-15), "Z": float64(0), "XD": float64(-15), "YD": float64(-35), "ZD": float64(0)},
		{"X": float64(15), "Y": float64(-35), "Z": float64(0), "XD": float64(35), "YD": float64(-15), "ZD": float64(0)},
	}
	for index, arguments := range quarters {
		if err := controller.Action(context.Background(), "curve", arguments); err != nil {
			t.Fatalf("quarter %d: %v", index+1, err)
		}
	}

	want := []string{
		"curve 0 -35 15 0 -50 50 30",
		"curve 0 15 35 0 50 50 30",
		"curve 0 35 -15 0 50 -50 30",
		"curve 0 -15 -35 0 -50 -50 30",
	}
	if strings.Join(device.commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %#v, want %#v", device.commands, want)
	}
	result := controller.Result()
	if result.X != 0 || result.Y != 100 || result.Z != 0 {
		t.Fatalf("final position = (%v, %v, %v), want (0, 100, 0)", result.X, result.Y, result.Z)
	}
}

func TestInvalidNativeCurveFallsBackToGoSegments(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	controller.y = 80
	arguments := map[string]program.Value{
		"X": float64(0), "Y": float64(0), "Z": float64(30),
		"XD": float64(0), "YD": float64(0), "ZD": float64(30),
	}
	if err := controller.Action(context.Background(), "curve", arguments); err != nil {
		t.Fatal(err)
	}
	want := []string{"go 30 0 0 30", "go 30 0 0 30"}
	if strings.Join(device.commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %#v, want %#v", device.commands, want)
	}
}

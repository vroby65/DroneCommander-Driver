package flight

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/vroby65/DroneCommander-Driver/program"
	"github.com/vroby65/DroneCommander-Driver/tello"
)

type fakeCommander struct {
	commands []string
	state    tello.Telemetry
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

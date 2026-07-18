package flight

import (
	"context"
	"strings"
	"testing"

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

func TestControllerScalesBasicFlight(t *testing.T) {
	device := &fakeCommander{state: tello.Telemetry{Height: 80}}
	controller := NewController(device, Config{CentimetersPerUnit: 20, MinimumBattery: 20})
	ctx := context.Background()
	for _, action := range []struct {
		kind string
		args map[string]program.Value
	}{{"take_off", nil}, {"walk", map[string]program.Value{"DIST": float64(1)}}, {"slide", map[string]program.Value{"SLIDE": float64(-2)}}, {"change_angle", map[string]program.Value{"ANGLE": float64(90)}}, {"land", nil}} {
		if err := controller.Action(ctx, action.kind, action.args); err != nil {
			t.Fatalf("%s: %v", action.kind, err)
		}
	}
	want := []string{"battery?", "takeoff", "speed 20", "forward 20", "left 40", "cw 90", "land"}
	if strings.Join(device.commands, "|") != strings.Join(want, "|") {
		t.Fatalf("commands = %#v, want %#v", device.commands, want)
	}
}

func TestControllerRejectsSubMinimumMovement(t *testing.T) {
	device := &fakeCommander{state: tello.Telemetry{Height: 80}}
	controller := NewController(device, Config{CentimetersPerUnit: 20})
	controller.flying = true
	err := controller.Action(context.Background(), "walk", map[string]program.Value{"DIST": float64(.5)})
	if err == nil || !strings.Contains(err.Error(), "minimo Tello") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVectorCoordinates(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{CentimetersPerUnit: 20})
	controller.flying = true
	err := controller.Action(context.Background(), "move_by", map[string]program.Value{"X": float64(2), "Y": float64(1), "Z": float64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if got := device.commands[0]; got != "go 60 40 20 20" {
		t.Fatalf("command = %q", got)
	}
}

func TestDefaultScaleIsOneCentimeter(t *testing.T) {
	device := &fakeCommander{}
	controller := NewController(device, Config{})
	controller.flying = true
	if err := controller.Action(context.Background(), "walk", map[string]program.Value{"DIST": float64(20)}); err != nil {
		t.Fatal(err)
	}
	if got := device.commands[0]; got != "forward 20" {
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

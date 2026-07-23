package tello

import (
	"context"
	"testing"
	"time"
)

func TestParseTelemetry(t *testing.T) {
	state := ParseTelemetry("pitch:1;roll:-2;yaw:45;templ:60;temph:64;h:83;bat:72;time:14;\r\n")
	if state.Pitch != 1 || state.Roll != -2 || state.Yaw != 45 {
		t.Fatalf("attitude: %+v", state)
	}
	if state.Height != 83 || state.Battery != 72 || state.FlightTime != 14 || state.Temperature != 62 {
		t.Fatalf("telemetry: %+v", state)
	}
}

func TestBatteryQueryUpdatesSnapshot(t *testing.T) {
	client := NewClient("", "", 0)
	client.recordResponse("battery?", "73")
	state := client.Snapshot()
	if state.Battery != 73 || state.UpdatedAt.IsZero() {
		t.Fatalf("snapshot after battery query = %#v", state)
	}
}

func TestMovementTimeoutScalesWithDistanceAndSpeed(t *testing.T) {
	client := NewClient("", "", 8*time.Second)
	if got := client.timeoutFor("battery?"); got != 8*time.Second {
		t.Fatalf("battery timeout = %s, want 8s", got)
	}
	if got := client.timeoutFor("forward 100"); got < 15*time.Second {
		t.Fatalf("100 cm walk timeout = %s, want at least 15s", got)
	}
	client.recordResponse("speed 50", "ok")
	if got := client.timeoutFor("forward 100"); got != 8*time.Second {
		t.Fatalf("100 cm walk at 50 cm/s timeout = %s, want base 8s", got)
	}
	if got := client.timeoutFor("go 500 500 500 10"); got < 90*time.Second {
		t.Fatalf("long go timeout = %s, want at least 90s", got)
	}
}

func TestSimulatorTracksAltitudeForVectorAndCurveCommands(t *testing.T) {
	simulator := NewSimulator()
	if err := simulator.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{
		"takeoff",
		"go 30 0 40 10",
		"curve 0 20 10 0 40 -20 10",
	} {
		if _, err := simulator.Command(context.Background(), command); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	if got := simulator.Snapshot().Height; got != 100 {
		t.Fatalf("simulated altitude = %d cm, want 100 cm", got)
	}
}

func TestSimulatorTracksClockwiseAndCounterclockwiseYaw(t *testing.T) {
	simulator := NewSimulator()
	if err := simulator.Connect(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"cw 90", "ccw 30"} {
		if _, err := simulator.Command(context.Background(), command); err != nil {
			t.Fatalf("%s: %v", command, err)
		}
	}
	if got := simulator.Snapshot().Yaw; got != 60 {
		t.Fatalf("simulated yaw = %d degrees, want 60", got)
	}
}

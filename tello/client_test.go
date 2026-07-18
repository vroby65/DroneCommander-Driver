package tello

import "testing"

func TestParseTelemetry(t *testing.T) {
	state := ParseTelemetry("pitch:1;roll:-2;yaw:45;templ:60;temph:64;h:83;bat:72;time:14;\r\n")
	if state.Pitch != 1 || state.Roll != -2 || state.Yaw != 45 {
		t.Fatalf("attitude: %+v", state)
	}
	if state.Height != 83 || state.Battery != 72 || state.FlightTime != 14 || state.Temperature != 62 {
		t.Fatalf("telemetry: %+v", state)
	}
}

package session

import (
	"context"
	"strings"
	"testing"
	"time"
)

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
	if !strings.Contains(s.Snapshot().LastError, "minimo Tello") {
		t.Fatalf("unexpected error: %q", s.Snapshot().LastError)
	}
}

package program

import (
	"context"
	"strings"
	"testing"
)

type recordedAction struct {
	kind string
	args map[string]Value
}

type recordingHost struct{ actions []recordedAction }

func (h *recordingHost) Action(_ context.Context, kind string, args map[string]Value) error {
	h.actions = append(h.actions, recordedAction{kind: kind, args: args})
	return nil
}
func (h *recordingHost) Sensor(string, string) Value { return false }

func TestParseAndExecuteRepeat(t *testing.T) {
	xml := `<xml xmlns="https://developers.google.com/blockly/xml">
  <block type="start_block"><next><block type="take_off"><next>
    <block type="controls_repeat_ext">
      <value name="TIMES"><shadow type="math_number"><field name="NUM">2</field></shadow></value>
      <statement name="DO"><block type="walk"><value name="DIST"><shadow type="math_number"><field name="NUM">1.5</field></shadow></value></block></statement>
      <next><block type="land"></block></next>
    </block>
  </next></block></next></block>
</xml>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if parsed.Summary.Commands != 3 {
		t.Fatalf("commands = %d, want 3 static commands", parsed.Summary.Commands)
	}
	host := &recordingHost{}
	interpreter := Interpreter{Program: parsed, Host: host}
	if err := interpreter.Execute(context.Background()); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []string{"take_off", "walk", "walk", "land"}
	if len(host.actions) != len(want) {
		t.Fatalf("actions = %d, want %d", len(host.actions), len(want))
	}
	for index, action := range host.actions {
		if action.kind != want[index] {
			t.Errorf("action %d = %s, want %s", index, action.kind, want[index])
		}
	}
	if got := host.actions[1].args["DIST"]; got != float64(1.5) {
		t.Errorf("walk distance = %v", got)
	}
}

func TestParseRejectsUnsupportedBlock(t *testing.T) {
	_, err := Parse([]byte(`<xml><block type="start_block"><next><block type="unknown_flight"></block></next></block></xml>`))
	if err == nil || !strings.Contains(err.Error(), "unknown_flight") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInterpreterVariablesAndCondition(t *testing.T) {
	xml := `<xml><block type="start_block"><next>
    <block type="variables_set"><field name="VAR" id="v">n</field><value name="VALUE"><block type="math_number"><field name="NUM">3</field></block></value><next>
      <block type="controls_if"><value name="IF0"><block type="logic_compare"><field name="OP">GT</field><value name="A"><block type="variables_get"><field name="VAR" id="v">n</field></block></value><value name="B"><block type="math_number"><field name="NUM">2</field></block></value></block></value><statement name="DO0"><block type="land"></block></statement></block>
    </next></block>
  </next></block></xml>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	host := &recordingHost{}
	if err := (&Interpreter{Program: parsed, Host: host}).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(host.actions) != 1 || host.actions[0].kind != "land" {
		t.Fatalf("actions: %#v", host.actions)
	}
}

func TestDisabledBlocksAreSkipped(t *testing.T) {
	xml := `<xml><block type="start_block"><next><block type="walk" disabled="true"><next><block type="land"></block></next></block></next></block></xml>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	host := &recordingHost{}
	if err := (&Interpreter{Program: parsed, Host: host}).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(host.actions) != 1 || host.actions[0].kind != "land" {
		t.Fatalf("actions: %#v", host.actions)
	}
}

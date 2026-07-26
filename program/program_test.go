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

func TestCameraBlocksParseAndExecute(t *testing.T) {
	xml := `<xml><block type="start_block"><next>
	  <block type="take_photo"><next>
	  <block type="start_recording"><next>
	  <block type="save_recording"></block>
	  </next></block></next></block>
	</next></block></xml>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Summary.Commands != 3 {
		t.Fatalf("commands = %d, want 3", parsed.Summary.Commands)
	}
	if parsed.Summary.MediaCommands != 3 {
		t.Fatalf("media commands = %d, want 3", parsed.Summary.MediaCommands)
	}
	host := &recordingHost{}
	if err := (&Interpreter{Program: parsed, Host: host}).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"take_photo", "start_recording", "save_recording"}
	for index, action := range host.actions {
		if action.kind != want[index] {
			t.Fatalf("action %d = %s, want %s", index, action.kind, want[index])
		}
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

func TestEveryBlockInDroneCommanderToolboxIsRecognized(t *testing.T) {
	// Kept in the same order as the editor toolbox. This catches newly visible
	// blocks that the driver can parse but might otherwise reject at load time.
	types := []string{
		"controls_if", "logic_compare", "logic_operation", "logic_negate", "logic_boolean", "logic_null", "logic_ternary",
		"controls_repeat_ext", "controls_whileUntil", "controls_for", "controls_forEach", "controls_flow_statements",
		"math_number", "math_arithmetic", "math_single", "math_trig", "math_constant", "math_number_property", "math_round", "math_on_list", "math_modulo", "math_constrain", "math_random_int", "math_random_float",
		"text", "text_join", "text_append", "text_length", "text_isEmpty", "text_indexOf", "text_charAt", "text_getSubstring", "text_changeCase", "text_trim", "text_print",
		"lists_create_with", "lists_repeat", "lists_length", "lists_isEmpty", "lists_indexOf", "lists_getIndex", "lists_setIndex", "lists_getSublist", "lists_split", "lists_sort",
		"sensor_keypressed", "sensor_x", "sensor_z", "sensor_altitude", "sensor_direction", "sensor_speed",
		"take_off", "land", "take_photo", "start_recording", "save_recording", "return_to_base", "set_altitude", "change_altitude", "set_angle", "change_angle", "slide", "walk", "walk_climbing", "go_to", "move_by", "curve_abs", "curve", "wait", "smoke", "set_speed", "end_block",
	}
	for _, blockType := range types {
		xml := `<xml><block type="start_block"></block><block type="` + blockType + `"></block></xml>`
		if _, err := Parse([]byte(xml)); err != nil {
			t.Errorf("block %s rejected: %v", blockType, err)
		}
	}
}

func TestTextAndListToolboxStatementsExecute(t *testing.T) {
	xml := `<xml><block type="start_block"><next>
      <block type="variables_set"><field name="VAR" id="text">message</field>
        <value name="VALUE"><block type="text"><field name="TEXT"> drone </field></block></value><next>
      <block type="text_append"><field name="VAR" id="text">message</field>
        <value name="TEXT"><block type="text"><field name="TEXT">commander </field></block></value><next>
      <block type="text_print"><value name="TEXT"><block type="text_changeCase"><field name="CASE">UPPERCASE</field>
        <value name="TEXT"><block type="text_trim"><field name="MODE">BOTH</field>
          <value name="TEXT"><block type="variables_get"><field name="VAR" id="text">message</field></block></value>
        </block></value></block></value>
      </block></next></block></next></block>
    </next></block></xml>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	host := &recordingHost{}
	if err := (&Interpreter{Program: parsed, Host: host}).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(host.actions) != 1 || host.actions[0].kind != "text_print" || host.actions[0].args["TEXT"] != "DRONE COMMANDER" {
		t.Fatalf("printed actions: %#v", host.actions)
	}
}

func TestListMutationAndMathOnListExecute(t *testing.T) {
	xml := `<xml><block type="start_block"><next>
      <block type="variables_set"><field name="VAR" id="numbers">numbers</field>
        <value name="VALUE"><block type="lists_create_with"><mutation items="3"></mutation>
          <value name="ADD0"><block type="math_number"><field name="NUM">3</field></block></value>
          <value name="ADD1"><block type="math_number"><field name="NUM">1</field></block></value>
          <value name="ADD2"><block type="math_number"><field name="NUM">2</field></block></value>
        </block></value><next>
      <block type="lists_setIndex"><field name="MODE">SET</field><field name="WHERE">FIRST</field>
        <value name="LIST"><block type="variables_get"><field name="VAR" id="numbers">numbers</field></block></value>
        <value name="TO"><block type="math_number"><field name="NUM">4</field></block></value><next>
      <block type="text_print"><value name="TEXT"><block type="math_on_list"><field name="OP">SUM</field>
        <value name="LIST"><block type="variables_get"><field name="VAR" id="numbers">numbers</field></block></value>
      </block></value></block>
      </next></block></next></block>
    </next></block></xml>`
	parsed, err := Parse([]byte(xml))
	if err != nil {
		t.Fatal(err)
	}
	host := &recordingHost{}
	if err := (&Interpreter{Program: parsed, Host: host}).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(host.actions) != 1 || host.actions[0].args["TEXT"] != float64(7) {
		t.Fatalf("printed actions: %#v", host.actions)
	}
}

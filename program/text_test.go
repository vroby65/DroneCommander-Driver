package program

import (
	"context"
	"os"
	"strings"
	"testing"
)

func TestTextProgramRoundTripExample(t *testing.T) {
	data, err := os.ReadFile("../examples/quadrato.xml")
	if err != nil {
		t.Fatal(err)
	}
	text, err := ToTextProgram(data)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"TAKE_OFF",
		"SET_SPEED speed=30",
		"REPEAT times=4 {",
		"  WALK distance=50",
		"  CHANGE_ANGLE angle=90",
		"LAND",
	} {
		if !strings.Contains(text, expected) {
			t.Fatalf("text program does not contain %q:\n%s", expected, text)
		}
	}

	generated, err := TextProgramToXML(text)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(generated)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Summary.Commands != 5 {
		t.Fatalf("generated command count = %d, want 5", parsed.Summary.Commands)
	}
	host := &recordingHost{}
	if err := (&Interpreter{Program: parsed, Host: host}).Execute(context.Background()); err != nil {
		t.Fatal(err)
	}
	wantActions := []string{"take_off", "set_speed", "walk", "change_angle", "walk", "change_angle", "walk", "change_angle", "walk", "change_angle", "land"}
	if len(host.actions) != len(wantActions) {
		t.Fatalf("executed actions = %d, want %d", len(host.actions), len(wantActions))
	}
	for index, want := range wantActions {
		if got := host.actions[index].kind; got != want {
			t.Fatalf("action %d = %s, want %s", index, got, want)
		}
	}
	again, err := ToTextProgram(generated)
	if err != nil {
		t.Fatal(err)
	}
	if again != text {
		t.Fatalf("text round trip changed:\n--- before ---\n%s--- after ---\n%s", text, again)
	}
}

func TestTextProgramSupportsLatestFlightCommands(t *testing.T) {
	source := `TAKE_OFF
TAKE_PHOTO
START_RECORDING
SAVE_RECORDING
SET_ALTITUDE altitude=80
CHANGE_ALTITUDE altitude=20
SET_ANGLE angle=180
CHANGE_ANGLE angle=-90
SLIDE distance=30
WALK distance=50
WALK_CLIMBING distance=40 climb=20
GO_TO x=100 y=80 z=100
MOVE_BY x=-30 y=20 z=40
CURVE x1=50 y1=0 z1=50 x2=50 y2=0 z2=-50
CURVE_ABS x1=100 y1=80 z1=100 x2=0 y2=80 z2=100
WAIT seconds=1.5
SMOKE enabled=true
SET_SPEED speed=6
RETURN_TO_BASE
LAND
`
	data, err := TextProgramToXML(source)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Summary.Commands != 20 {
		t.Fatalf("command count = %d, want 20\n%s", parsed.Summary.Commands, data)
	}
	text, err := ToTextProgram(data)
	if err != nil {
		t.Fatal(err)
	}
	if text != source {
		t.Fatalf("round trip changed commands:\n--- before ---\n%s--- after ---\n%s", source, text)
	}
}

func TestTextProgramRejectsInvalidAndAdvancedPrograms(t *testing.T) {
	for _, source := range []string{
		"WALK distance=nope\n",
		"REPEAT times=2 {\nLAND\n",
		"UNKNOWN value=1\n",
		"GO_TO x=1 y=2\n",
	} {
		if _, err := TextProgramToXML(source); err == nil {
			t.Fatalf("invalid source was accepted: %q", source)
		}
	}
	advanced := `<xml><block type="start_block"><next><block type="controls_if"></block></next></block></xml>`
	if _, err := ToTextProgram([]byte(advanced)); err == nil {
		t.Fatal("advanced Blockly program was converted with data loss")
	}
}

func TestTextProgramReferenceListsParameters(t *testing.T) {
	reference := TextProgramReference()
	for _, signature := range []string{
		"REPEAT times=<number> {",
		"WALK distance=<number>",
		"TAKE_PHOTO",
		"START_RECORDING",
		"SAVE_RECORDING",
		"CURVE x1=<number> y1=<number> z1=<number> x2=<number> y2=<number> z2=<number>",
		"SMOKE enabled=<true|false>",
	} {
		if !strings.Contains(reference, signature) {
			t.Fatalf("command reference does not contain %q:\n%s", signature, reference)
		}
	}
}

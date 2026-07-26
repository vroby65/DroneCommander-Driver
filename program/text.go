package program

import (
	"encoding/xml"
	"fmt"
	"strconv"
	"strings"
)

// TextProgram is a deliberately small, human-readable representation of the
// flight blocks most commonly edited outside Drone Commander. Keywords remain
// stable across UI languages so a program never changes meaning when the user
// switches locale.

type textParameter struct {
	Name    string
	XMLName string
	Boolean bool
}

type textCommand struct {
	Keyword   string
	BlockType string
	Params    []textParameter
}

var textCommands = []textCommand{
	{Keyword: "TAKE_OFF", BlockType: "take_off"},
	{Keyword: "LAND", BlockType: "land"},
	{Keyword: "TAKE_PHOTO", BlockType: "take_photo"},
	{Keyword: "START_RECORDING", BlockType: "start_recording"},
	{Keyword: "SAVE_RECORDING", BlockType: "save_recording"},
	{Keyword: "RETURN_TO_BASE", BlockType: "return_to_base"},
	{Keyword: "SET_ALTITUDE", BlockType: "set_altitude", Params: []textParameter{{Name: "altitude", XMLName: "ALTITUDE"}}},
	{Keyword: "CHANGE_ALTITUDE", BlockType: "change_altitude", Params: []textParameter{{Name: "altitude", XMLName: "ALTITUDE"}}},
	{Keyword: "SET_ANGLE", BlockType: "set_angle", Params: []textParameter{{Name: "angle", XMLName: "ANGLE"}}},
	{Keyword: "CHANGE_ANGLE", BlockType: "change_angle", Params: []textParameter{{Name: "angle", XMLName: "ANGLE"}}},
	{Keyword: "SLIDE", BlockType: "slide", Params: []textParameter{{Name: "distance", XMLName: "SLIDE"}}},
	{Keyword: "WALK", BlockType: "walk", Params: []textParameter{{Name: "distance", XMLName: "DIST"}}},
	{Keyword: "WALK_CLIMBING", BlockType: "walk_climbing", Params: []textParameter{{Name: "distance", XMLName: "DIST"}, {Name: "climb", XMLName: "CLIMB"}}},
	{Keyword: "GO_TO", BlockType: "go_to", Params: []textParameter{{Name: "x", XMLName: "X"}, {Name: "y", XMLName: "Y"}, {Name: "z", XMLName: "Z"}}},
	{Keyword: "MOVE_BY", BlockType: "move_by", Params: []textParameter{{Name: "x", XMLName: "X"}, {Name: "y", XMLName: "Y"}, {Name: "z", XMLName: "Z"}}},
	{Keyword: "CURVE", BlockType: "curve", Params: []textParameter{{Name: "x1", XMLName: "X"}, {Name: "y1", XMLName: "Y"}, {Name: "z1", XMLName: "Z"}, {Name: "x2", XMLName: "XD"}, {Name: "y2", XMLName: "YD"}, {Name: "z2", XMLName: "ZD"}}},
	{Keyword: "CURVE_ABS", BlockType: "curve_abs", Params: []textParameter{{Name: "x1", XMLName: "X"}, {Name: "y1", XMLName: "Y"}, {Name: "z1", XMLName: "Z"}, {Name: "x2", XMLName: "XD"}, {Name: "y2", XMLName: "YD"}, {Name: "z2", XMLName: "ZD"}}},
	{Keyword: "WAIT", BlockType: "wait", Params: []textParameter{{Name: "seconds", XMLName: "DIST"}}},
	{Keyword: "SMOKE", BlockType: "smoke", Params: []textParameter{{Name: "enabled", XMLName: "SMOKE", Boolean: true}}},
	{Keyword: "SET_SPEED", BlockType: "set_speed", Params: []textParameter{{Name: "speed", XMLName: "SPEED"}}},
	{Keyword: "END", BlockType: "end_block"},
}

var textCommandByKeyword = func() map[string]textCommand {
	commands := make(map[string]textCommand, len(textCommands))
	for _, command := range textCommands {
		commands[command.Keyword] = command
	}
	return commands
}()

var textCommandByBlock = func() map[string]textCommand {
	commands := make(map[string]textCommand, len(textCommands))
	for _, command := range textCommands {
		commands[command.BlockType] = command
	}
	return commands
}()

// TextProgramReference returns the accepted command signatures for the editor.
func TextProgramReference() string {
	var output strings.Builder
	output.WriteString("REPEAT times=<number> {\n  ...\n}\n")
	for _, command := range textCommands {
		output.WriteString(command.Keyword)
		for _, parameter := range command.Params {
			value := "<number>"
			if parameter.Boolean {
				value = "<true|false>"
			}
			fmt.Fprintf(&output, " %s=%s", parameter.Name, value)
		}
		output.WriteByte('\n')
	}
	return output.String()
}

// ToTextProgram converts a Blockly workspace into editable textual commands.
// It refuses advanced Blockly expressions instead of silently losing them.
func ToTextProgram(data []byte) (string, error) {
	parsed, err := Parse(data)
	if err != nil {
		return "", err
	}
	for _, root := range parsed.Roots {
		if root != parsed.Start {
			return "", fmt.Errorf("advanced root block %s is not representable as text", root.Type)
		}
	}
	var output strings.Builder
	if err := writeTextChain(&output, parsed.Start.Next.Selected(), 0); err != nil {
		return "", err
	}
	return strings.TrimRight(output.String(), "\n") + "\n", nil
}

func writeTextChain(output *strings.Builder, block *Block, depth int) error {
	indent := strings.Repeat("  ", depth)
	for block != nil {
		if block.IsDisabled() {
			return fmt.Errorf("disabled block %s is not representable as text", block.Type)
		}
		if block.Type == "controls_repeat_ext" {
			if len(block.Values) != 1 || block.Values[0].Name != "TIMES" || len(block.Statements) > 1 ||
				(len(block.Statements) == 1 && block.Statements[0].Name != "DO") {
				return fmt.Errorf("advanced REPEAT inputs are not representable as text")
			}
			times, err := textLiteral(block.Value("TIMES"), false)
			if err != nil {
				return fmt.Errorf("REPEAT times: %w", err)
			}
			fmt.Fprintf(output, "%sREPEAT times=%s {\n", indent, times)
			if err := writeTextChain(output, block.Statement("DO"), depth+1); err != nil {
				return err
			}
			fmt.Fprintf(output, "%s}\n", indent)
			block = block.Next.Selected()
			continue
		}
		command, ok := textCommandByBlock[block.Type]
		if !ok {
			return fmt.Errorf("advanced block %s is not representable as text", block.Type)
		}
		if len(block.Statements) != 0 || !onlyTextParameters(block.Values, command.Params) {
			return fmt.Errorf("advanced inputs on block %s are not representable as text", block.Type)
		}
		output.WriteString(indent)
		output.WriteString(command.Keyword)
		for _, parameter := range command.Params {
			value, err := textLiteral(block.Value(parameter.XMLName), parameter.Boolean)
			if err != nil {
				return fmt.Errorf("%s %s: %w", command.Keyword, parameter.Name, err)
			}
			fmt.Fprintf(output, " %s=%s", parameter.Name, value)
		}
		output.WriteByte('\n')
		block = block.Next.Selected()
	}
	return nil
}

func onlyTextParameters(values []Slot, parameters []textParameter) bool {
	if len(values) != len(parameters) {
		return false
	}
	allowed := make(map[string]bool, len(parameters))
	for _, parameter := range parameters {
		allowed[parameter.XMLName] = true
	}
	for _, value := range values {
		if !allowed[value.Name] {
			return false
		}
	}
	return true
}

func textLiteral(block *Block, boolean bool) (string, error) {
	if block == nil {
		if boolean {
			return "false", nil
		}
		return "0", nil
	}
	if boolean {
		if block.Type != "logic_boolean" {
			return "", fmt.Errorf("expression %s is not a boolean literal", block.Type)
		}
		return strconv.FormatBool(strings.EqualFold(block.Field("BOOL").Value, "TRUE")), nil
	}
	if block.Type != "math_number" {
		return "", fmt.Errorf("expression %s is not a numeric literal", block.Type)
	}
	value := block.Field("NUM").Value
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return "", fmt.Errorf("invalid number %q", value)
	}
	return value, nil
}

type textSourceLine struct {
	Number int
	Text   string
}

// TextProgramToXML parses textual commands and creates a Blockly workspace
// that can be reopened in Drone Commander.
func TextProgramToXML(source string) ([]byte, error) {
	lines := make([]textSourceLine, 0)
	for index, raw := range strings.Split(source, "\n") {
		line := strings.TrimSpace(raw)
		if comment := strings.IndexByte(line, '#'); comment >= 0 {
			line = strings.TrimSpace(line[:comment])
		}
		if line != "" {
			lines = append(lines, textSourceLine{Number: index + 1, Text: line})
		}
	}
	position := 0
	chain, err := parseTextChain(lines, &position, false)
	if err != nil {
		return nil, err
	}
	root := &Block{Type: "start_block", X: "20", Y: "20"}
	if chain != nil {
		root.Next = &Next{Block: chain}
	}
	document := struct {
		XMLName xml.Name `xml:"xml"`
		XMLNS   string   `xml:"xmlns,attr"`
		Blocks  []*Block `xml:"block"`
	}{XMLNS: "https://developers.google.com/blockly/xml", Blocks: []*Block{root}}
	data, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if _, err := Parse(data); err != nil {
		return nil, fmt.Errorf("generated Blockly XML is invalid: %w", err)
	}
	return data, nil
}

func parseTextChain(lines []textSourceLine, position *int, nested bool) (*Block, error) {
	var first, last *Block
	for *position < len(lines) {
		line := lines[*position]
		if line.Text == "}" {
			if !nested {
				return nil, fmt.Errorf("line %d: unexpected }", line.Number)
			}
			(*position)++
			return first, nil
		}

		var block *Block
		upper := strings.ToUpper(line.Text)
		if strings.HasPrefix(upper, "REPEAT ") || upper == "REPEAT{" || upper == "REPEAT {" {
			if !strings.HasSuffix(line.Text, "{") {
				return nil, fmt.Errorf("line %d: REPEAT must end with {", line.Number)
			}
			header := strings.TrimSpace(strings.TrimSuffix(line.Text, "{"))
			parts := strings.Fields(header)
			params, err := parseTextParameters(line.Number, parts[1:])
			if err != nil {
				return nil, err
			}
			times, err := requiredTextParameter(line.Number, params, "times", false)
			if err != nil {
				return nil, err
			}
			if len(params) != 1 {
				return nil, fmt.Errorf("line %d: REPEAT accepts only times=", line.Number)
			}
			(*position)++
			body, err := parseTextChain(lines, position, true)
			if err != nil {
				return nil, err
			}
			block = &Block{Type: "controls_repeat_ext", Values: []Slot{numberSlot("TIMES", times)}}
			if body != nil {
				block.Statements = []Slot{{Name: "DO", Block: body}}
			}
		} else {
			parts := strings.Fields(line.Text)
			keyword := strings.ToUpper(parts[0])
			command, ok := textCommandByKeyword[keyword]
			if !ok {
				return nil, fmt.Errorf("line %d: unknown command %s", line.Number, parts[0])
			}
			params, err := parseTextParameters(line.Number, parts[1:])
			if err != nil {
				return nil, err
			}
			if len(params) != len(command.Params) {
				return nil, fmt.Errorf("line %d: %s requires %d parameter(s)", line.Number, keyword, len(command.Params))
			}
			block = &Block{Type: command.BlockType}
			for _, parameter := range command.Params {
				value, err := requiredTextParameter(line.Number, params, parameter.Name, parameter.Boolean)
				if err != nil {
					return nil, err
				}
				if parameter.Boolean {
					block.Values = append(block.Values, booleanSlot(parameter.XMLName, value))
				} else {
					block.Values = append(block.Values, numberSlot(parameter.XMLName, value))
				}
			}
			(*position)++
		}

		if first == nil {
			first = block
		} else {
			last.Next = &Next{Block: block}
		}
		last = block
	}
	if nested {
		return nil, fmt.Errorf("missing } at end of REPEAT")
	}
	return first, nil
}

func parseTextParameters(line int, fields []string) (map[string]string, error) {
	params := make(map[string]string, len(fields))
	for _, field := range fields {
		name, value, ok := strings.Cut(field, "=")
		name = strings.ToLower(strings.TrimSpace(name))
		value = strings.TrimSpace(value)
		if !ok || name == "" || value == "" {
			return nil, fmt.Errorf("line %d: expected name=value, found %q", line, field)
		}
		if _, exists := params[name]; exists {
			return nil, fmt.Errorf("line %d: duplicate parameter %s", line, name)
		}
		params[name] = value
	}
	return params, nil
}

func requiredTextParameter(line int, params map[string]string, name string, boolean bool) (string, error) {
	value, ok := params[name]
	if !ok {
		return "", fmt.Errorf("line %d: missing parameter %s=", line, name)
	}
	if boolean {
		parsed, err := strconv.ParseBool(strings.ToLower(value))
		if err != nil {
			return "", fmt.Errorf("line %d: %s must be true or false", line, name)
		}
		return strconv.FormatBool(parsed), nil
	}
	if _, err := strconv.ParseFloat(value, 64); err != nil {
		return "", fmt.Errorf("line %d: %s is not a valid number", line, name)
	}
	return value, nil
}

func numberSlot(name, value string) Slot {
	return Slot{Name: name, Shadow: &Block{Type: "math_number", Fields: []Field{{Name: "NUM", Value: value}}}}
}

func booleanSlot(name, value string) Slot {
	return Slot{Name: name, Shadow: &Block{Type: "logic_boolean", Fields: []Field{{Name: "BOOL", Value: strings.ToUpper(value)}}}}
}

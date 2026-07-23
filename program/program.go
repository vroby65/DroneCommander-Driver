// Package program parses and interprets Blockly XML files produced by Drone Commander.
package program

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"math/rand"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// Field is a Blockly field. Variable fields also carry a stable ID.
type Field struct {
	Name  string `xml:"name,attr"`
	ID    string `xml:"id,attr,omitempty"`
	Value string `xml:",chardata"`
}

// Mutation contains Blockly's block-specific metadata.
type Mutation struct {
	Attrs []xml.Attr    `xml:",any,attr"`
	Args  []MutationArg `xml:"arg"`
}

type MutationArg struct {
	Name  string `xml:"name,attr"`
	VarID string `xml:"varid,attr"`
}

// Child is the connected block in a value, statement, or next slot. Blockly may
// retain a shadow under a real block; the real block always wins.
type Child struct {
	Block  *Block `xml:"block"`
	Shadow *Block `xml:"shadow"`
}

func (c Child) Selected() *Block {
	if c.Block != nil {
		return c.Block
	}
	return c.Shadow
}

type Slot struct {
	Name   string `xml:"name,attr"`
	Block  *Block `xml:"block"`
	Shadow *Block `xml:"shadow"`
}

func (s Slot) Selected() *Block {
	if s.Block != nil {
		return s.Block
	}
	return s.Shadow
}

type Next struct {
	Block  *Block `xml:"block"`
	Shadow *Block `xml:"shadow"`
}

func (n *Next) Selected() *Block {
	if n == nil {
		return nil
	}
	if n.Block != nil {
		return n.Block
	}
	return n.Shadow
}

// Block is the subset of Blockly XML needed by the interpreter.
type Block struct {
	Type       string    `xml:"type,attr"`
	ID         string    `xml:"id,attr,omitempty"`
	X          string    `xml:"x,attr,omitempty"`
	Y          string    `xml:"y,attr,omitempty"`
	Disabled   string    `xml:"disabled,attr,omitempty"`
	Fields     []Field   `xml:"field"`
	Values     []Slot    `xml:"value"`
	Statements []Slot    `xml:"statement"`
	Next       *Next     `xml:"next"`
	Mutation   *Mutation `xml:"mutation"`
}

func (b *Block) IsDisabled() bool {
	return b != nil && strings.EqualFold(b.Disabled, "true")
}

func (b *Block) Field(name string) Field {
	for _, field := range b.Fields {
		if field.Name == name {
			// Whitespace is data in Blockly text literals; other fields are
			// identifiers, modes, or numbers and may be normalized safely.
			if field.Name != "TEXT" {
				field.Value = strings.TrimSpace(field.Value)
			}
			return field
		}
	}
	return Field{Name: name}
}

func (b *Block) Value(name string) *Block {
	for _, slot := range b.Values {
		if slot.Name == name {
			return slot.Selected()
		}
	}
	return nil
}

func (b *Block) Statement(name string) *Block {
	for _, slot := range b.Statements {
		if slot.Name == name {
			return slot.Selected()
		}
	}
	return nil
}

func (b *Block) MutationAttr(name string) string {
	if b.Mutation == nil {
		return ""
	}
	for _, attr := range b.Mutation.Attrs {
		if attr.Name.Local == name {
			return attr.Value
		}
	}
	return ""
}

type xmlDocument struct {
	XMLName xml.Name `xml:"xml"`
	Blocks  []*Block `xml:"block"`
}

// Program is a parsed Drone Commander workspace.
type Program struct {
	Roots      []*Block
	Start      *Block
	Procedures map[string]*Block
	Summary    Summary
}

type Summary struct {
	Blocks   int      `json:"blocks"`
	Commands int      `json:"commands"`
	Warnings []string `json:"warnings,omitempty"`
}

var actionTypes = map[string]bool{
	"take_off": true, "land": true, "return_to_base": true,
	"set_altitude": true, "change_altitude": true, "set_angle": true,
	"change_angle": true, "slide": true, "walk": true,
	"walk_climbing": true, "go_to": true, "move_by": true,
	"curve_abs": true, "curve": true, "wait": true, "smoke": true,
	"set_speed": true,
}

var statementTypes = map[string]bool{
	"start_block": true, "end_block": true,
	"controls_if": true, "controls_repeat_ext": true,
	"controls_whileUntil": true, "controls_for": true,
	"controls_forEach": true, "controls_flow_statements": true,
	"variables_set": true, "math_change": true,
	"text_append": true, "text_print": true,
	"lists_getIndex": true, "lists_setIndex": true,
	"procedures_defnoreturn": true, "procedures_defreturn": true,
	"procedures_callnoreturn": true, "procedures_ifreturn": true,
}

var expressionTypes = map[string]bool{
	"math_number": true, "math_arithmetic": true, "math_single": true,
	"math_trig": true, "math_constant": true, "math_number_property": true,
	"math_round": true, "math_on_list": true, "math_modulo": true, "math_constrain": true,
	"math_random_int": true, "math_random_float": true,
	"logic_boolean": true, "logic_compare": true, "logic_operation": true,
	"logic_negate": true, "logic_null": true, "logic_ternary": true,
	"variables_get": true, "text": true, "text_join": true,
	"text_length": true, "text_isEmpty": true, "text_indexOf": true,
	"text_charAt": true, "text_getSubstring": true,
	"text_changeCase": true, "text_trim": true,
	"lists_create_with": true, "lists_repeat": true, "lists_length": true,
	"lists_isEmpty": true, "lists_indexOf": true, "lists_getIndex": true,
	"lists_getSublist": true, "lists_split": true, "lists_sort": true,
	"procedures_callreturn": true,
	"sensor_keypressed":     true, "sensor_x": true, "sensor_z": true,
	"sensor_altitude": true, "sensor_direction": true, "sensor_speed": true,
}

// Parse validates a Blockly workspace and locates its entry point and procedures.
func Parse(data []byte) (*Program, error) {
	var document xmlDocument
	if err := xml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("XML Blockly non valido: %w", err)
	}
	if document.XMLName.Local != "xml" {
		return nil, errors.New("il file non contiene un workspace Blockly <xml>")
	}

	p := &Program{Roots: document.Blocks, Procedures: make(map[string]*Block)}
	var unsupported []string
	starts := 0
	var walk func(*Block)
	walk = func(block *Block) {
		if block == nil {
			return
		}
		if block.IsDisabled() {
			walk(block.Next.Selected())
			return
		}
		p.Summary.Blocks++
		if actionTypes[block.Type] {
			p.Summary.Commands++
		} else if !statementTypes[block.Type] && !expressionTypes[block.Type] {
			unsupported = append(unsupported, block.Type)
		}
		if block.Type == "start_block" {
			starts++
			p.Start = block
		}
		if block.Type == "procedures_defnoreturn" || block.Type == "procedures_defreturn" {
			name := block.Field("NAME").Value
			if name == "" {
				name = block.MutationAttr("name")
			}
			if name != "" {
				p.Procedures[name] = block
			}
		}
		for _, value := range block.Values {
			walk(value.Selected())
		}
		for _, statement := range block.Statements {
			walk(statement.Selected())
		}
		walk(block.Next.Selected())
	}
	for _, root := range p.Roots {
		walk(root)
	}
	if len(unsupported) > 0 {
		return nil, fmt.Errorf("blocchi non supportati dal driver: %s", strings.Join(unique(unsupported), ", "))
	}
	if starts == 0 {
		return nil, errors.New("manca il blocco Inizio (start_block)")
	}
	if starts > 1 {
		return nil, errors.New("il programma contiene piu di un blocco Inizio")
	}
	if p.Summary.Commands == 0 {
		p.Summary.Warnings = append(p.Summary.Warnings, "Il programma non contiene comandi per il drone.")
	}
	if !containsAction(p.Start, "land") {
		p.Summary.Warnings = append(p.Summary.Warnings, "Il programma non contiene Atterra; lascia attivo l'atterraggio automatico.")
	}
	return p, nil
}

func unique(items []string) []string {
	seen := make(map[string]bool)
	result := make([]string, 0, len(items))
	for _, item := range items {
		if item != "" && !seen[item] {
			seen[item] = true
			result = append(result, item)
		}
	}
	return result
}

func containsAction(block *Block, kind string) bool {
	for block != nil {
		if block.Type == kind {
			return true
		}
		for _, statement := range block.Statements {
			if containsAction(statement.Selected(), kind) {
				return true
			}
		}
		block = block.Next.Selected()
	}
	return false
}

// Value is the dynamically typed value used by Blockly expressions.
type Value any

// Host bridges the generic Blockly interpreter to a physical or simulated drone.
type Host interface {
	Action(context.Context, string, map[string]Value) error
	Sensor(name, argument string) Value
}

type Interpreter struct {
	Program  *Program
	Host     Host
	MaxSteps int
	steps    int
	vars     map[string]Value
	depth    int
}

var errBreak = errors.New("break")
var errContinue = errors.New("continue")

type returnSignal struct{ value Value }

func (r returnSignal) Error() string { return "return" }

// Execute runs the chain connected to the unique Start block.
func (i *Interpreter) Execute(ctx context.Context) error {
	if i.Program == nil || i.Program.Start == nil || i.Host == nil {
		return errors.New("interprete non inizializzato")
	}
	if i.MaxSteps <= 0 {
		i.MaxSteps = 10000
	}
	i.steps = 0
	i.vars = make(map[string]Value)
	return i.execChain(ctx, i.Program.Start.Next.Selected())
}

func (i *Interpreter) tick(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	i.steps++
	if i.steps > i.MaxSteps {
		return fmt.Errorf("limite di %d blocchi eseguiti superato", i.MaxSteps)
	}
	return nil
}

func (i *Interpreter) execChain(ctx context.Context, block *Block) error {
	for block != nil {
		if block.IsDisabled() {
			block = block.Next.Selected()
			continue
		}
		if err := i.tick(ctx); err != nil {
			return err
		}
		if err := i.execBlock(ctx, block); err != nil {
			return err
		}
		block = block.Next.Selected()
	}
	return nil
}

func (i *Interpreter) execBlock(ctx context.Context, block *Block) error {
	if actionTypes[block.Type] {
		arguments := make(map[string]Value, len(block.Values))
		for _, slot := range block.Values {
			value, err := i.eval(ctx, slot.Selected())
			if err != nil {
				return fmt.Errorf("blocco %s, valore %s: %w", block.Type, slot.Name, err)
			}
			arguments[slot.Name] = value
		}
		return i.Host.Action(ctx, block.Type, arguments)
	}

	switch block.Type {
	case "start_block", "end_block", "procedures_defnoreturn", "procedures_defreturn":
		return nil
	case "variables_set":
		value, err := i.eval(ctx, block.Value("VALUE"))
		if err != nil {
			return err
		}
		i.vars[variableKey(block.Field("VAR"))] = value
		return nil
	case "math_change":
		change, err := i.eval(ctx, block.Value("DELTA"))
		if err != nil {
			return err
		}
		key := variableKey(block.Field("VAR"))
		i.vars[key] = number(i.vars[key]) + number(change)
		return nil
	case "text_append":
		addition, err := i.eval(ctx, block.Value("TEXT"))
		if err != nil {
			return err
		}
		key := variableKey(block.Field("VAR"))
		i.vars[key] = fmt.Sprint(i.vars[key]) + fmt.Sprint(addition)
		return nil
	case "text_print":
		text, err := i.eval(ctx, block.Value("TEXT"))
		if err != nil {
			return err
		}
		return i.Host.Action(ctx, "text_print", map[string]Value{"TEXT": text})
	case "lists_getIndex":
		if block.Field("MODE").Value != "REMOVE" {
			return fmt.Errorf("lists_getIndex usato come istruzione senza modalita REMOVE")
		}
		_, err := i.evalListGetIndex(ctx, block)
		return err
	case "lists_setIndex":
		return i.execListSetIndex(ctx, block)
	case "controls_if":
		branches := 1 + intAttr(block.MutationAttr("elseif"))
		for n := 0; n < branches; n++ {
			condition, err := i.eval(ctx, block.Value(fmt.Sprintf("IF%d", n)))
			if err != nil {
				return err
			}
			if boolean(condition) {
				return i.execChain(ctx, block.Statement(fmt.Sprintf("DO%d", n)))
			}
		}
		return i.execChain(ctx, block.Statement("ELSE"))
	case "controls_repeat_ext":
		value, err := i.eval(ctx, block.Value("TIMES"))
		if err != nil {
			return err
		}
		count := int(math.Floor(number(value)))
		if count < 0 {
			count = 0
		}
		for n := 0; n < count; n++ {
			err = i.execChain(ctx, block.Statement("DO"))
			if errors.Is(err, errBreak) {
				break
			}
			if errors.Is(err, errContinue) {
				continue
			}
			if err != nil {
				return err
			}
		}
		return nil
	case "controls_whileUntil":
		until := block.Field("MODE").Value == "UNTIL"
		for {
			condition, err := i.eval(ctx, block.Value("BOOL"))
			if err != nil {
				return err
			}
			if boolean(condition) == until {
				break
			}
			err = i.execChain(ctx, block.Statement("DO"))
			if errors.Is(err, errBreak) {
				break
			}
			if errors.Is(err, errContinue) {
				continue
			}
			if err != nil {
				return err
			}
		}
		return nil
	case "controls_for":
		from, err := i.eval(ctx, block.Value("FROM"))
		if err != nil {
			return err
		}
		to, err := i.eval(ctx, block.Value("TO"))
		if err != nil {
			return err
		}
		by, err := i.eval(ctx, block.Value("BY"))
		if err != nil {
			return err
		}
		start, end, step := number(from), number(to), number(by)
		if step == 0 {
			step = 1
		}
		if start > end && step > 0 {
			step = -step
		}
		key := variableKey(block.Field("VAR"))
		for value := start; (step > 0 && value <= end) || (step < 0 && value >= end); value += step {
			i.vars[key] = value
			err = i.execChain(ctx, block.Statement("DO"))
			if errors.Is(err, errBreak) {
				break
			}
			if errors.Is(err, errContinue) {
				continue
			}
			if err != nil {
				return err
			}
		}
		return nil
	case "controls_forEach":
		list, err := i.eval(ctx, block.Value("LIST"))
		if err != nil {
			return err
		}
		key := variableKey(block.Field("VAR"))
		for _, value := range listValue(list) {
			i.vars[key] = value
			err = i.execChain(ctx, block.Statement("DO"))
			if errors.Is(err, errBreak) {
				break
			}
			if errors.Is(err, errContinue) {
				continue
			}
			if err != nil {
				return err
			}
		}
		return nil
	case "controls_flow_statements":
		if block.Field("FLOW").Value == "CONTINUE" {
			return errContinue
		}
		return errBreak
	case "procedures_callnoreturn":
		_, err := i.callProcedure(ctx, block)
		return err
	case "procedures_ifreturn":
		condition, err := i.eval(ctx, block.Value("CONDITION"))
		if err != nil {
			return err
		}
		if boolean(condition) {
			value, err := i.eval(ctx, block.Value("VALUE"))
			if err != nil {
				return err
			}
			return returnSignal{value: value}
		}
		return nil
	default:
		return fmt.Errorf("blocco istruzione non supportato: %s", block.Type)
	}
}

func (i *Interpreter) callProcedure(ctx context.Context, call *Block) (Value, error) {
	name := call.MutationAttr("name")
	if name == "" {
		name = call.Field("NAME").Value
	}
	definition := i.Program.Procedures[name]
	if definition == nil {
		return nil, fmt.Errorf("procedura %q non trovata", name)
	}
	if i.depth >= 64 {
		return nil, errors.New("profondita massima delle procedure superata")
	}

	saved := i.vars
	var definitionArgs []MutationArg
	if definition.Mutation != nil {
		definitionArgs = definition.Mutation.Args
	}
	local := make(map[string]Value, len(saved)+len(definitionArgs))
	for key, value := range saved {
		local[key] = value
	}
	for index, argument := range definitionArgs {
		value, err := i.eval(ctx, call.Value(fmt.Sprintf("ARG%d", index)))
		if err != nil {
			return nil, err
		}
		key := argument.VarID
		if key == "" {
			key = argument.Name
		}
		local[key] = value
	}
	i.vars, i.depth = local, i.depth+1
	err := i.execChain(ctx, definition.Statement("STACK"))
	var result Value
	var returned returnSignal
	if errors.As(err, &returned) {
		result, err = returned.value, nil
	}
	if err == nil && definition.Type == "procedures_defreturn" {
		result, err = i.eval(ctx, definition.Value("RETURN"))
	}
	// Blockly variables are global; copy changes back except procedure arguments.
	argumentKeys := make(map[string]bool)
	if definition.Mutation != nil {
		for _, argument := range definition.Mutation.Args {
			key := argument.VarID
			if key == "" {
				key = argument.Name
			}
			argumentKeys[key] = true
		}
	}
	for key, value := range i.vars {
		if !argumentKeys[key] {
			saved[key] = value
		}
	}
	i.vars, i.depth = saved, i.depth-1
	return result, err
}

func (i *Interpreter) eval(ctx context.Context, block *Block) (Value, error) {
	if err := i.tick(ctx); err != nil {
		return nil, err
	}
	if block == nil || block.IsDisabled() {
		return float64(0), nil
	}
	value := func(name string) (Value, error) { return i.eval(ctx, block.Value(name)) }

	switch block.Type {
	case "math_number":
		result, err := strconv.ParseFloat(block.Field("NUM").Value, 64)
		if err != nil {
			return nil, fmt.Errorf("numero non valido %q", block.Field("NUM").Value)
		}
		return result, nil
	case "logic_boolean":
		return block.Field("BOOL").Value == "TRUE", nil
	case "logic_null":
		return nil, nil
	case "text":
		return block.Field("TEXT").Value, nil
	case "variables_get":
		return i.vars[variableKey(block.Field("VAR"))], nil
	case "sensor_keypressed":
		return i.Host.Sensor("key", block.Field("KEY").Value), nil
	case "sensor_x":
		return i.Host.Sensor("x", ""), nil
	case "sensor_z":
		return i.Host.Sensor("z", ""), nil
	case "sensor_altitude":
		return i.Host.Sensor("altitude", ""), nil
	case "sensor_direction":
		return i.Host.Sensor("direction", ""), nil
	case "sensor_speed":
		return i.Host.Sensor("speed", ""), nil
	case "math_arithmetic":
		a, err := value("A")
		if err != nil {
			return nil, err
		}
		b, err := value("B")
		if err != nil {
			return nil, err
		}
		switch block.Field("OP").Value {
		case "ADD":
			return number(a) + number(b), nil
		case "MINUS":
			return number(a) - number(b), nil
		case "MULTIPLY":
			return number(a) * number(b), nil
		case "DIVIDE":
			return number(a) / number(b), nil
		case "POWER":
			return math.Pow(number(a), number(b)), nil
		}
	case "math_single", "math_trig", "math_round":
		x, err := value("NUM")
		if err != nil {
			return nil, err
		}
		n := number(x)
		switch block.Field("OP").Value {
		case "ROOT":
			return math.Sqrt(n), nil
		case "ABS":
			return math.Abs(n), nil
		case "NEG":
			return -n, nil
		case "LN":
			return math.Log(n), nil
		case "LOG10":
			return math.Log10(n), nil
		case "EXP":
			return math.Exp(n), nil
		case "POW10":
			return math.Pow(10, n), nil
		case "SIN":
			return math.Sin(n * math.Pi / 180), nil
		case "COS":
			return math.Cos(n * math.Pi / 180), nil
		case "TAN":
			return math.Tan(n * math.Pi / 180), nil
		case "ASIN":
			return math.Asin(n) * 180 / math.Pi, nil
		case "ACOS":
			return math.Acos(n) * 180 / math.Pi, nil
		case "ATAN":
			return math.Atan(n) * 180 / math.Pi, nil
		case "ROUND":
			return math.Round(n), nil
		case "ROUNDUP":
			return math.Ceil(n), nil
		case "ROUNDDOWN":
			return math.Floor(n), nil
		}
	case "math_constant":
		switch block.Field("CONSTANT").Value {
		case "PI":
			return math.Pi, nil
		case "E":
			return math.E, nil
		case "GOLDEN_RATIO":
			return (1 + math.Sqrt(5)) / 2, nil
		case "SQRT2":
			return math.Sqrt2, nil
		case "SQRT1_2":
			return math.Sqrt(0.5), nil
		case "INFINITY":
			return math.Inf(1), nil
		}
	case "math_modulo":
		a, err := value("DIVIDEND")
		if err != nil {
			return nil, err
		}
		b, err := value("DIVISOR")
		if err != nil {
			return nil, err
		}
		return math.Mod(number(a), number(b)), nil
	case "math_constrain":
		x, err := value("VALUE")
		if err != nil {
			return nil, err
		}
		low, err := value("LOW")
		if err != nil {
			return nil, err
		}
		high, err := value("HIGH")
		if err != nil {
			return nil, err
		}
		return math.Max(number(low), math.Min(number(high), number(x))), nil
	case "math_random_int":
		a, err := value("FROM")
		if err != nil {
			return nil, err
		}
		b, err := value("TO")
		if err != nil {
			return nil, err
		}
		low, high := int(math.Ceil(number(a))), int(math.Floor(number(b)))
		if low > high {
			low, high = high, low
		}
		return float64(low + rand.Intn(high-low+1)), nil
	case "math_random_float":
		return rand.Float64(), nil
	case "math_on_list":
		list, err := value("LIST")
		if err != nil {
			return nil, err
		}
		return mathOnList(block.Field("OP").Value, listValue(list)), nil
	case "logic_compare":
		a, err := value("A")
		if err != nil {
			return nil, err
		}
		b, err := value("B")
		if err != nil {
			return nil, err
		}
		op := block.Field("OP").Value
		if op == "EQ" {
			return fmt.Sprint(a) == fmt.Sprint(b), nil
		}
		if op == "NEQ" {
			return fmt.Sprint(a) != fmt.Sprint(b), nil
		}
		x, y := number(a), number(b)
		switch op {
		case "LT":
			return x < y, nil
		case "LTE":
			return x <= y, nil
		case "GT":
			return x > y, nil
		case "GTE":
			return x >= y, nil
		}
	case "logic_operation":
		a, err := value("A")
		if err != nil {
			return nil, err
		}
		if block.Field("OP").Value == "AND" && !boolean(a) {
			return false, nil
		}
		if block.Field("OP").Value == "OR" && boolean(a) {
			return true, nil
		}
		b, err := value("B")
		if err != nil {
			return nil, err
		}
		if block.Field("OP").Value == "AND" {
			return boolean(a) && boolean(b), nil
		}
		return boolean(a) || boolean(b), nil
	case "logic_negate":
		x, err := value("BOOL")
		return !boolean(x), err
	case "logic_ternary":
		condition, err := value("IF")
		if err != nil {
			return nil, err
		}
		if boolean(condition) {
			return value("THEN")
		}
		return value("ELSE")
	case "text_join":
		count := intAttr(block.MutationAttr("items"))
		var builder strings.Builder
		for n := 0; n < count; n++ {
			part, err := value(fmt.Sprintf("ADD%d", n))
			if err != nil {
				return nil, err
			}
			builder.WriteString(fmt.Sprint(part))
		}
		return builder.String(), nil
	case "text_length":
		x, err := value("VALUE")
		return float64(len([]rune(fmt.Sprint(x)))), err
	case "text_isEmpty":
		x, err := value("VALUE")
		return len(fmt.Sprint(x)) == 0, err
	case "text_indexOf":
		text, err := value("VALUE")
		if err != nil {
			return nil, err
		}
		find, err := value("FIND")
		if err != nil {
			return nil, err
		}
		return float64(textIndexOf(fmt.Sprint(text), fmt.Sprint(find), block.Field("END").Value)), nil
	case "text_charAt":
		text, err := value("VALUE")
		if err != nil {
			return nil, err
		}
		where := block.Field("WHERE").Value
		at, err := i.optionalIndex(ctx, block, "AT", where)
		if err != nil {
			return nil, err
		}
		runes := []rune(fmt.Sprint(text))
		index := blocklyIndex(where, at, len(runes), false)
		if index < 0 || index >= len(runes) {
			return "", nil
		}
		return string(runes[index]), nil
	case "text_getSubstring":
		text, err := value("STRING")
		if err != nil {
			return nil, err
		}
		runes := []rune(fmt.Sprint(text))
		startAt, err := i.optionalIndex(ctx, block, "AT1", block.Field("WHERE1").Value)
		if err != nil {
			return nil, err
		}
		endAt, err := i.optionalIndex(ctx, block, "AT2", block.Field("WHERE2").Value)
		if err != nil {
			return nil, err
		}
		start := blocklyIndex(block.Field("WHERE1").Value, startAt, len(runes), false)
		end := blocklyIndex(block.Field("WHERE2").Value, endAt, len(runes), false)
		start, end = boundedRange(start, end, len(runes))
		if start >= end {
			return "", nil
		}
		return string(runes[start:end]), nil
	case "text_changeCase":
		text, err := value("TEXT")
		if err != nil {
			return nil, err
		}
		switch block.Field("CASE").Value {
		case "UPPERCASE":
			return strings.ToUpper(fmt.Sprint(text)), nil
		case "LOWERCASE":
			return strings.ToLower(fmt.Sprint(text)), nil
		case "TITLECASE":
			return titleCase(fmt.Sprint(text)), nil
		}
	case "text_trim":
		text, err := value("TEXT")
		if err != nil {
			return nil, err
		}
		switch block.Field("MODE").Value {
		case "LEFT":
			return strings.TrimLeftFunc(fmt.Sprint(text), unicode.IsSpace), nil
		case "RIGHT":
			return strings.TrimRightFunc(fmt.Sprint(text), unicode.IsSpace), nil
		default:
			return strings.TrimSpace(fmt.Sprint(text)), nil
		}
	case "lists_create_with":
		count := intAttr(block.MutationAttr("items"))
		items := make([]Value, 0, count)
		for n := 0; n < count; n++ {
			item, err := value(fmt.Sprintf("ADD%d", n))
			if err != nil {
				return nil, err
			}
			items = append(items, item)
		}
		return items, nil
	case "lists_repeat":
		item, err := value("ITEM")
		if err != nil {
			return nil, err
		}
		count, err := value("NUM")
		if err != nil {
			return nil, err
		}
		items := make([]Value, max(0, int(number(count))))
		for n := range items {
			items[n] = item
		}
		return items, nil
	case "lists_length":
		collection, err := value("VALUE")
		return float64(collectionLength(collection)), err
	case "lists_isEmpty":
		collection, err := value("VALUE")
		return collectionLength(collection) == 0, err
	case "lists_indexOf":
		list, err := value("VALUE")
		if err != nil {
			return nil, err
		}
		find, err := value("FIND")
		if err != nil {
			return nil, err
		}
		items := listValue(list)
		index := -1
		if block.Field("END").Value == "LAST" {
			for n := len(items) - 1; n >= 0; n-- {
				if reflect.DeepEqual(items[n], find) {
					index = n
					break
				}
			}
		} else {
			for n, item := range items {
				if reflect.DeepEqual(item, find) {
					index = n
					break
				}
			}
		}
		return float64(index + 1), nil
	case "lists_getIndex":
		return i.evalListGetIndex(ctx, block)
	case "lists_getSublist":
		list, err := value("LIST")
		if err != nil {
			return nil, err
		}
		items := listValue(list)
		startAt, err := i.optionalIndex(ctx, block, "AT1", block.Field("WHERE1").Value)
		if err != nil {
			return nil, err
		}
		endAt, err := i.optionalIndex(ctx, block, "AT2", block.Field("WHERE2").Value)
		if err != nil {
			return nil, err
		}
		start := blocklyIndex(block.Field("WHERE1").Value, startAt, len(items), false)
		end := blocklyIndex(block.Field("WHERE2").Value, endAt, len(items), false)
		start, end = boundedRange(start, end, len(items))
		if start >= end {
			return []Value{}, nil
		}
		return append([]Value(nil), items[start:end]...), nil
	case "lists_split":
		input, err := value("INPUT")
		if err != nil {
			return nil, err
		}
		delimiter, err := value("DELIM")
		if err != nil {
			return nil, err
		}
		separator := fmt.Sprint(delimiter)
		if block.Field("MODE").Value == "JOIN" {
			items := listValue(input)
			parts := make([]string, len(items))
			for n, item := range items {
				parts[n] = fmt.Sprint(item)
			}
			return strings.Join(parts, separator), nil
		}
		parts := strings.Split(fmt.Sprint(input), separator)
		items := make([]Value, len(parts))
		for n, part := range parts {
			items[n] = part
		}
		return items, nil
	case "lists_sort":
		list, err := value("LIST")
		if err != nil {
			return nil, err
		}
		return sortList(listValue(list), block.Field("TYPE").Value, block.Field("DIRECTION").Value), nil
	case "math_number_property":
		x, err := value("NUMBER_TO_CHECK")
		if err != nil {
			return nil, err
		}
		n := number(x)
		switch block.Field("PROPERTY").Value {
		case "EVEN":
			return math.Mod(n, 2) == 0, nil
		case "ODD":
			return math.Mod(n, 2) != 0, nil
		case "WHOLE":
			return n == math.Trunc(n), nil
		case "POSITIVE":
			return n > 0, nil
		case "NEGATIVE":
			return n < 0, nil
		case "DIVISIBLE_BY":
			d, err := value("DIVISOR")
			return math.Mod(n, number(d)) == 0, err
		case "PRIME":
			return isPrime(n), nil
		}
	case "procedures_callreturn":
		return i.callProcedure(ctx, block)
	}
	return nil, fmt.Errorf("espressione non supportata: %s", block.Type)
}

func (i *Interpreter) optionalIndex(ctx context.Context, block *Block, input, where string) (float64, error) {
	if where != "FROM_START" && where != "FROM_END" {
		return 0, nil
	}
	value, err := i.eval(ctx, block.Value(input))
	return number(value), err
}

func (i *Interpreter) evalListGetIndex(ctx context.Context, block *Block) (Value, error) {
	source := block.Value("VALUE")
	value, err := i.eval(ctx, source)
	if err != nil {
		return nil, err
	}
	items := listValue(value)
	where := block.Field("WHERE").Value
	at, err := i.optionalIndex(ctx, block, "AT", where)
	if err != nil {
		return nil, err
	}
	index := blocklyIndex(where, at, len(items), false)
	if index < 0 || index >= len(items) {
		return nil, nil
	}
	result := items[index]
	mode := block.Field("MODE").Value
	if mode == "GET_REMOVE" || mode == "REMOVE" {
		items = append(items[:index], items[index+1:]...)
		i.storeList(source, items)
	}
	if mode == "REMOVE" {
		return nil, nil
	}
	return result, nil
}

func (i *Interpreter) execListSetIndex(ctx context.Context, block *Block) error {
	source := block.Value("LIST")
	value, err := i.eval(ctx, source)
	if err != nil {
		return err
	}
	item, err := i.eval(ctx, block.Value("TO"))
	if err != nil {
		return err
	}
	items := listValue(value)
	where := block.Field("WHERE").Value
	at, err := i.optionalIndex(ctx, block, "AT", where)
	if err != nil {
		return err
	}
	insert := block.Field("MODE").Value == "INSERT"
	index := blocklyIndex(where, at, len(items), insert)
	if insert {
		index = max(0, min(len(items), index))
		items = append(items, nil)
		copy(items[index+1:], items[index:])
		items[index] = item
	} else {
		if index < 0 {
			return nil
		}
		if index >= len(items) {
			items = append(items, make([]Value, index-len(items)+1)...)
		}
		items[index] = item
	}
	i.storeList(source, items)
	return nil
}

func (i *Interpreter) storeList(source *Block, items []Value) {
	if source != nil && source.Type == "variables_get" {
		i.vars[variableKey(source.Field("VAR"))] = items
	}
}

func blocklyIndex(where string, at float64, length int, insertion bool) int {
	switch where {
	case "FIRST":
		return 0
	case "LAST":
		if insertion {
			return length
		}
		return length - 1
	case "FROM_END":
		return length - int(math.Floor(at))
	case "RANDOM":
		if length == 0 {
			return 0
		}
		return rand.Intn(length)
	default: // FROM_START
		return int(math.Floor(at)) - 1
	}
}

// boundedRange converts Blockly's inclusive start/end indexes to a bounded Go
// half-open slice range.
func boundedRange(start, end, length int) (int, int) {
	start = max(0, min(length, start))
	end = max(0, min(length, end+1))
	return start, end
}

func textIndexOf(text, find, end string) int {
	byteIndex := strings.Index(text, find)
	if end == "LAST" {
		byteIndex = strings.LastIndex(text, find)
	}
	if byteIndex < 0 {
		return 0
	}
	return len([]rune(text[:byteIndex])) + 1
}

func titleCase(value string) string {
	runes := []rune(strings.ToLower(value))
	start := true
	for index, r := range runes {
		if unicode.IsSpace(r) {
			start = true
			continue
		}
		if start {
			runes[index] = unicode.ToTitle(r)
			start = false
		}
	}
	return string(runes)
}

func mathOnList(operation string, items []Value) Value {
	numbers := make([]float64, len(items))
	for index, item := range items {
		numbers[index] = number(item)
	}
	switch operation {
	case "SUM":
		return sum(numbers)
	case "MIN":
		if len(numbers) == 0 {
			return math.Inf(1)
		}
		return slicesMin(numbers)
	case "MAX":
		if len(numbers) == 0 {
			return math.Inf(-1)
		}
		return slicesMax(numbers)
	case "AVERAGE":
		if len(numbers) == 0 {
			return float64(0)
		}
		return sum(numbers) / float64(len(numbers))
	case "MEDIAN":
		if len(numbers) == 0 {
			return float64(0)
		}
		ordered := append([]float64(nil), numbers...)
		sort.Float64s(ordered)
		middle := len(ordered) / 2
		if len(ordered)%2 == 1 {
			return ordered[middle]
		}
		return (ordered[middle-1] + ordered[middle]) / 2
	case "MODE":
		counts := make(map[float64]int)
		most := 0
		for _, value := range numbers {
			counts[value]++
			most = max(most, counts[value])
		}
		modes := make([]Value, 0)
		seen := make(map[float64]bool)
		for _, value := range numbers {
			if counts[value] == most && !seen[value] {
				modes = append(modes, value)
				seen[value] = true
			}
		}
		return modes
	case "STD_DEV":
		if len(numbers) == 0 {
			return float64(0)
		}
		mean := sum(numbers) / float64(len(numbers))
		variance := 0.0
		for _, value := range numbers {
			variance += (value - mean) * (value - mean)
		}
		return math.Sqrt(variance / float64(len(numbers)))
	case "RANDOM":
		if len(items) == 0 {
			return nil
		}
		return items[rand.Intn(len(items))]
	}
	return float64(0)
}

func sum(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total
}

func slicesMin(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		result = math.Min(result, value)
	}
	return result
}

func slicesMax(values []float64) float64 {
	result := values[0]
	for _, value := range values[1:] {
		result = math.Max(result, value)
	}
	return result
}

func sortList(items []Value, kind, direction string) []Value {
	result := append([]Value(nil), items...)
	descending := direction == "-1"
	sort.SliceStable(result, func(left, right int) bool {
		comparison := 0
		switch kind {
		case "NUMERIC":
			comparison = compare(number(result[left]), number(result[right]))
		case "IGNORE_CASE":
			comparison = strings.Compare(strings.ToLower(fmt.Sprint(result[left])), strings.ToLower(fmt.Sprint(result[right])))
		default:
			comparison = strings.Compare(fmt.Sprint(result[left]), fmt.Sprint(result[right]))
		}
		if descending {
			return comparison > 0
		}
		return comparison < 0
	})
	return result
}

func compare(left, right float64) int {
	if left < right {
		return -1
	}
	if left > right {
		return 1
	}
	return 0
}

func variableKey(field Field) string {
	if field.ID != "" {
		return field.ID
	}
	return field.Value
}
func intAttr(value string) int { result, _ := strconv.Atoi(value); return result }
func number(value Value) float64 {
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case bool:
		if v {
			return 1
		}
		return 0
	case string:
		n, _ := strconv.ParseFloat(v, 64)
		return n
	}
	return 0
}
func boolean(value Value) bool {
	switch v := value.(type) {
	case bool:
		return v
	case nil:
		return false
	case float64:
		return v != 0 && !math.IsNaN(v)
	case string:
		return v != "" && v != "0" && v != "false"
	}
	return true
}
func listValue(value Value) []Value {
	if list, ok := value.([]Value); ok {
		return list
	}
	return nil
}

func collectionLength(value Value) int {
	if text, ok := value.(string); ok {
		return len([]rune(text))
	}
	return len(listValue(value))
}
func isPrime(value float64) bool {
	n := int(value)
	if value != float64(n) || n < 2 {
		return false
	}
	for d := 2; d*d <= n; d++ {
		if n%d == 0 {
			return false
		}
	}
	return true
}

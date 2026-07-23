package desktopui

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"strings"
)

var errEmptyXML = errors.New("empty XML")

// formatDroneXML pretty-prints a Blockly workspace without changing the text
// stored in fields. Blockly exports use a default namespace; element namespace
// metadata is cleared before encoding so it is not repeated on every line.
func formatDroneXML(data []byte) ([]byte, error) {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	var output bytes.Buffer
	encoder := xml.NewEncoder(&output)
	encoder.Indent("", "  ")
	parents := make([]string, 0, 16)

	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		switch value := token.(type) {
		case xml.StartElement:
			// Keep the original xmlns attribute on the root. Clearing Space
			// prevents encoding/xml from adding xmlns to every descendant.
			value.Name.Space = ""
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}
			parents = append(parents, value.Name.Local)
		case xml.EndElement:
			value.Name.Space = ""
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}
			if len(parents) != 0 {
				parents = parents[:len(parents)-1]
			}
		case xml.CharData:
			if strings.TrimSpace(string(value)) == "" && !preserveXMLWhitespace(parents) {
				continue
			}
			if err := encoder.EncodeToken(value); err != nil {
				return nil, err
			}
		default:
			if err := encoder.EncodeToken(token); err != nil {
				return nil, err
			}
		}
	}
	if err := encoder.Flush(); err != nil {
		return nil, err
	}
	formatted := bytes.TrimSpace(output.Bytes())
	if len(formatted) == 0 {
		return nil, errEmptyXML
	}
	return append(formatted, '\n'), nil
}

func preserveXMLWhitespace(parents []string) bool {
	if len(parents) == 0 {
		return false
	}
	switch parents[len(parents)-1] {
	case "field", "comment":
		return true
	default:
		return false
	}
}

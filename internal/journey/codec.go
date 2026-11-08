package journey

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"strings"
)

// xmlJourney is the on-disk shape (see examples/journeys/login.journey.xml). Tags live
// here, not on the domain type.
type xmlJourney struct {
	XMLName     xml.Name `xml:"journey"`
	Name        string   `xml:"name,attr"`
	Description string   `xml:"description"`
	Actions     []string `xml:"actions>action"`
}

// Decode parses a journey document.
func Decode(data []byte) (Journey, error) {
	var x xmlJourney
	if err := xml.Unmarshal(data, &x); err != nil {
		return Journey{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	acts := make([]Action, len(x.Actions))
	for i, a := range x.Actions {
		acts[i] = Action(a)
	}
	return New(x.Name, x.Description, acts)
}

// Encode renders a journey in the canonical layout.
func Encode(j Journey) []byte {
	var b bytes.Buffer
	fmt.Fprintf(&b, "<journey name=\"%s\">\n", escAttr(j.name))
	fmt.Fprintf(&b, "    <description>%s</description>\n", esc(j.description))
	b.WriteString("    <actions>\n")
	for _, a := range j.actions {
		fmt.Fprintf(&b, "        <action>%s</action>\n", esc(string(a)))
	}
	b.WriteString("    </actions>\n</journey>\n")
	return b.Bytes()
}

// escAttr escapes a double-quoted attribute value.
func escAttr(s string) string { return strings.ReplaceAll(esc(s), `"`, "&quot;") }

// esc escapes element text. Quotes stay literal so journeys remain readable.
func esc(s string) string {
	var b bytes.Buffer
	for _, r := range s {
		switch r {
		case '&':
			b.WriteString("&amp;")
		case '<':
			b.WriteString("&lt;")
		case '>':
			b.WriteString("&gt;")
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Load reads and decodes a journey file.
func Load(path string) (Journey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Journey{}, err
	}
	j, err := Decode(data)
	if err != nil {
		return Journey{}, fmt.Errorf("%s: %w", path, err)
	}
	return j, nil
}

// Save encodes j to path.
func Save(path string, j Journey) error {
	return os.WriteFile(path, Encode(j), 0o644)
}

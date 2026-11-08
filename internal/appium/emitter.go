package appium

import (
	"fmt"
	"sort"
	"strings"
)

// Emitter renders a Script as source code in one language.
type Emitter interface {
	// FileName is the output file name for the script.
	FileName(s Script) string
	Render(s Script) []byte
}

// emitters is the language registry. To add a language, implement Emitter in
// <lang>.go and register it here.
var emitters = map[string]Emitter{
	"java": javaEmitter{},
}

// Lookup returns the emitter for lang.
func Lookup(lang string) (Emitter, error) {
	if e, ok := emitters[strings.ToLower(lang)]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("unknown appium language %q (supported: %s)", lang, strings.Join(Languages(), ", "))
}

// Languages lists the supported languages.
func Languages() []string {
	var out []string
	for k := range emitters {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

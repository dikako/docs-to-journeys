// Package appium turns the steps an agent performed on a device into Appium
// test code. The agent records each step as language-neutral data (action,
// input, element locator); emitters render that data as code for one
// language. Adding a language means adding one emitter, not changing what the
// agent records. This context does not run anything on a device.
package appium

import (
	"regexp"
	"strings"
)

// Action is what a step does to an element.
type Action string

const (
	Tap    Action = "tap"
	Type   Action = "type"
	Verify Action = "verify"
	Scroll Action = "scroll"
	Swipe  Action = "swipe"
	Back   Action = "back"
	Wait   Action = "wait"
	Other  Action = "other"
)

var knownActions = map[Action]bool{Tap: true, Type: true, Verify: true, Scroll: true, Swipe: true, Back: true, Wait: true, Other: true}

// Strategy is an Appium locator strategy, in preference order.
type Strategy string

const (
	AccessibilityID    Strategy = "accessibilityId"
	ID                 Strategy = "id"
	AndroidUIAutomator Strategy = "androidUIAutomator"
	XPath              Strategy = "xpath"
)

var knownStrategies = map[Strategy]bool{AccessibilityID: true, ID: true, AndroidUIAutomator: true, XPath: true}

// Locator identifies one element.
type Locator struct {
	Name  string // UPPER_SNAKE_CASE, unique within a Script
	By    Strategy
	Value string
}

// IsZero reports whether the step has no element (e.g. back, wait).
func (l Locator) IsZero() bool { return l.Value == "" }

// Step is one executed journey action.
type Step struct {
	Index   int    // 1-based index of the journey action
	Text    string // the journey action, verbatim
	Action  Action
	Input   string // text typed, for Type
	Locator Locator
}

// Script is everything needed to render test code for one journey.
type Script struct {
	Journey string // journey name, e.g. "Login with standard user"
	Slug    string // file-safe journey id, e.g. "login"
	Steps   []Step
	// Total is the number of actions in the journey.
	Total int
	// FailedAt is the 1-based failing action, or 0 if the run passed.
	FailedAt int
	// Kind names what the code replays: "Journey" (default) or "Session".
	Kind string
}

// ClassName is the type name an emitter should use, e.g. "LoginJourney".
func (s Script) ClassName() string {
	kind := s.Kind
	if kind == "" {
		kind = "Journey"
	}
	return PascalCase(s.Slug) + kind
}

// Locators returns the distinct locators in first-use order.
func (s Script) Locators() []Locator {
	seen := map[string]bool{}
	var out []Locator
	for _, st := range s.Steps {
		if st.Locator.IsZero() || seen[st.Locator.Name] {
			continue
		}
		seen[st.Locator.Name] = true
		out = append(out, st.Locator)
	}
	return out
}

var nonIdent = regexp.MustCompile(`[^A-Za-z0-9]+`)

// constName normalizes a name to UPPER_SNAKE_CASE that is a valid identifier
// in every supported language.
func constName(s string) string {
	s = strings.Trim(nonIdent.ReplaceAllString(strings.ToUpper(s), "_"), "_")
	if s == "" {
		s = "ELEMENT"
	}
	if s[0] >= '0' && s[0] <= '9' {
		s = "E_" + s
	}
	return s
}

// PascalCase turns "checkout-prd" or "Login with user" into "CheckoutPrd" / "LoginWithUser".
func PascalCase(s string) string {
	var b strings.Builder
	for _, w := range nonIdent.Split(s, -1) {
		if w == "" {
			continue
		}
		b.WriteString(strings.ToUpper(w[:1]) + w[1:])
	}
	out := b.String()
	if out == "" || (out[0] >= '0' && out[0] <= '9') {
		out = "J" + out
	}
	return out
}

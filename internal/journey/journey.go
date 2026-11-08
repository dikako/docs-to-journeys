// Package journey owns the journey model: an ordered list of natural-language
// UI actions that an agent executes on a device. It also owns the rule that
// decides whether a healed journey still tests the same thing.
// It does not run journeys or talk to devices; that is the runner context.
package journey

import (
	"errors"
	"regexp"
	"strings"
)

// assertionRe marks actions that check the app rather than drive it. Healing
// never rewrites these.
var assertionRe = regexp.MustCompile(`(?i)^\s*(verify|check|ensure|assert|confirm)\b`)

// Action is one step of a journey.
type Action string

// Text returns the action with whitespace normalized.
func (a Action) Text() string { return strings.Join(strings.Fields(string(a)), " ") }

// IsAssertion reports whether the action verifies app state.
func (a Action) IsAssertion() bool { return assertionRe.MatchString(string(a)) }

// Journey is a named, ordered list of actions.
type Journey struct {
	name        string
	description string
	actions     []Action
}

// ErrInvalid is returned for journeys that cannot be executed.
var ErrInvalid = errors.New("invalid journey")

// New validates and builds a journey.
func New(name, description string, actions []Action) (Journey, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Journey{}, errors.Join(ErrInvalid, errors.New("name is empty"))
	}
	clean := make([]Action, 0, len(actions))
	for _, a := range actions {
		if t := a.Text(); t != "" {
			clean = append(clean, Action(t))
		}
	}
	if len(clean) == 0 {
		return Journey{}, errors.Join(ErrInvalid, errors.New("no actions"))
	}
	return Journey{name: name, description: strings.TrimSpace(description), actions: clean}, nil
}

func (j Journey) Name() string        { return j.name }
func (j Journey) Description() string { return j.description }
func (j Journey) IsZero() bool        { return j.name == "" }

// Actions returns a copy of the actions.
func (j Journey) Actions() []Action { return append([]Action(nil), j.actions...) }

// Assertions returns the assertion actions in order.
func (j Journey) Assertions() []Action {
	var out []Action
	for _, a := range j.actions {
		if a.IsAssertion() {
			out = append(out, a)
		}
	}
	return out
}

// IndexOf finds the action matching text (whitespace and case-insensitive),
// trying an exact match first and then a prefix match. It returns -1 if none.
func (j Journey) IndexOf(text string) int {
	want := strings.ToLower(Action(text).Text())
	if want == "" {
		return -1
	}
	for i, a := range j.actions {
		if strings.ToLower(a.Text()) == want {
			return i
		}
	}
	for i, a := range j.actions {
		got := strings.ToLower(a.Text())
		if strings.HasPrefix(got, want) || strings.HasPrefix(want, got) {
			return i
		}
	}
	return -1
}

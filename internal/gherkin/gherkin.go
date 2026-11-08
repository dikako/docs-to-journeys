// Package gherkin turns a PRD into a Gherkin feature and a feature into
// executable journeys. Gherkin is the reviewable contract between the PRD and
// the device runs: Background and Scenario Outline expansion is deterministic
// (via the cucumber parser), and only step wording is delegated to Claude.
// It does not fetch PRDs (source context) or execute journeys (runner context).
package gherkin

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"strings"

	gh "github.com/cucumber/gherkin/go/v42"
	messages "github.com/cucumber/messages/go/v34"
)

// StepKind is the role of a step after Given/When/Then/And resolution.
type StepKind string

const (
	Context StepKind = "Given"
	Action  StepKind = "When"
	Outcome StepKind = "Then"
)

// Step is one expanded scenario step.
type Step struct {
	Kind StepKind
	Text string
	// Arg is a rendered data table or doc string, if any.
	Arg string
}

// Scenario is one runnable case: Background steps prepended, Outline rows
// substituted.
type Scenario struct {
	Feature string
	Name    string
	Slug    string
	Steps   []Step
}

// Outcomes counts Then steps, each of which must become an assertion.
func (s Scenario) Outcomes() int {
	n := 0
	for _, st := range s.Steps {
		if st.Kind == Outcome {
			n++
		}
	}
	return n
}

// Parse validates a feature and returns its expanded scenarios.
func Parse(src []byte, uri string) ([]Scenario, error) {
	doc, err := gh.ParseGherkinDocument(bytes.NewReader(src), (&messages.Incrementing{}).NewId)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", uri, err)
	}
	if doc.Feature == nil {
		return nil, fmt.Errorf("%s: no Feature", uri)
	}
	pickles := gh.Pickles(*doc, uri, (&messages.Incrementing{}).NewId)
	if len(pickles) == 0 {
		return nil, fmt.Errorf("%s: no scenarios", uri)
	}
	total := map[string]int{}
	for _, p := range pickles {
		total[slugify(p.Name)]++
	}
	seen := map[string]int{}
	out := make([]Scenario, 0, len(pickles))
	for _, p := range pickles {
		sc := Scenario{Feature: doc.Feature.Name, Name: p.Name, Slug: slugify(p.Name)}
		// Scenario Outline rows share a name; number every row.
		if total[sc.Slug] > 1 {
			seen[sc.Slug]++
			n := seen[sc.Slug]
			sc.Name = fmt.Sprintf("%s (%d)", p.Name, n)
			sc.Slug = fmt.Sprintf("%s-%d", sc.Slug, n)
		}
		for _, ps := range p.Steps {
			sc.Steps = append(sc.Steps, Step{Kind: kindOf(ps.Type), Text: ps.Text, Arg: renderArg(ps.Argument)})
		}
		out = append(out, sc)
	}
	return out, nil
}

// ParseFile reads and parses a feature file.
func ParseFile(path string) ([]Scenario, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(b, path)
}

func kindOf(t messages.PickleStepType) StepKind {
	switch t {
	case messages.PickleStepType_OUTCOME:
		return Outcome
	case messages.PickleStepType_ACTION:
		return Action
	default:
		return Context
	}
}

func renderArg(a *messages.PickleStepArgument) string {
	if a == nil {
		return ""
	}
	if a.DocString != nil {
		return a.DocString.Content
	}
	if a.DataTable != nil {
		var b strings.Builder
		for _, r := range a.DataTable.Rows {
			b.WriteString("|")
			for _, c := range r.Cells {
				b.WriteString(" " + c.Value + " |")
			}
			b.WriteString("\n")
		}
		return strings.TrimRight(b.String(), "\n")
	}
	return ""
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		s = "scenario"
	}
	return s
}

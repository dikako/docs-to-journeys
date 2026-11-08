// Package explore runs time-boxed, charter-based exploratory testing sessions:
// an agent explores the app on a device, logs findings as it goes, and the
// tool records every step it performs from the agent's live event stream, so
// the record is complete even when the time box ends the session.
// Findings are written in a format the gherkin context can turn into
// regression journeys. It does not run scripted journeys (runner context).
package explore

import (
	"errors"
	"regexp"
	"strings"
	"time"
)

// Charter is the mission of a session.
type Charter struct {
	text string
	slug string
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// SessionName turns a user-chosen name into a safe folder name.
func SessionName(name string) (string, error) {
	n := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(strings.TrimSpace(name)), "-"), "-")
	if n == "" {
		return "", errors.New("session name must contain letters or digits")
	}
	if len(n) > 40 { // keeps "<name>-findings" under the 60-char feature slug limit
		n = strings.Trim(n[:40], "-")
	}
	return n, nil
}

// NewCharter validates a charter.
func NewCharter(text string) (Charter, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return Charter{}, errors.New("charter is empty: pass -charter or -charter-file")
	}
	// Slug: whole words from the first line, at most 30 chars, so
	// "<stamp>-<slug>-findings" stays under the 60-char feature slug limit.
	first := strings.SplitN(text, "\n", 2)[0]
	var words []string
	for _, w := range strings.Fields(nonSlug.ReplaceAllString(strings.ToLower(first), " ")) {
		if len(strings.Join(append(words, w), "-")) > 30 {
			break
		}
		words = append(words, w)
	}
	slug := strings.Join(words, "-")
	if slug == "" {
		slug = "session"
	}
	return Charter{text: text, slug: slug}, nil
}

func (c Charter) Text() string { return c.text }
func (c Charter) Slug() string { return c.slug }

// Options tune one session.
type Options struct {
	Minutes int
	// Prompt is extra user guidance appended to the agent's instructions.
	Prompt string
	// NoReset keeps the app's current state instead of a fresh launch.
	NoReset bool
	// Name is a custom session folder name; default "<YYYYMMDD-HHMM>-<charter>".
	Name string
}

// StopReason says why a session ended.
type StopReason string

const (
	Completed StopReason = "completed"
	TimeBox   StopReason = "time box reached"
	Failed    StopReason = "error"
)

// Finding is one issue the agent logged.
type Finding struct {
	Title    string
	Severity string
}

// Session is the outcome of one exploratory run.
type Session struct {
	ID       string
	Dir      string
	Charter  Charter
	Started  time.Time
	Ended    time.Time
	Stop     StopReason
	Note     string
	Steps    []Step
	Findings []Finding
	// FindingsPath feeds `journeys gherkin` to create regression tests.
	FindingsPath string
	// Appium is the generated replay code, if steps were recorded.
	Appium string
}

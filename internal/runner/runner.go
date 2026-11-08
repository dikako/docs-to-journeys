// Package runner executes journeys on a device through a Claude agent and
// heals journeys whose steps don't match the real UI flow. Healing loops:
// heal → guard (journey.FlowPreserved) → re-run from a clean app, up to
// MaxHeal times. Assertions are never changed, so a healed pass still proves
// what the original journey meant to prove.
// It does not generate journeys (gherkin context) or own the journey rules
// (journey context).
package runner

import "time"

// Status is the outcome of one journey.
type Status string

const (
	Pass         Status = "PASS"
	Healed       Status = "HEALED"
	Cached       Status = "CACHED"
	Fail         Status = "FAIL"
	AppBug       Status = "APP_BUG"
	HealRejected Status = "HEAL_REJECTED"
	Timeout      Status = "TIMEOUT"
	Error        Status = "ERROR"
	NoReport     Status = "NO_REPORT"
	NoResult     Status = "NO_RESULT"
)

// Passed reports whether the journey counts as green.
func (s Status) Passed() bool { return s == Pass || s == Healed || s == Cached }

// Result is the outcome of one journey including its heal attempts.
type Result struct {
	Name    string
	Path    string
	Serial  string
	Status  Status
	Heals   int
	Elapsed time.Duration
	Note    string
	// Appium is the generated test code file, if any steps were recorded.
	Appium      string
	AppiumSteps int
	// CachedAt is when the cached pass was recorded (Status == Cached).
	CachedAt time.Time
}

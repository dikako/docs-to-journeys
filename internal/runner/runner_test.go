package runner

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"docs-to-journeys/internal/common/claude"
)

func TestParseReportFixture(t *testing.T) {
	rep, err := parseReport("testdata/checkout_report.md")
	if err != nil {
		t.Fatal(err)
	}
	st, ok := rep.failed()
	if !ok || st.Text != `Type "Dika" in the First Name field` {
		t.Fatalf("failed step = %+v %v", st, ok)
	}
	if !rep.anyPassed() || len(rep.steps) != 17 {
		t.Fatalf("want 17 steps with some passed, got %d", len(rep.steps))
	}
}

func TestAppBug(t *testing.T) {
	if r, ok := appBug("I looked.\nAPP_BUG: crash on checkout\n"); !ok || r != "crash on checkout" {
		t.Fatalf("got %q %v", r, ok)
	}
	if _, ok := appBug("HEALED: renamed"); ok {
		t.Fatal("false positive")
	}
}

// fakeDevice records launches.
type fakeDevice struct{ launches int }

func (d *fakeDevice) ClearAndLaunch(context.Context, string, string) error { d.launches++; return nil }
func (d *fakeDevice) Screencap(context.Context, string) error              { return nil }

// fakeAgent plays a script: each call is either a run (writes a report) or a
// heal (edits the journey), told apart by the prompt.
type fakeAgent struct {
	t       *testing.T
	runs    []string // report bodies, one per run call
	heals   []func(path string) string
	journey string
}

func (a *fakeAgent) Run(_ context.Context, p claude.Prompt) (claude.Result, error) {
	if strings.HasPrefix(p.Text, "Run the journey") {
		body := a.runs[0]
		a.runs = a.runs[1:]
		report := between(p.Text, "Write results to ", ":")
		if err := os.WriteFile(report, []byte(body), 0o644); err != nil {
			a.t.Fatal(err)
		}
		return claude.Result{}, nil
	}
	h := a.heals[0]
	a.heals = a.heals[1:]
	return claude.Result{Reply: h(a.journey)}, nil
}

func between(s, a, b string) string {
	i := strings.Index(s, a) + len(a)
	return s[i : i+strings.Index(s[i:], b)]
}

const orig = `<journey name="Checkout">
    <description>d</description>
    <actions>
        <action>Tap "LOGIN"</action>
        <action>Verify products shown</action>
        <action>Type "Dika" in the First Name field</action>
        <action>Type "QA" in the Last Name field</action>
        <action>Tap the "CONTINUE" button</action>
        <action>Verify overview shown</action>
    </actions>
</journey>
`

const failReport = "### Action: Tap \"LOGIN\" ✅\n### Action: Verify products shown ✅\n### Action: Type \"Dika\" in the First Name field ❌\n### Action: Type \"QA\" in the Last Name field\n"
const passReport = "### Action: a ✅\n### Action: b ✅\n"

func write(path, body string) string {
	_ = os.WriteFile(path, []byte(body), 0o644)
	return "HEALED: edited"
}

func setup(t *testing.T, a *fakeAgent, maxHeal int) (*RunnerService, string, *fakeDevice) {
	dir := t.TempDir()
	p := filepath.Join(dir, "checkout.journey.xml")
	_ = os.WriteFile(p, []byte(orig), 0o644)
	a.t, a.journey = t, p
	dev := &fakeDevice{}
	return NewRunnerService(Deps{Agent: a, Device: dev, Pkg: "p", ReportDir: filepath.Join(dir, "reports"), MaxHeal: maxHeal}), p, dev
}

func goodHeal(path string) string {
	return write(path, strings.NewReplacer(
		`<action>Type "Dika" in the First Name field</action>`, `<action>Type "Dika" in the Full Name field</action>`,
		`<action>Type "QA" in the Last Name field</action>`, `<action>Type "Main St 1" in the Address Line 1 field</action>`,
		`<action>Tap the "CONTINUE" button</action>`, `<action>Tap the "To Payment" button</action>`,
	).Replace(orig))
}

func TestHealThenPass(t *testing.T) {
	a := &fakeAgent{runs: []string{failReport, passReport}, heals: []func(string) string{goodHeal}}
	s, p, dev := setup(t, a, 3)
	res, _ := s.Run(context.Background(), []string{p})
	if res[0].Status != Healed || res[0].Heals != 1 || dev.launches != 2 {
		t.Fatalf("got %+v launches=%d", res[0], dev.launches)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "Full Name") {
		t.Fatal("healed journey not kept")
	}
	diff, _ := os.ReadFile(filepath.Join(filepath.Dir(p), "reports", "checkout.heal.diff"))
	if !strings.Contains(string(diff), `+        <action>Tap the "To Payment" button</action>`) {
		t.Fatalf("diff missing change:\n%s", diff)
	}
}

func TestHealLoopsUntilExhausted(t *testing.T) {
	a := &fakeAgent{
		runs: []string{failReport, failReport, failReport},
		heals: []func(string) string{goodHeal, func(p string) string {
			b, _ := os.ReadFile(p)
			return write(p, strings.Replace(string(b), "Main St 1", "Main St 2", 1))
		}},
	}
	s, p, _ := setup(t, a, 2)
	res, _ := s.Run(context.Background(), []string{p})
	if res[0].Status != Fail || res[0].Heals != 2 {
		t.Fatalf("got %+v", res[0])
	}
}

func TestHealRejectedWhenAssertionChanges(t *testing.T) {
	bad := func(p string) string {
		return write(p, strings.Replace(orig, "Verify overview shown", "Verify something easier", 1))
	}
	a := &fakeAgent{runs: []string{failReport}, heals: []func(string) string{bad}}
	s, p, _ := setup(t, a, 3)
	res, _ := s.Run(context.Background(), []string{p})
	if res[0].Status != HealRejected {
		t.Fatalf("got %+v", res[0])
	}
	if b, _ := os.ReadFile(p); string(b) != orig {
		t.Fatal("journey not restored")
	}
}

func TestHealRejectedWhenPassedPrefixChanges(t *testing.T) {
	bad := func(p string) string { return write(p, strings.Replace(orig, `Tap "LOGIN"`, `Tap "Login"`, 1)) }
	a := &fakeAgent{runs: []string{failReport}, heals: []func(string) string{bad}}
	s, p, _ := setup(t, a, 3)
	if res, _ := s.Run(context.Background(), []string{p}); res[0].Status != HealRejected {
		t.Fatalf("got %+v", res[0])
	}
}

func TestAppBugStopsHealing(t *testing.T) {
	a := &fakeAgent{runs: []string{failReport}, heals: []func(string) string{func(string) string { return "APP_BUG: checkout crashes" }}}
	s, p, _ := setup(t, a, 3)
	res, _ := s.Run(context.Background(), []string{p})
	if res[0].Status != AppBug || res[0].Note != "checkout crashes" {
		t.Fatalf("got %+v", res[0])
	}
}

func TestPassNoHeal(t *testing.T) {
	a := &fakeAgent{runs: []string{passReport}}
	s, p, _ := setup(t, a, 3)
	if res, _ := s.Run(context.Background(), []string{p}); res[0].Status != Pass || res[0].Heals != 0 {
		t.Fatalf("got %+v", res[0])
	}
}

func TestRerunVerdictRerunsWithoutEdit(t *testing.T) {
	a := &fakeAgent{runs: []string{failReport, passReport}, heals: []func(string) string{func(string) string { return "RERUN: only letter case differs" }}}
	s, p, dev := setup(t, a, 3)
	res, _ := s.Run(context.Background(), []string{p})
	if res[0].Status != Healed || res[0].Heals != 1 || dev.launches != 2 {
		t.Fatalf("got %+v launches=%d", res[0], dev.launches)
	}
	if b, _ := os.ReadFile(p); string(b) != orig {
		t.Fatal("RERUN must not change the journey")
	}
}

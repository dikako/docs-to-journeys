package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"docs-to-journeys/internal/common/claude"
	"docs-to-journeys/internal/common/proc"
	"docs-to-journeys/internal/journey"
)

// agent is the slice of the Claude client the runner needs.
type agent interface {
	Run(ctx context.Context, p claude.Prompt) (claude.Result, error)
}

// device is the slice of adb the runner needs.
type device interface {
	ClearAndLaunch(ctx context.Context, pkg, activity string) error
	Screencap(ctx context.Context, path string) error
}

var (
	runTools  = []string{"Bash(android:*)", "Bash(adb:*)", "Read", "Write"}
	healTools = []string{"Bash(android:*)", "Bash(adb:*)", "Read", "Edit"}
)

// Deps are the RunnerService's collaborators.
type Deps struct {
	Agent    agent
	Device   device
	Serial   string
	Pkg      string
	Activity string
	// ReportDir receives <name>.md, .log, .locators.java, .png, heal files.
	ReportDir string
	// MaxHeal is how many heal → re-run cycles a failing journey gets.
	MaxHeal int
	Out     io.Writer
}

// RunnerService runs and heals journeys.
type RunnerService struct{ d Deps }

// NewRunnerService builds the service.
func NewRunnerService(d Deps) *RunnerService {
	if d.Out == nil {
		d.Out = io.Discard
	}
	return &RunnerService{d: d}
}

const sep = "-----"

// Run executes each journey, healing failures, and returns one result each.
func (s *RunnerService) Run(ctx context.Context, paths []string) ([]Result, error) {
	if err := os.MkdirAll(s.d.ReportDir, 0o755); err != nil {
		return nil, err
	}
	var results []Result
	for _, p := range paths {
		if ctx.Err() != nil {
			break
		}
		r := s.runJourney(ctx, p)
		s.printLocators(r)
		fmt.Fprintf(s.d.Out, "  %s (%s)\n", r.Status, r.Elapsed.Round(time.Second))
		results = append(results, r)
	}
	return results, ctx.Err()
}

type files struct{ report, log, locators, png, diff string }

func (s *RunnerService) files(name string) files {
	b := filepath.Join(s.d.ReportDir, name)
	return files{report: b + ".md", log: b + ".log", locators: b + ".locators.java", png: b + ".png", diff: b + ".heal.diff"}
}

func (s *RunnerService) clean(name string) {
	f := s.files(name)
	for _, p := range []string{f.report, f.log, f.locators, f.png, f.diff} {
		_ = os.Remove(p)
	}
	old, _ := filepath.Glob(filepath.Join(s.d.ReportDir, name+".heal.*.log"))
	for _, p := range old {
		_ = os.Remove(p)
	}
}

func (s *RunnerService) runJourney(ctx context.Context, path string) Result {
	name := strings.TrimSuffix(filepath.Base(path), ".journey.xml")
	f := s.files(name)
	s.clean(name)
	fmt.Fprintf(s.d.Out, "%s %s %s\n", sep, name, sep)
	start := time.Now()
	r := Result{Name: name, Path: path, Locators: f.locators}

	original, err := os.ReadFile(path)
	if err != nil {
		r.Status, r.Note = Error, err.Error()
		return r
	}
	if _, err := journey.Decode(original); err != nil {
		r.Status, r.Note = Error, err.Error()
		return r
	}

	r.Status, r.Note = s.runOnce(ctx, path, f)
	for r.Status == Fail && r.Heals < s.d.MaxHeal && ctx.Err() == nil {
		healed, status, note := s.heal(ctx, path, f, r.Heals+1)
		if !healed {
			if status != "" {
				r.Status = status
			}
			r.Note = note
			break
		}
		r.Heals++
		fmt.Fprintf(s.d.Out, "  heal %d applied, re-running: %s\n", r.Heals, note)
		r.Status, r.Note = s.runOnce(ctx, path, f)
	}

	if r.Heals > 0 {
		s.writeDiff(ctx, original, path, f.diff)
		if r.Status == Pass {
			r.Status = Healed
		}
		fmt.Fprintf(s.d.Out, "  healed diff: %s\n", f.diff)
	}
	if r.Note != "" {
		fmt.Fprintf(s.d.Out, "  note: %s\n", r.Note)
	}
	r.Elapsed = time.Since(start)
	return r
}

// runOnce resets the app and runs the journey once.
func (s *RunnerService) runOnce(ctx context.Context, path string, f files) (Status, string) {
	_ = os.Remove(f.report)
	if err := s.d.Device.ClearAndLaunch(ctx, s.d.Pkg, s.d.Activity); err != nil {
		return Error, err.Error()
	}
	res, err := s.d.Agent.Run(ctx, claude.Prompt{
		Text:  runPrompt(path, s.d.Serial, s.d.Pkg, f.report, f.locators),
		Tools: runTools,
		Log:   f.log,
	})
	st, note := classify(res, err, f.report)
	if st != Pass {
		_ = s.d.Device.Screencap(ctx, f.png)
	}
	return st, note
}

func classify(res claude.Result, err error, reportPath string) (Status, string) {
	switch {
	case err != nil:
		return Error, err.Error()
	case res.TimedOut:
		return Timeout, ""
	case res.ExitCode != 0:
		return Error, fmt.Sprintf("agent exit code %d", res.ExitCode)
	}
	rep, rerr := parseReport(reportPath)
	if rerr != nil {
		return NoReport, ""
	}
	if st, ok := rep.failed(); ok {
		return Fail, "failed at: " + st.Text
	}
	if rep.anyPassed() {
		return Pass, ""
	}
	return NoResult, ""
}

// heal asks the agent to fix the journey and guards the result. healed is true
// when the journey changed and still tests the same thing.
func (s *RunnerService) heal(ctx context.Context, path string, f files, attempt int) (healed bool, status Status, note string) {
	before, err := os.ReadFile(path)
	if err != nil {
		return false, Error, err.Error()
	}
	orig, _ := journey.Decode(before)
	rep, _ := parseReport(f.report)
	failStep, _ := rep.failed()
	failIdx := orig.IndexOf(failStep.Text)
	locked := max(failIdx, 0)

	res, err := s.d.Agent.Run(ctx, claude.Prompt{
		Text:  healPrompt(path, s.d.Serial, s.d.Pkg, f.report, failStep.Text, failIdx, attempt, s.d.MaxHeal),
		Tools: healTools,
		Log:   filepath.Join(s.d.ReportDir, fmt.Sprintf("%s.heal.%d.log", strings.TrimSuffix(filepath.Base(path), ".journey.xml"), attempt)),
	})
	restore := func() { _ = os.WriteFile(path, before, 0o644) }
	if err != nil {
		restore()
		return false, "", "heal error: " + err.Error()
	}
	if reason, ok := appBug(res.Reply); ok {
		restore()
		return false, AppBug, reason
	}
	after, err := os.ReadFile(path)
	if err != nil {
		restore()
		return false, "", "heal left no journey: " + err.Error()
	}
	if bytes.Equal(before, after) {
		if reason, ok := rerun(res.Reply); ok {
			// Nothing to fix; spend this attempt on a clean re-run.
			return true, "", "re-run without changes: " + reason
		}
		return false, "", fmt.Sprintf("heal %d made no change (timeout=%v exit=%d)", attempt, res.TimedOut, res.ExitCode)
	}
	healedJ, err := journey.Decode(after)
	if err != nil {
		restore()
		return false, HealRejected, "healed journey is invalid: " + err.Error()
	}
	if v := journey.FlowPreserved(orig, healedJ, locked); !v.OK {
		restore()
		return false, HealRejected, "heal rejected, restored: " + v.Reason
	}
	// Normalize formatting so diffs stay readable.
	_ = journey.Save(path, healedJ)
	return true, "", firstLine(res.Reply)
}

func (s *RunnerService) writeDiff(ctx context.Context, original []byte, path, out string) {
	tmp, err := os.CreateTemp("", "journey-orig-*.xml")
	if err != nil {
		return
	}
	defer os.Remove(tmp.Name())
	_, _ = tmp.Write(original)
	tmp.Close()
	res, _ := proc.Run(ctx, proc.Cmd{Name: "diff", Args: []string{"-u", "-L", "original/" + filepath.Base(path), "-L", "healed/" + filepath.Base(path), tmp.Name(), path}})
	_ = os.WriteFile(out, []byte(res.Stdout), 0o644)
}

func (s *RunnerService) printLocators(r Result) {
	fmt.Fprintf(s.d.Out, "  %s Appium locators: %s %s\n", sep, r.Name, sep)
	b, err := os.ReadFile(r.Locators)
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		fmt.Fprintln(s.d.Out, "  (none recorded)")
	} else {
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			fmt.Fprintln(s.d.Out, "  "+l)
		}
	}
	fmt.Fprintf(s.d.Out, "  %s\n", sep)
}

// Summary prints the result table and returns how many did not pass.
func Summary(w io.Writer, header string, results []Result) int {
	failed := 0
	fmt.Fprintf(w, "\n%s %s %s\n", sep, header, sep)
	for _, r := range results {
		if !r.Status.Passed() {
			failed++
		}
		fmt.Fprintf(w, "%-14s %6s  heals=%d  %s\n", r.Status, r.Elapsed.Round(time.Second), r.Heals, r.Name)
	}
	fmt.Fprintln(w, sep)
	fmt.Fprintf(w, "%d run, %d not passed\n", len(results), failed)
	return failed
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}

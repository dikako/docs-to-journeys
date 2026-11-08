package runner

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dikako/docs-to-journeys/internal/common/claude"
	"github.com/dikako/docs-to-journeys/internal/common/proc"
	"github.com/dikako/docs-to-journeys/internal/journey"
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

// Lane is one prepared device. Journeys are spread across lanes in parallel.
type Lane struct {
	Serial   string
	Device   device
	Activity string
	// Env identifies what a pass depends on besides the journey itself:
	// app version, device model, OS version. It is part of the cache key.
	Env string
}

// Deps are the RunnerService's collaborators.
type Deps struct {
	Agent agent
	Lanes []Lane
	Pkg   string
	// Model is recorded in the cache key; a different model re-runs journeys.
	Model string
	// ReportDir receives <name>.md, .log, .locators.java, .png, heal files.
	ReportDir string
	// MaxHeal is how many heal → re-run cycles a failing journey gets.
	MaxHeal int
	// NoCache re-runs journeys even when an identical pass is cached.
	NoCache bool
	Out     io.Writer
}

// RunnerService runs and heals journeys.
type RunnerService struct {
	d     Deps
	cache resultCache
	outMu sync.Mutex
}

// NewRunnerService builds the service.
func NewRunnerService(d Deps) *RunnerService {
	if d.Out == nil {
		d.Out = io.Discard
	}
	return &RunnerService{d: d, cache: resultCache{dir: filepath.Join(d.ReportDir, ".cache")}}
}

const sep = "-----"

// Run executes the journeys across all lanes and returns results in input
// order. Journeys whose identical run already passed are skipped as CACHED.
func (s *RunnerService) Run(ctx context.Context, paths []string) ([]Result, error) {
	if len(s.d.Lanes) == 0 {
		return nil, fmt.Errorf("no device lanes")
	}
	if err := uniqueNames(paths); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(s.d.ReportDir, 0o755); err != nil {
		return nil, err
	}

	results := make([]Result, len(paths))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for _, lane := range s.d.Lanes {
		wg.Add(1)
		go func(l Lane) {
			defer wg.Done()
			for i := range jobs {
				results[i] = s.runJob(ctx, l, paths[i])
			}
		}(lane)
	}
feed:
	for i := range paths {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	out := results[:0]
	for _, r := range results {
		if r.Name != "" { // empty = never started (cancelled)
			out = append(out, r)
		}
	}
	return out, ctx.Err()
}

// runJob runs one journey on a lane. With one lane, progress streams live;
// with several, each journey's output is printed as one block when done.
func (s *RunnerService) runJob(ctx context.Context, l Lane, path string) Result {
	var buf bytes.Buffer
	w := io.Writer(&buf)
	if len(s.d.Lanes) == 1 {
		w = lockedWriter{mu: &s.outMu, w: s.d.Out}
	}
	r := s.runJourney(ctx, l, path, w)
	s.printLocators(w, r)
	suffix := ""
	if r.Status == Cached {
		suffix = ", passed " + r.CachedAt.Format("2006-01-02 15:04")
	}
	fmt.Fprintf(w, "  %s (%s%s)\n", r.Status, r.Elapsed.Round(time.Second), suffix)
	if buf.Len() > 0 {
		s.outMu.Lock()
		_, _ = s.d.Out.Write(buf.Bytes())
		s.outMu.Unlock()
	}
	return r
}

type lockedWriter struct {
	mu *sync.Mutex
	w  io.Writer
}

func (l lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

// uniqueNames rejects journeys that would share report files.
func uniqueNames(paths []string) error {
	seen := map[string]string{}
	for _, p := range paths {
		n := journeyName(p)
		if prev, ok := seen[n]; ok {
			return fmt.Errorf("journeys %s and %s would share reports/%s.*; rename one", prev, p, n)
		}
		seen[n] = p
	}
	return nil
}

func journeyName(path string) string { return strings.TrimSuffix(filepath.Base(path), ".journey.xml") }

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

func (s *RunnerService) runJourney(ctx context.Context, l Lane, path string, out io.Writer) Result {
	name := journeyName(path)
	f := s.files(name)
	r := Result{Name: name, Path: path, Serial: l.Serial, Locators: f.locators}
	title := name
	if len(s.d.Lanes) > 1 {
		title += " [" + l.Serial + "]"
	}
	fmt.Fprintf(out, "%s %s %s\n", sep, title, sep)

	original, err := os.ReadFile(path)
	if err != nil {
		r.Status, r.Note = Error, err.Error()
		return r
	}
	if _, err := journey.Decode(original); err != nil {
		r.Status, r.Note = Error, err.Error()
		return r
	}

	key := s.cacheKey(original, l)
	if !s.d.NoCache {
		if e, ok := s.cache.lookup(name, key, f.report); ok {
			r.Status, r.Heals, r.CachedAt = Cached, e.Heals, e.At
			return r
		}
	}

	s.clean(name)
	start := time.Now()
	r.Status, r.Note = s.runOnce(ctx, l, path, f)
	for r.Status == Fail && r.Heals < s.d.MaxHeal && ctx.Err() == nil {
		healed, status, note := s.heal(ctx, l, path, f, r.Heals+1)
		if !healed {
			if status != "" {
				r.Status = status
			}
			r.Note = note
			break
		}
		r.Heals++
		fmt.Fprintf(out, "  heal %d applied, re-running: %s\n", r.Heals, note)
		r.Status, r.Note = s.runOnce(ctx, l, path, f)
	}

	if r.Heals > 0 {
		s.writeDiff(ctx, original, path, f.diff)
		if r.Status == Pass {
			r.Status = Healed
		}
		fmt.Fprintf(out, "  healed diff: %s\n", f.diff)
	}
	if r.Note != "" {
		fmt.Fprintf(out, "  note: %s\n", r.Note)
	}
	r.Elapsed = time.Since(start)

	if r.Status.Passed() {
		// Key the final file: a healed journey is cached as healed.
		if final, err := os.ReadFile(path); err == nil {
			s.cache.store(name, s.cacheKey(final, l), r)
		}
	} else {
		s.cache.forget(name)
	}
	return r
}

// runOnce resets the app and runs the journey once.
func (s *RunnerService) runOnce(ctx context.Context, l Lane, path string, f files) (Status, string) {
	_ = os.Remove(f.report)
	if err := l.Device.ClearAndLaunch(ctx, s.d.Pkg, l.Activity); err != nil {
		return Error, err.Error()
	}
	res, err := s.d.Agent.Run(ctx, claude.Prompt{
		Text:  runPrompt(path, l.Serial, s.d.Pkg, f.report, f.locators),
		Tools: runTools,
		Log:   f.log,
	})
	st, note := classify(res, err, f.report)
	if st != Pass {
		_ = l.Device.Screencap(ctx, f.png)
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
// when the journey should be re-run: it changed and still tests the same
// thing, or the agent judged the failure a misjudged check (RERUN).
func (s *RunnerService) heal(ctx context.Context, l Lane, path string, f files, attempt int) (healed bool, status Status, note string) {
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
		Text:  healPrompt(path, l.Serial, s.d.Pkg, f.report, failStep.Text, failIdx, attempt, s.d.MaxHeal),
		Tools: healTools,
		Log:   filepath.Join(s.d.ReportDir, fmt.Sprintf("%s.heal.%d.log", journeyName(path), attempt)),
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

func (s *RunnerService) printLocators(w io.Writer, r Result) {
	if r.Locators == "" {
		return
	}
	fmt.Fprintf(w, "  %s Appium locators: %s %s\n", sep, r.Name, sep)
	b, err := os.ReadFile(r.Locators)
	if err != nil || len(bytes.TrimSpace(b)) == 0 {
		fmt.Fprintln(w, "  (none recorded)")
	} else {
		for _, l := range strings.Split(strings.TrimRight(string(b), "\n"), "\n") {
			fmt.Fprintln(w, "  "+l)
		}
	}
	fmt.Fprintf(w, "  %s\n", sep)
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

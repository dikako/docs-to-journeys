package explore

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/dikako/docs-to-journeys/internal/appium"
	"github.com/dikako/docs-to-journeys/internal/common/claude"
)

// agent is the slice of the Claude client this context needs.
type agent interface {
	Run(ctx context.Context, p claude.Prompt) (claude.Result, error)
}

// device is the slice of adb this context needs.
type device interface {
	ClearAndLaunch(ctx context.Context, pkg, activity string) error
	Screencap(ctx context.Context, path string) error
}

var tools = []string{"Bash(android:*)", "Bash(adb:*)", "Bash(date:*)", "Read", "Write", "Edit"}

// Grace is how long the agent may run past the time box to finish its notes
// before it is stopped.
const Grace = 3 * time.Minute

// Deps are the ExploreService's collaborators.
type Deps struct {
	Agent    agent
	Device   device
	Serial   string
	Pkg      string
	Activity string
	// Env describes the app build and device, for the session report.
	Env string
	// SessionsDir is where session folders are created: <SessionsDir>/<id>/.
	SessionsDir string
	Appium      appium.Emitter
	Out         io.Writer
}

// ExploreService runs exploratory sessions.
type ExploreService struct{ d Deps }

// NewExploreService builds the service.
func NewExploreService(d Deps) *ExploreService {
	if d.Out == nil {
		d.Out = io.Discard
	}
	if d.Appium == nil {
		d.Appium, _ = appium.Lookup("java")
	}
	return &ExploreService{d: d}
}

// Run executes one session. The agent is stopped Grace after the time box;
// everything recorded up to then is kept and reported.
func (s *ExploreService) Run(ctx context.Context, c Charter, o Options) (Session, error) {
	if o.Minutes <= 0 {
		o.Minutes = 20
	}
	start := time.Now()
	id := start.Format("20060102-1504") + "-" + c.Slug()
	if o.Name != "" {
		n, err := SessionName(o.Name)
		if err != nil {
			return Session{}, err
		}
		id = n
	}
	dir := filepath.Join(s.d.SessionsDir, id)
	if _, err := os.Stat(dir); err == nil {
		return Session{}, fmt.Errorf("session folder %s already exists; choose another -name", dir)
	}
	if err := os.MkdirAll(filepath.Join(dir, "screenshots"), 0o755); err != nil {
		return Session{}, err
	}
	findings := filepath.Join(dir, id+"-findings.md")
	steps := filepath.Join(dir, "appium.steps.json")
	transcript := filepath.Join(dir, "transcript.jsonl")
	if err := os.WriteFile(findings, []byte(findingsHeader(c, s.d.Pkg)), 0o644); err != nil {
		return Session{}, err
	}

	if !o.NoReset {
		if err := s.d.Device.ClearAndLaunch(ctx, s.d.Pkg, s.d.Activity); err != nil {
			return Session{}, err
		}
	}

	deadline := start.Add(time.Duration(o.Minutes) * time.Minute)
	fmt.Fprintf(s.d.Out, "exploring %s on %s until %s (hard stop %s)\n  charter: %s\n  output:  %s\n",
		s.d.Pkg, s.d.Serial, deadline.Format("15:04:05"), deadline.Add(Grace).Format("15:04:05"), firstLine(c.Text()), dir)

	runCtx, cancel := context.WithDeadline(ctx, deadline.Add(Grace))
	defer cancel()
	res, err := s.d.Agent.Run(runCtx, claude.Prompt{
		Text:       explorePrompt(c, o, s.d.Serial, s.d.Pkg, deadline, dir, findings, steps),
		Tools:      tools,
		Log:        filepath.Join(dir, "agent.log"),
		Transcript: transcript,
	})

	sess := Session{ID: id, Dir: dir, Charter: c, Started: start, Ended: time.Now(), FindingsPath: findings}
	switch {
	case err != nil:
		sess.Stop, sess.Note = Failed, err.Error()
	case res.TimedOut || runCtx.Err() == context.DeadlineExceeded:
		sess.Stop, sess.Note = TimeBox, "stopped at the hard limit"
	case res.ExitCode != 0:
		sess.Stop, sess.Note = Failed, fmt.Sprintf("agent exit code %d", res.ExitCode)
	default:
		sess.Stop, sess.Note = Completed, firstLine(res.Reply)
	}

	// Fresh context: the run context may be past its deadline.
	capCtx, capCancel := context.WithTimeout(context.Background(), 30*time.Second)
	_ = s.d.Device.Screencap(capCtx, filepath.Join(dir, "screenshots", "final.png"))
	capCancel()

	sess.Steps, _ = parseTranscript(transcript, findings)
	sess.Findings = readFindings(findings)
	sess.Appium = s.renderAppium(dir, steps, c, id)

	if err := writeSteps(filepath.Join(dir, "steps.md"), sess); err != nil {
		return sess, err
	}
	return sess, writeSession(filepath.Join(dir, "session.md"), s.d.Env, o.Minutes, sess)
}

func (s *ExploreService) renderAppium(dir, stepsPath string, c Charter, id string) string {
	data, err := os.ReadFile(stepsPath)
	if err != nil {
		return ""
	}
	sc, err := appium.DecodeSession(data, firstLine(c.Text()), c.Slug())
	if err != nil || len(sc.Steps) == 0 {
		return ""
	}
	out := filepath.Join(dir, "appium", s.d.Appium.FileName(sc))
	if os.MkdirAll(filepath.Dir(out), 0o755) != nil || os.WriteFile(out, s.d.Appium.Render(sc), 0o644) != nil {
		return ""
	}
	return out
}

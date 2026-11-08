package explore

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dikako/docs-to-journeys/internal/common/claude"
)

func line(t time.Time, ev string) string {
	b, _ := json.Marshal(struct {
		T string          `json:"t"`
		E json.RawMessage `json:"e"`
	}{t.Format(time.RFC3339), json.RawMessage(ev)})
	return string(b) + "\n"
}

// transcriptFixture mimics claude stream-json: text, device commands, a
// non-device command, a failed command, and a findings write.
func transcriptFixture(findings string) string {
	t0 := time.Now()
	return line(t0, `{"type":"system","subtype":"init"}`) +
		line(t0, `{"type":"assistant","message":{"content":[{"type":"text","text":"Let me open the login screen first."},{"type":"tool_use","id":"a","name":"Bash","input":{"command":"android layout"}}]}}`) +
		line(t0.Add(5*time.Second), `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"b","name":"Bash","input":{"command":"adb -s emu shell input text 😀","description":"Type emoji into username"}}]}}`) +
		line(t0.Add(6*time.Second), `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"c","name":"Bash","input":{"command":"date +%H:%M"}}]}}`) +
		line(t0.Add(6*time.Second), `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"w","name":"Bash","input":{"command":"date; mkdir -p shots"}}]}}`) +
		line(t0.Add(7*time.Second), `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"d","name":"Bash","input":{"command":"adb -s emu shell input tap 9999 9999 | grep x","description":"Tap off-screen"}}]}}`) +
		line(t0.Add(8*time.Second), `{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"d","is_error":true,"content":"error"}]}}`) +
		line(t0.Add(9*time.Second), `{"type":"assistant","message":{"content":[{"type":"tool_use","id":"e","name":"Edit","input":{"file_path":"`+findings+`"}}]}}`) +
		`{"t":"broken`
}

func TestParseTranscript(t *testing.T) {
	dir := t.TempDir()
	tr := filepath.Join(dir, "t.jsonl")
	_ = os.WriteFile(tr, []byte(transcriptFixture("/x/s-findings.md")), 0o644)
	steps, err := parseTranscript(tr, "/x/s-findings.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(steps) != 4 {
		t.Fatalf("want 4 steps (3 commands + 1 note; bookkeeping skipped), got %d: %+v", len(steps), steps)
	}
	if steps[0].Intent != "Let me open the login screen first." || steps[1].Intent != "Type emoji into username" {
		t.Fatalf("intents: %q / %q", steps[0].Intent, steps[1].Intent)
	}
	if !steps[2].Failed || !steps[3].Note {
		t.Fatalf("failed/note flags: %+v", steps)
	}
}

func TestReadFindings(t *testing.T) {
	p := filepath.Join(t.TempDir(), "f.md")
	_ = os.WriteFile(p, []byte("# x\n\n## Finding 1: Emoji username accepted\n- Severity: High\n- Expected: error\n\n## Finding 2: Back loses cart\n- Steps to reproduce:\n  1. a\n"), 0o644)
	got := readFindings(p)
	if len(got) != 2 || got[0].Severity != "high" || got[1].Title != "Back loses cart" || got[1].Severity != "?" {
		t.Fatalf("got %+v", got)
	}
}

type fakeDevice struct{ launches, caps int }

func (d *fakeDevice) ClearAndLaunch(context.Context, string, string) error { d.launches++; return nil }
func (d *fakeDevice) Screencap(context.Context, string) error              { d.caps++; return nil }

// fakeAgent writes what a real session would, then reports res.
type fakeAgent struct {
	res    claude.Result
	prompt string
}

func (a *fakeAgent) Run(_ context.Context, p claude.Prompt) (claude.Result, error) {
	a.prompt = p.Text
	findings := between(p.Text, "immediately append it to ", " in exactly")
	steps := between(p.Text, "rewrite ", " as a JSON array")
	_ = os.WriteFile(p.Transcript, []byte(transcriptFixture(findings)), 0o644)
	f, _ := os.OpenFile(findings, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString("## Finding 1: Emoji username accepted\n- Severity: high\n\n## Finding 2: Back loses cart\n- Severity: medium\n")
	f.Close()
	_ = os.WriteFile(steps, []byte(`[{"action":"type","input":"😀","element":"USERNAME","by":"id","value":"a:id/u"},{"action":"back"}]`), 0o644)
	return a.res, nil
}

func between(s, a, b string) string {
	i := strings.Index(s, a) + len(a)
	return s[i : i+strings.Index(s[i:], b)]
}

func run(t *testing.T, res claude.Result, o Options) (Session, *fakeAgent, *fakeDevice) {
	t.Helper()
	c, err := NewCharter("Login with edge-case input: emoji, long text, back mid-flow\nmore detail")
	if err != nil {
		t.Fatal(err)
	}
	a, d := &fakeAgent{res: res}, &fakeDevice{}
	s := NewExploreService(Deps{Agent: a, Device: d, Serial: "emu", Pkg: "com.app", Env: "app=1", ReportDir: t.TempDir()})
	sess, err := s.Run(context.Background(), c, o)
	if err != nil {
		t.Fatal(err)
	}
	return sess, a, d
}

func TestSessionCompleted(t *testing.T) {
	sess, a, d := run(t, claude.Result{Reply: "SUMMARY: 2 findings"}, Options{Minutes: 5, Prompt: "Use bob@example.com"})
	if sess.Stop != Completed || len(sess.Findings) != 2 || len(sess.Steps) != 4 {
		t.Fatalf("got %+v", sess)
	}
	if d.launches != 1 || d.caps != 1 {
		t.Fatalf("launches=%d caps=%d", d.launches, d.caps)
	}
	if !strings.Contains(a.prompt, "TIME BOX: stop exploring at ") || !strings.Contains(a.prompt, "ADDITIONAL INSTRUCTIONS FROM THE TESTER:\nUse bob@example.com") {
		t.Fatal("prompt missing deadline or user prompt")
	}
	if !strings.HasSuffix(sess.ID, "-login-with-edge-case-input") {
		t.Fatalf("id %q", sess.ID)
	}
	md, _ := os.ReadFile(filepath.Join(sess.Dir, "session.md"))
	for _, want := range []string{"| Stopped | completed (SUMMARY: 2 findings) |", "| 1 | high | Emoji username accepted |", "journeys gherkin ", "| Device actions | 3 "} {
		if !strings.Contains(string(md), want) {
			t.Errorf("session.md missing %q:\n%s", want, md)
		}
	}
	steps, _ := os.ReadFile(filepath.Join(sess.Dir, "steps.md"))
	if !strings.Contains(string(steps), "Type emoji into username") || !strings.Contains(string(steps), "input tap 9999 9999 | grep x") {
		t.Errorf("steps.md:\n%s", steps)
	}
	if filepath.Base(sess.Appium) != "LoginWithEdgeCaseInputSession.java" {
		t.Fatalf("appium %q", sess.Appium)
	}
}

func TestSessionTimeBoxKeepsPartialResults(t *testing.T) {
	sess, _, _ := run(t, claude.Result{TimedOut: true, ExitCode: 124}, Options{Minutes: 1, NoReset: true})
	if sess.Stop != TimeBox || len(sess.Findings) != 2 || len(sess.Steps) == 0 {
		t.Fatalf("got %+v", sess)
	}
}

func TestNoResetSkipsLaunch(t *testing.T) {
	_, _, d := run(t, claude.Result{}, Options{Minutes: 1, NoReset: true})
	if d.launches != 0 {
		t.Fatal("NoReset must not relaunch the app")
	}
}

func TestNewCharterRejectsEmpty(t *testing.T) {
	if _, err := NewCharter("   "); err == nil {
		t.Fatal("want error")
	}
}

func TestHelperScriptCommandsAreRecorded(t *testing.T) {
	for cmd, want := range map[string]bool{
		"/tmp/a.sh tap 700 1086; /tmp/a.sh text wrongpass": true,
		"cd reports/x; android layout --flat | grep text":  true,
		"D=\"adb -s e shell\"; $D input tap 1 2":           true,
		"date; mkdir -p x; which adb":                      false,
		"date":                                             false,
	} {
		_, got := toStep("Bash", toolInput{Command: cmd}, "")
		if got != want {
			t.Errorf("%q: recorded=%v want %v", cmd, got, want)
		}
	}
}

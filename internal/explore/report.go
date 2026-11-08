package explore

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

var (
	findingRe  = regexp.MustCompile(`^##\s+Finding\s+\d+\s*[:.-]\s*(.+)$`)
	severityRe = regexp.MustCompile(`(?i)^\s*-?\s*severity\s*:\s*(\w+)`)
)

// readFindings parses findings.md written by the agent.
func readFindings(path string) []Finding {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []Finding
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := findingRe.FindStringSubmatch(line); m != nil {
			out = append(out, Finding{Title: strings.TrimSpace(m[1]), Severity: "?"})
			continue
		}
		if m := severityRe.FindStringSubmatch(line); m != nil && len(out) > 0 && out[len(out)-1].Severity == "?" {
			out[len(out)-1].Severity = strings.ToLower(m[1])
		}
	}
	return out
}

func writeSteps(path string, s Session) error {
	var b strings.Builder
	fmt.Fprintf(&b, "# Steps: %s\n\nEvery command the agent ran, in order, recorded live from the agent's event stream.\n", firstLine(s.Charter.Text()))
	if len(s.Steps) == 0 {
		b.WriteString("\n_No commands were recorded._\n")
		return os.WriteFile(path, []byte(b.String()), 0o644)
	}
	fence := "```"
	for i, st := range s.Steps {
		at := ""
		if !st.At.IsZero() {
			at = " · +" + st.At.Sub(s.Started).Truncate(time.Second).String()
		}
		if st.Note {
			fmt.Fprintf(&b, "\n### %d%s · 📝 %s\n", i+1, at, st.Command)
			continue
		}
		title := strings.Join(strings.Fields(st.Intent), " ")
		if title == "" {
			title = "(no description)"
		}
		if st.Failed {
			title += " ❌ failed"
		}
		cmd := strings.ReplaceAll(st.Command, fence, "` ` `")
		fmt.Fprintf(&b, "\n### %d%s · %s\n\n%ssh\n%s\n%s\n", i+1, at, title, fence, cmd, fence)
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

// cell makes text safe for a one-line markdown table cell.
func cell(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	s = strings.ReplaceAll(s, "|", `\|`)
	s = strings.ReplaceAll(s, "`", "'")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

func writeSession(path, env string, minutes int, s Session) error {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	device := 0
	for _, st := range s.Steps {
		if !st.Note {
			device++
		}
	}
	w("# Exploratory session: %s\n\n", firstLine(s.Charter.Text()))
	w("| | |\n|---|---|\n")
	w("| Charter | %s |\n", cell(s.Charter.Text(), 400))
	w("| Device | %s |\n", cell(env, 200))
	w("| Started | %s |\n", s.Started.Format("2006-01-02 15:04:05"))
	w("| Duration | %s of %d min time box |\n", s.Ended.Sub(s.Started).Round(time.Second), minutes)
	stop := string(s.Stop)
	if s.Note != "" {
		stop += " (" + cell(s.Note, 200) + ")"
	}
	w("| Stopped | %s |\n", stop)
	w("| Device actions | %d ([steps.md](steps.md)) |\n", device)
	w("| Findings | %d ([%s](%s)) |\n\n", len(s.Findings), filepath.Base(s.FindingsPath), filepath.Base(s.FindingsPath))

	w("## Findings\n\n")
	if len(s.Findings) == 0 {
		w("_None logged._\n\n")
	} else {
		w("| # | Severity | Title |\n|---|---|---|\n")
		for i, f := range s.Findings {
			w("| %d | %s | %s |\n", i+1, f.Severity, cell(f.Title, 200))
		}
		w("\n")
	}
	w("## Files\n\n")
	w("- `%s`: findings (steps to reproduce, expected vs actual, screenshot)\n", filepath.Base(s.FindingsPath))
	w("- `steps.md`: every device command, time-stamped\n")
	w("- `transcript.jsonl`: full agent event stream (audit trail)\n")
	w("- `screenshots/`: evidence per finding and the final screen\n")
	if s.Appium != "" {
		w("- `appium/%s`: the session as Appium code\n", filepath.Base(s.Appium))
	}
	if len(s.Findings) > 0 {
		w("\n## Turn findings into regression tests\n\n```bash\n")
		w("journeys gherkin %s\n", s.FindingsPath)
		w("journeys convert features/%s.feature\n", strings.TrimSuffix(filepath.Base(s.FindingsPath), ".md"))
		w("```\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}

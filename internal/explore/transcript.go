package explore

import (
	"bufio"
	"encoding/json"
	"os"
	"strings"
	"time"
)

// Step is one thing the agent did on the device.
type Step struct {
	At      time.Time
	Command string
	Intent  string
	Failed  bool
	// Note marks a non-device step, e.g. recording a finding.
	Note bool
}

type record struct {
	T string          `json:"t"`
	E json.RawMessage `json:"e"`
}

type event struct {
	Type    string `json:"type"`
	Message struct {
		Content []struct {
			Type      string          `json:"type"`
			Text      string          `json:"text"`
			ID        string          `json:"id"`
			Name      string          `json:"name"`
			Input     json.RawMessage `json:"input"`
			ToolUseID string          `json:"tool_use_id"`
			IsError   bool            `json:"is_error"`
		} `json:"content"`
	} `json:"message"`
}

type toolInput struct {
	Command     string `json:"command"`
	Description string `json:"description"`
	FilePath    string `json:"file_path"`
}

// parseTranscript extracts device steps from a transcript written by the
// claude client. Unreadable lines are skipped; a truncated file (killed run)
// yields the steps up to the cut.
func parseTranscript(path, findingsPath string) ([]Step, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var steps []Step
	byID := map[string]int{}
	lastText := ""
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), 64*1024*1024)
	for sc.Scan() {
		var r record
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		at, _ := time.Parse(time.RFC3339, r.T)
		var ev event
		if json.Unmarshal(r.E, &ev) != nil {
			continue
		}
		for _, c := range ev.Message.Content {
			switch {
			case c.Type == "text" && strings.TrimSpace(c.Text) != "":
				lastText = firstLine(c.Text)
			case c.Type == "tool_use":
				var in toolInput
				_ = json.Unmarshal(c.Input, &in)
				st, ok := toStep(c.Name, in, findingsPath)
				if !ok {
					continue
				}
				st.At = at
				if st.Intent == "" {
					st.Intent = lastText
				}
				byID[c.ID] = len(steps)
				steps = append(steps, st)
			case c.Type == "tool_result" && c.IsError:
				if i, ok := byID[c.ToolUseID]; ok {
					steps[i].Failed = true
				}
			}
		}
	}
	return steps, sc.Err()
}

// bookkeeping commands carry no test action; everything else is recorded.
var bookkeeping = map[string]bool{"date": true, "mkdir": true, "which": true, "ls": true, "pwd": true, "true": true}

func toStep(tool string, in toolInput, findingsPath string) (Step, bool) {
	switch tool {
	case "Bash":
		cmd := strings.TrimSpace(in.Command)
		if cmd == "" || onlyBookkeeping(cmd) {
			return Step{}, false
		}
		return Step{Command: cmd, Intent: in.Description}, true
	case "Write", "Edit":
		if in.FilePath != "" && findingsPath != "" && sameFile(in.FilePath, findingsPath) {
			return Step{Command: "recorded a finding in findings", Note: true}, true
		}
	}
	return Step{}, false
}

// onlyBookkeeping reports whether every command in a simple ;/&&-chain is a
// bookkeeping command (e.g. "date; mkdir -p x; which adb").
func onlyBookkeeping(cmd string) bool {
	if strings.ContainsAny(cmd, "\n|><$`") {
		return false
	}
	for _, part := range strings.FieldsFunc(strings.ReplaceAll(cmd, "&&", ";"), func(r rune) bool { return r == ';' }) {
		f := strings.Fields(part)
		if len(f) > 0 && !bookkeeping[f[0]] {
			return false
		}
	}
	return true
}

func sameFile(a, b string) bool {
	return strings.HasSuffix(a, strings.TrimPrefix(b, "./")) || strings.HasSuffix(b, a)
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 140 {
		s = s[:140] + "…"
	}
	return s
}

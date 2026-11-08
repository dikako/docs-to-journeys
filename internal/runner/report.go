package runner

import (
	"bufio"
	"os"
	"regexp"
	"strings"
)

var actionHeader = regexp.MustCompile(`^###\s+Action:\s*(.*?)\s*(✅|❌)?\s*$`)

// step is one "### Action:" header from the agent's markdown report.
type step struct {
	Text string
	Mark string // "✅", "❌", or "" for not run
}

type report struct{ steps []step }

func parseReport(path string) (report, error) {
	f, err := os.Open(path)
	if err != nil {
		return report{}, err
	}
	defer f.Close()
	var r report
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 1024*1024)
	for sc.Scan() {
		if m := actionHeader.FindStringSubmatch(strings.TrimSpace(sc.Text())); m != nil {
			r.steps = append(r.steps, step{Text: m[1], Mark: m[2]})
		}
	}
	return r, sc.Err()
}

// failed returns the first failed step.
func (r report) failed() (step, bool) {
	for _, s := range r.steps {
		if s.Mark == "❌" {
			return s, true
		}
	}
	return step{}, false
}

func (r report) anyPassed() bool {
	for _, s := range r.steps {
		if s.Mark == "✅" {
			return true
		}
	}
	return false
}

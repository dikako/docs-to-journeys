package gherkin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dikako/docs-to-journeys/internal/common/claude"
	"github.com/dikako/docs-to-journeys/internal/journey"
	"github.com/dikako/docs-to-journeys/internal/source"
)

// agent is the slice of the Claude client this context needs.
type agent interface {
	Run(ctx context.Context, p claude.Prompt) (claude.Result, error)
}

// Deps are the GherkinService's collaborators.
type Deps struct {
	Agent agent
	// Pkg is the Android application id under test.
	Pkg string
	// FeatureDir receives <slug>.feature.
	FeatureDir string
	// JourneyDir receives <feature-slug>/<scenario-slug>.journey.xml.
	JourneyDir string
	Force      bool
	// Retries is how many times a rejected agent answer is retried.
	Retries int
}

// GherkinService generates features and journeys.
type GherkinService struct{ d Deps }

// NewGherkinService builds the service.
func NewGherkinService(d Deps) *GherkinService {
	if d.Retries <= 0 {
		d.Retries = 1
	}
	return &GherkinService{d: d}
}

const hashPrefix = "# prd-sha256: "

// Generate writes <FeatureDir>/<slug>.feature from doc. If the feature already
// exists and was generated from the same PRD content, it is reused.
func (s *GherkinService) Generate(ctx context.Context, doc source.Document) (string, error) {
	if err := os.MkdirAll(s.d.FeatureDir, 0o755); err != nil {
		return "", err
	}
	out := filepath.Join(s.d.FeatureDir, doc.Slug+".feature")
	if !s.d.Force {
		if b, err := os.ReadFile(out); err == nil && strings.HasPrefix(string(b), hashPrefix+doc.Hash) {
			if _, err := Parse(b, out); err == nil {
				return out, nil
			}
		}
	}
	abs, _ := filepath.Abs(doc.Path)
	absOut, _ := filepath.Abs(out)
	prevErr := ""
	for attempt := 0; attempt <= s.d.Retries; attempt++ {
		_ = os.Remove(out)
		log := filepath.Join(filepath.Dir(doc.Path), fmt.Sprintf("gherkin.%d.log", attempt+1))
		res, err := s.d.Agent.Run(ctx, claude.Prompt{
			Text:  generatePrompt(abs, absOut, s.d.Pkg, prevErr),
			Tools: []string{"Read", "Write"},
			Log:   log,
		})
		if err != nil {
			return "", err
		}
		b, rerr := os.ReadFile(out)
		if rerr != nil || !res.OK() {
			prevErr = fmt.Sprintf("no feature file was written (exit=%d, timeout=%v)", res.ExitCode, res.TimedOut)
			continue
		}
		if _, perr := Parse(b, out); perr != nil {
			prevErr = perr.Error()
			continue
		}
		stamped := hashPrefix + doc.Hash + "\n# source: " + doc.Ref + "\n" + string(b)
		return out, os.WriteFile(out, []byte(stamped), 0o644)
	}
	return "", fmt.Errorf("gherkin generation failed for %s: %s", doc.Ref, prevErr)
}

// Converted reports what ToJourneys did with one scenario.
type Converted struct {
	Path    string
	Skipped bool
	Err     error
}

// ToJourneys writes one journey per expanded scenario of the feature. Existing
// journeys are kept unless Force is set, so healed journeys survive.
func (s *GherkinService) ToJourneys(ctx context.Context, featurePath string) ([]Converted, error) {
	scs, err := ParseFile(featurePath)
	if err != nil {
		return nil, err
	}
	slug := strings.TrimSuffix(filepath.Base(featurePath), filepath.Ext(featurePath))
	dir := filepath.Join(s.d.JourneyDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	var out []Converted
	for _, sc := range scs {
		path := filepath.Join(dir, sc.Slug+".journey.xml")
		if _, err := os.Stat(path); err == nil && !s.d.Force {
			out = append(out, Converted{Path: path, Skipped: true})
			continue
		}
		j, err := s.translate(ctx, sc, filepath.Join(dir, sc.Slug+".translate.log"))
		if err == nil {
			err = journey.Save(path, j)
		}
		out = append(out, Converted{Path: path, Err: err})
	}
	return out, nil
}

type translation struct {
	Description string   `json:"description"`
	Actions     []string `json:"actions"`
}

func (s *GherkinService) translate(ctx context.Context, sc Scenario, log string) (journey.Journey, error) {
	prevErr := ""
	for attempt := 0; attempt <= s.d.Retries; attempt++ {
		res, err := s.d.Agent.Run(ctx, claude.Prompt{Text: translatePrompt(sc, s.d.Pkg, prevErr), Log: log})
		if err != nil {
			return journey.Journey{}, err
		}
		if !res.OK() {
			prevErr = fmt.Sprintf("agent failed (exit=%d, timeout=%v)", res.ExitCode, res.TimedOut)
			continue
		}
		j, verr := decodeTranslation(sc, res.Reply)
		if verr == nil {
			return j, nil
		}
		prevErr = verr.Error()
	}
	return journey.Journey{}, fmt.Errorf("scenario %q: %s", sc.Name, prevErr)
}

// decodeTranslation parses the agent's JSON and checks that every Then step
// survived as exactly one assertion.
func decodeTranslation(sc Scenario, reply string) (journey.Journey, error) {
	start, end := strings.Index(reply, "{"), strings.LastIndex(reply, "}")
	if start < 0 || end <= start {
		return journey.Journey{}, errors.New("reply contains no JSON object")
	}
	var t translation
	if err := json.Unmarshal([]byte(reply[start:end+1]), &t); err != nil {
		return journey.Journey{}, fmt.Errorf("invalid JSON: %v", err)
	}
	acts := make([]journey.Action, len(t.Actions))
	for i, a := range t.Actions {
		acts[i] = journey.Action(a)
	}
	j, err := journey.New(sc.Name, t.Description, acts)
	if err != nil {
		return journey.Journey{}, err
	}
	if got, want := len(j.Assertions()), sc.Outcomes(); got != want {
		return journey.Journey{}, fmt.Errorf("scenario has %d Then steps but the journey has %d Verify actions", want, got)
	}
	return j, nil
}

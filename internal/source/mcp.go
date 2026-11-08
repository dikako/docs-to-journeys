package source

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"github.com/dikako/docs-to-journeys/internal/common/claude"
)

// agent is the slice of the Claude client this context needs.
type agent interface {
	Run(ctx context.Context, p claude.Prompt) (claude.Result, error)
}

// agentResolver asks Claude to read the reference with the kind's tools and
// write it as markdown.
type agentResolver struct {
	kind  Kind
	re    *regexp.Regexp
	agent agent
}

func (r agentResolver) Name() string          { return r.kind.Name }
func (r agentResolver) Match(ref string) bool { return r.re.MatchString(ref) }

func (r agentResolver) Fetch(ctx context.Context, ref, out, log string) error {
	if r.agent == nil {
		return errors.New("no agent configured")
	}
	_ = os.Remove(out)
	prompt := fmt.Sprintf(`Read the product requirements document at %s and write it to %s as clean markdown.
Keep every requirement, user flow, acceptance criterion, business rule, field name, button label, and example value verbatim.
Keep tables as markdown tables. Drop navigation, comments, and unrelated boilerplate. Do not summarize or invent anything.
%s
If you cannot access the document, do not write the file; reply with: ACCESS_ERROR: <reason>.`, ref, out, r.kind.Instructions)

	res, err := r.agent.Run(ctx, claude.Prompt{Text: prompt, Tools: r.kind.AllowedTools, Log: log})
	if err != nil {
		return err
	}
	if st, err := os.Stat(out); err != nil || st.Size() == 0 || !res.OK() {
		msg := fmt.Sprintf("%s source: could not fetch %s (exit=%d timeout=%v, log: %s)", r.kind.Name, ref, res.ExitCode, res.TimedOut, log)
		if r.kind.Hint != "" {
			msg += "\nhint: " + r.kind.Hint
		}
		return errors.New(msg)
	}
	return nil
}

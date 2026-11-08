// Package claude wraps the `claude -p` CLI as a headless agent. It is part of
// the shared kernel: it knows how to call Claude, not what to ask.
package claude

import (
	"context"
	"strings"
	"time"

	"docs-to-journeys/internal/common/proc"
)

// Prompt is one headless agent invocation.
type Prompt struct {
	Text string
	// Tools is the --allowedTools list. Empty means no tool is pre-approved.
	Tools []string
	// Log receives the agent's stdout and stderr.
	Log string
}

// Result is the agent's final reply and how the process ended.
type Result struct {
	Reply    string
	ExitCode int
	TimedOut bool
}

// OK reports whether the agent exited cleanly.
func (r Result) OK() bool { return r.ExitCode == 0 && !r.TimedOut }

// Client runs prompts with a fixed model and per-call timeout.
type Client struct {
	Bin     string
	Model   string
	Timeout time.Duration
}

// Run executes p and returns once the agent exits or the timeout passes.
func (c Client) Run(ctx context.Context, p Prompt) (Result, error) {
	if c.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, c.Timeout)
		defer cancel()
	}
	bin := c.Bin
	if bin == "" {
		bin = "claude"
	}
	args := []string{"-p", p.Text, "--output-format", "text"}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if len(p.Tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(p.Tools, ","))
	}
	res, err := proc.Run(ctx, proc.Cmd{Name: bin, Args: args, LogPath: p.Log})
	return Result{Reply: res.Stdout, ExitCode: res.ExitCode, TimedOut: res.TimedOut}, err
}

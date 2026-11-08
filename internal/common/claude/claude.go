// Package claude wraps the `claude -p` CLI as a headless agent. It is part of
// the shared kernel: it knows how to call Claude, not what to ask.
package claude

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/dikako/docs-to-journeys/internal/common/proc"
)

// Prompt is one headless agent invocation.
type Prompt struct {
	Text string
	// Tools is the --allowedTools list. Empty means no tool is pre-approved.
	Tools []string
	// Log receives the agent's stdout and stderr.
	Log string
	// Transcript, when set, receives every streamed agent event as it happens,
	// one JSON object per line: {"t": <RFC3339 time>, "e": <claude event>}.
	// Use it when every step must be kept, even if the run is killed.
	Transcript string
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
	args := []string{"-p", p.Text}
	if c.Model != "" {
		args = append(args, "--model", c.Model)
	}
	if len(p.Tools) > 0 {
		args = append(args, "--allowedTools", strings.Join(p.Tools, ","))
	}
	if p.Transcript != "" {
		return c.stream(ctx, bin, args, p)
	}
	args = append(args, "--output-format", "text")
	res, err := proc.Run(ctx, proc.Cmd{Name: bin, Args: args, LogPath: p.Log})
	return Result{Reply: res.Stdout, ExitCode: res.ExitCode, TimedOut: res.TimedOut}, err
}

// stream runs with --output-format stream-json, writing each event to the
// transcript with the time it arrived. The reply is the final result event.
func (c Client) stream(ctx context.Context, bin string, args []string, p Prompt) (Result, error) {
	f, err := os.Create(p.Transcript)
	if err != nil {
		return Result{}, err
	}
	defer f.Close()
	args = append(args, "--output-format", "stream-json", "--verbose")

	var reply string
	res, err := proc.Stream(ctx, proc.Cmd{Name: bin, Args: args, LogPath: p.Log}, func(line []byte) {
		if !json.Valid(line) {
			return
		}
		line = stripImages(line)
		_ = marshalLine(f, struct {
			T string          `json:"t"`
			E json.RawMessage `json:"e"`
		}{time.Now().Format(time.RFC3339), line})
		var ev struct {
			Type   string `json:"type"`
			Result string `json:"result"`
		}
		if json.Unmarshal(line, &ev) == nil && ev.Type == "result" {
			reply = ev.Result
		}
	})
	return Result{Reply: reply, ExitCode: res.ExitCode, TimedOut: res.TimedOut}, err
}

// stripImages replaces base64 image data in an event (screenshots the agent
// looked at) with a size note. A single screenshot is ~0.5-1 MB of base64,
// which would make long transcripts huge; the text of every step is kept.
func stripImages(line []byte) []byte {
	if !bytes.Contains(line, []byte(`"type":"image"`)) {
		return line
	}
	var v any
	if json.Unmarshal(line, &v) != nil {
		return line
	}
	var walk func(any)
	walk = func(n any) {
		switch t := n.(type) {
		case map[string]any:
			if t["type"] == "base64" {
				if d, ok := t["data"].(string); ok {
					t["data"] = fmt.Sprintf("<image omitted: %d bytes>", len(d))
				}
			}
			for _, c := range t {
				walk(c)
			}
		case []any:
			for _, c := range t {
				walk(c)
			}
		}
	}
	walk(v)
	var b bytes.Buffer
	if marshalLine(&b, v) != nil {
		return line
	}
	return bytes.TrimRight(b.Bytes(), "\n")
}

// marshalLine writes v as one JSON line without HTML escaping, so text such
// as "<image omitted>" or "a < b" stays readable.
func marshalLine(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

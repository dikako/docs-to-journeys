package claude

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// fakeClaude writes a script that prints a canned stream-json transcript.
func fakeClaude(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "claude")
	if err := os.WriteFile(p, []byte("#!/bin/sh\n"+body), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

const events = `{"type":"system","subtype":"init"}
{"type":"assistant","message":{"content":[{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"adb shell input tap 1 2","description":"Tap login"}}]}}
{"type":"user","message":{"content":[{"type":"tool_result","tool_use_id":"t1","content":"ok"}]}}
{"type":"result","subtype":"success","result":"DONE","is_error":false}
`

func TestStreamTranscript(t *testing.T) {
	bin := fakeClaude(t, "cat <<'EOF'\n"+events+"EOF\n")
	tr := filepath.Join(t.TempDir(), "t.jsonl")
	res, err := Client{Bin: bin}.Run(context.Background(), Prompt{Text: "x", Transcript: tr})
	if err != nil || !res.OK() || res.Reply != "DONE" {
		t.Fatalf("res=%+v err=%v", res, err)
	}
	b, _ := os.ReadFile(tr)
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 4 || !strings.HasPrefix(lines[1], `{"t":"`) || !strings.Contains(lines[1], `"adb shell input tap 1 2"`) {
		t.Fatalf("transcript:\n%s", b)
	}
}

func TestStreamKeepsTranscriptOnTimeout(t *testing.T) {
	bin := fakeClaude(t, "echo '{\"type\":\"system\",\"subtype\":\"init\"}'\nsleep 30\n")
	tr := filepath.Join(t.TempDir(), "t.jsonl")
	start := time.Now()
	res, _ := Client{Bin: bin, Timeout: 500 * time.Millisecond}.Run(context.Background(), Prompt{Text: "x", Transcript: tr})
	if !res.TimedOut {
		t.Fatalf("want timeout, got %+v", res)
	}
	if time.Since(start) > 10*time.Second {
		t.Fatal("timeout did not stop the run promptly")
	}
	if b, _ := os.ReadFile(tr); !strings.Contains(string(b), "init") {
		t.Fatal("events before the kill must be kept")
	}
}

func TestStripImages(t *testing.T) {
	in := []byte(`{"type":"user","message":{"content":[{"type":"tool_result","content":[{"type":"image","source":{"type":"base64","media_type":"image/png","data":"` + strings.Repeat("A", 5000) + `"}},{"type":"text","text":"keep me"}]}]}}`)
	out := string(stripImages(in))
	if strings.Contains(out, "AAAA") || !strings.Contains(out, "<image omitted: 5000 bytes>") || !strings.Contains(out, "keep me") {
		t.Fatalf("got %s", out)
	}
	plain := []byte(`{"type":"result","result":"DONE"}`)
	if string(stripImages(plain)) != string(plain) {
		t.Fatal("events without images must be untouched")
	}
}

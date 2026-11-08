package source

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"docs-to-journeys/internal/common/claude"
)

type fakeAgent struct{ calls int }

func (f *fakeAgent) Run(_ context.Context, p claude.Prompt) (claude.Result, error) {
	f.calls++
	return claude.Result{}, nil
}

func TestDetect(t *testing.T) {
	extra := []Kind{{Name: "gdocs", Match: `^https://docs\.google\.com/document/`, AllowedTools: []string{"Write"}}}
	s, err := NewSourceService(Deps{Kinds: extra, WorkDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ ref, want string }{
		{"prd.md", "text"},
		{"spec.TXT", "text"},
		{"prd.docx", "rich"},
		{"prd.pdf", "pdf"},
		{"https://www.notion.so/acme/Checkout-PRD-0123456789abcdef0123456789abcdef", "notion"},
		{"https://acme.notion.site/Checkout-0123456789abcdef0123456789abcdef", "notion"},
		{"https://acme.atlassian.net/wiki/spaces/QA/pages/12345/Checkout+PRD", "confluence"},
		{"https://docs.google.com/document/d/abc/edit", "gdocs"},
		{"https://example.com/prd", "web"},
	}
	for _, c := range cases {
		got, err := s.Detect(c.ref)
		if err != nil || got != c.want {
			t.Errorf("%s: got %q,%v want %q", c.ref, got, err, c.want)
		}
	}
	if _, err := s.Detect("prd.xyz"); err == nil {
		t.Error("want error for unknown kind")
	}
}

func TestSlug(t *testing.T) {
	cases := []struct{ ref, want string }{
		{"docs/Checkout PRD.md", "checkout-prd"},
		{"https://www.notion.so/acme/Checkout-PRD-0123456789abcdef0123456789abcdef", "checkout-prd"},
		{"https://acme.atlassian.net/wiki/spaces/QA/pages/12345", "pages-12345"},
	}
	for _, c := range cases {
		if got := Slug(c.ref); got != c.want {
			t.Errorf("%s: got %q want %q", c.ref, got, c.want)
		}
	}
}

func TestIngestFileAndURLCache(t *testing.T) {
	dir := t.TempDir()
	prd := filepath.Join(dir, "login.md")
	if err := os.WriteFile(prd, []byte("# Login\nUser can log in."), 0o644); err != nil {
		t.Fatal(err)
	}
	ag := &fakeAgent{}
	s, _ := NewSourceService(Deps{Agent: ag, WorkDir: filepath.Join(dir, "work")})
	doc, err := s.Ingest(context.Background(), prd)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Slug != "login" || doc.Hash == "" {
		t.Fatalf("bad doc %+v", doc)
	}

	// The fake agent writes nothing, so an uncached URL fails with a hint.
	if _, err := s.Ingest(context.Background(), "https://www.notion.so/x/Page-0123456789abcdef0123456789abcdef"); err == nil {
		t.Fatal("want error when agent writes nothing")
	}
	// Once cached, the agent is not called again.
	cache := filepath.Join(dir, "work", "page", "prd.md")
	_ = os.WriteFile(cache, []byte("# Page"), 0o644)
	calls := ag.calls
	if _, err := s.Ingest(context.Background(), "https://www.notion.so/x/Page-0123456789abcdef0123456789abcdef"); err != nil {
		t.Fatal(err)
	}
	if ag.calls != calls {
		t.Fatal("cached URL should not call the agent")
	}
}

func TestLoadKinds(t *testing.T) {
	p := filepath.Join(t.TempDir(), "sources.yaml")
	_ = os.WriteFile(p, []byte("sources:\n  - name: jira\n    match: '^https://[^/]+\\.atlassian\\.net/browse/'\n    allowed_tools: [mcp__claude_ai_Atlassian_Rovo, Write]\n"), 0o644)
	ks, err := LoadKinds(p)
	if err != nil || len(ks) != 1 || ks[0].Name != "jira" {
		t.Fatalf("got %v %v", ks, err)
	}
	if ks, err := LoadKinds(filepath.Join(t.TempDir(), "missing.yaml")); err != nil || ks != nil {
		t.Fatal("missing file should be ignored")
	}
}

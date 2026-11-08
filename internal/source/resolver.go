package source

import (
	"context"
	"path/filepath"
	"regexp"
	"strings"
)

// resolver fetches one kind of PRD reference into a markdown file at out.
type resolver interface {
	Name() string
	Match(ref string) bool
	Fetch(ctx context.Context, ref, out, log string) error
}

// extResolver matches local files by extension.
type extResolver struct {
	name  string
	exts  []string
	fetch func(ctx context.Context, ref, out, log string) error
}

func (r extResolver) Name() string { return r.name }
func (r extResolver) Match(ref string) bool {
	if IsURL(ref) {
		return false
	}
	ext := strings.ToLower(filepath.Ext(ref))
	for _, e := range r.exts {
		if ext == e {
			return true
		}
	}
	return false
}
func (r extResolver) Fetch(ctx context.Context, ref, out, log string) error {
	return r.fetch(ctx, ref, out, log)
}

// Kind configures an agent-backed resolver: a regex on the reference and the
// tools the agent may use to read it (usually one MCP server).
type Kind struct {
	Name         string   `yaml:"name"`
	Match        string   `yaml:"match"`
	AllowedTools []string `yaml:"allowed_tools"`
	// Hint is shown when fetching fails, e.g. how to authenticate the MCP.
	Hint string `yaml:"hint"`
	// Instructions are extra, source-specific guidance for the agent.
	Instructions string `yaml:"instructions"`
}

// builtinKinds are tried after any kinds from sources.yaml.
var builtinKinds = []Kind{
	{
		Name:         "notion",
		Match:        `^https?://([a-z0-9-]+\.)?notion\.(so|site)/`,
		AllowedTools: []string{"mcp__claude_ai_Notion__notion-fetch", "mcp__claude_ai_Notion__notion-search", "Write"},
		Hint:         "connect Notion: run `claude`, then /mcp and authenticate claude.ai Notion",
		Instructions: "Use the Notion fetch tool with the URL. Include child pages and linked databases that hold requirements.",
	},
	{
		Name:         "confluence",
		Match:        `^https?://[^/]+(\.atlassian\.net/wiki|/confluence)/`,
		AllowedTools: []string{"mcp__claude_ai_Atlassian_Rovo", "Write"},
		Hint:         "connect Confluence: run `claude`, then /mcp and authenticate claude.ai Atlassian Rovo",
		Instructions: "Use the Atlassian tools to read the Confluence page. Read only; never edit Confluence.",
	},
	{
		Name:         "web",
		Match:        `^https?://`,
		AllowedTools: []string{"WebFetch", "Write"},
		Hint:         "the page must be publicly reachable; for private docs add an MCP kind to sources.yaml",
	},
	{
		Name:         "pdf",
		Match:        `(?i)\.pdf$`,
		AllowedTools: []string{"Read", "Write"},
	},
}

func (k Kind) compile() (*regexp.Regexp, error) { return regexp.Compile(k.Match) }

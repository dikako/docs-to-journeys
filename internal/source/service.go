package source

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Deps are the SourceService's collaborators.
type Deps struct {
	Agent agent
	// WorkDir holds one folder per ingested document.
	WorkDir string
	// Kinds are extra agent-backed kinds (from sources.yaml), tried first.
	Kinds []Kind
	Force bool
}

// SourceService ingests PRDs into markdown.
type SourceService struct {
	resolvers []resolver
	workDir   string
	force     bool
}

// NewSourceService builds the ordered resolver registry: configured kinds,
// then local file types, then built-in link kinds.
func NewSourceService(d Deps) (*SourceService, error) {
	var rs []resolver
	add := func(k Kind) error {
		re, err := k.compile()
		if err != nil {
			return fmt.Errorf("source kind %s: %w", k.Name, err)
		}
		rs = append(rs, agentResolver{kind: k, re: re, agent: d.Agent})
		return nil
	}
	for _, k := range d.Kinds {
		if err := add(k); err != nil {
			return nil, err
		}
	}
	rs = append(rs, fileResolvers()...)
	for _, k := range builtinKinds {
		if err := add(k); err != nil {
			return nil, err
		}
	}
	return &SourceService{resolvers: rs, workDir: d.WorkDir, force: d.Force}, nil
}

// Detect returns the name of the resolver that handles ref.
func (s *SourceService) Detect(ref string) (string, error) {
	r, err := s.pick(ref)
	if err != nil {
		return "", err
	}
	return r.Name(), nil
}

func (s *SourceService) pick(ref string) (resolver, error) {
	for _, r := range s.resolvers {
		if r.Match(ref) {
			return r, nil
		}
	}
	names := make([]string, len(s.resolvers))
	for i, r := range s.resolvers {
		names[i] = r.Name()
	}
	return nil, fmt.Errorf("no source kind matches %q (known: %s); add one to sources.yaml", ref, strings.Join(names, ", "))
}

// Ingest fetches ref into <WorkDir>/<slug>/prd.md. Local files are re-read on
// every call; links are cached until Force is set, because fetching costs an
// agent run.
func (s *SourceService) Ingest(ctx context.Context, ref string) (Document, error) {
	r, err := s.pick(ref)
	if err != nil {
		return Document{}, err
	}
	slug := Slug(ref)
	dir := filepath.Join(s.workDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Document{}, err
	}
	out := filepath.Join(dir, "prd.md")
	log := filepath.Join(dir, "ingest.log")

	cached := false
	if IsURL(ref) && !s.force {
		if st, err := os.Stat(out); err == nil && st.Size() > 0 {
			cached = true
		}
	}
	if !cached {
		if err := r.Fetch(ctx, ref, out, log); err != nil {
			return Document{}, err
		}
	}
	b, err := os.ReadFile(out)
	if err != nil {
		return Document{}, err
	}
	if strings.TrimSpace(string(b)) == "" {
		return Document{}, fmt.Errorf("%s: ingested document is empty", out)
	}
	_ = os.WriteFile(filepath.Join(dir, "source.txt"), []byte(r.Name()+" "+ref+"\n"), 0o644)
	return Document{Slug: slug, Path: out, Hash: hash(b), Ref: ref}, nil
}

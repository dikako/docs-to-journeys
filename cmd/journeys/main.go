// Command journeys is the PRD → Gherkin → journey → device pipeline.
//
//	journeys ingest   <source>              PRD (file or link) → work/<slug>/prd.md
//	journeys gherkin  <source>              → features/<slug>.feature
//	journeys convert  <feature>...          → generated/<slug>/<scenario>.journey.xml
//	journeys run      <serials> [glob|dir]… run journeys with self-healing
//	journeys all      <source> <serials>    every stage in order
//
// <serials> is one adb serial or a comma-separated list; journeys are spread
// across the devices in parallel.
//
// For compatibility, `journeys <serial> [glob]` means `journeys run`.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"docs-to-journeys/internal/common/adb"
	"docs-to-journeys/internal/common/claude"
	"docs-to-journeys/internal/common/proc"
	"docs-to-journeys/internal/gherkin"
	"docs-to-journeys/internal/runner"
	"docs-to-journeys/internal/source"
)

const usage = `usage:
  journeys ingest   [flags] <source>
  journeys gherkin  [flags] <source>
  journeys convert  [flags] <feature>...
  journeys run      [flags] <serials> [glob|dir]...
  journeys all      [flags] <source> <serials>
  journeys version

<source> is a .md/.txt/.docx/.pdf file or a Notion, Confluence, or web link.
<serials> is one adb serial or a comma-separated list (run in parallel).
Unchanged journeys that already passed on the same app build are skipped as CACHED.
Extra link kinds (other MCPs) go in sources.yaml.

flags (env var in brackets):`

type config struct {
	pkg, model, reports, work, features, generated, sources string
	timeout                                                 time.Duration
	maxHeal                                                 int
	force, noCache                                          bool
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func envInt(k string, def int) int {
	if n, err := strconv.Atoi(os.Getenv(k)); err == nil {
		return n
	}
	return def
}

func parseFlags(name string, args []string) (config, []string) {
	var c config
	fs := flag.NewFlagSet(name, flag.ExitOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, usage)
		fs.PrintDefaults()
	}
	fs.StringVar(&c.pkg, "pkg", env("PKG", "com.saucelabs.mydemoapp.android"), "app package under test [PKG]")
	fs.StringVar(&c.model, "model", env("MODEL", "sonnet"), "Claude model [MODEL]")
	timeout := fs.Int("timeout", envInt("JOURNEY_TIMEOUT", 600), "seconds per agent call [JOURNEY_TIMEOUT]")
	fs.StringVar(&c.reports, "reports", env("REPORT_DIR", "reports"), "report directory [REPORT_DIR]")
	fs.StringVar(&c.work, "work", env("WORK_DIR", "work"), "ingested PRDs [WORK_DIR]")
	fs.StringVar(&c.features, "features", env("FEATURE_DIR", "features"), "generated .feature files [FEATURE_DIR]")
	fs.StringVar(&c.generated, "generated", env("GENERATED_DIR", "generated"), "generated journeys [GENERATED_DIR]")
	fs.StringVar(&c.sources, "sources", env("SOURCES_FILE", "sources.yaml"), "extra source kinds [SOURCES_FILE]")
	fs.IntVar(&c.maxHeal, "max-heal", envInt("MAX_HEAL", 3), "heal → re-run cycles per failing journey [MAX_HEAL]")
	fs.BoolVar(&c.force, "force", os.Getenv("FORCE") == "1", "ignore caches and overwrite generated files [FORCE=1]")
	fs.BoolVar(&c.noCache, "no-cache", os.Getenv("NO_CACHE") == "1", "re-run journeys even if an identical run already passed [NO_CACHE=1]")
	_ = fs.Parse(args)
	c.noCache = c.noCache || c.force
	c.timeout = time.Duration(*timeout) * time.Second
	return c, fs.Args()
}

// version is set at release build time: -ldflags "-X main.version=v1.2.3".
var version = "dev"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) > 0 && (args[0] == "version" || args[0] == "--version" || args[0] == "-v") {
		fmt.Println("journeys", version)
		return 0
	}
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Fprintln(os.Stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "ingest", "gherkin", "convert", "run", "all":
	default:
		cmd, rest = "run", args // legacy: <serial> [glob]
	}
	c, pos := parseFlags(cmd, rest)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	code := 0
	switch cmd {
	case "ingest":
		err = need(pos, 1, "ingest <source>")
		if err == nil {
			var d source.Document
			if d, err = c.ingest(ctx, pos[0]); err == nil {
				fmt.Println(d.Path)
			}
		}
	case "gherkin":
		err = need(pos, 1, "gherkin <source>")
		if err == nil {
			var p string
			if p, err = c.gherkin(ctx, pos[0]); err == nil {
				fmt.Println(p)
			}
		}
	case "convert":
		err = need(pos, 1, "convert <feature>...")
		for i := 0; err == nil && i < len(pos); i++ {
			_, err = c.convert(ctx, pos[i])
		}
	case "run":
		err = need(pos, 1, "run <serials> [glob|dir]...")
		if err == nil {
			code, err = c.run(ctx, pos[0], pos[1:])
		}
	case "all":
		err = need(pos, 2, "all <source> <serials>")
		if err == nil {
			var feature, dir string
			if feature, err = c.gherkin(ctx, pos[0]); err == nil {
				if dir, err = c.convert(ctx, feature); err == nil {
					code, err = c.run(ctx, pos[1], []string{dir})
				}
			}
		}
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		return 2
	}
	return code
}

func need(pos []string, n int, form string) error {
	if len(pos) < n {
		return errors.New("usage: journeys " + form)
	}
	return nil
}

func (c config) agent() claude.Client {
	return claude.Client{Model: c.model, Timeout: c.timeout}
}

func (c config) ingest(ctx context.Context, ref string) (source.Document, error) {
	kinds, err := source.LoadKinds(c.sources)
	if err != nil {
		return source.Document{}, err
	}
	svc, err := source.NewSourceService(source.Deps{Agent: c.agent(), WorkDir: c.work, Kinds: kinds, Force: c.force})
	if err != nil {
		return source.Document{}, err
	}
	kind, err := svc.Detect(ref)
	if err != nil {
		return source.Document{}, err
	}
	fmt.Fprintf(os.Stderr, "ingest: %s as %s\n", ref, kind)
	return svc.Ingest(ctx, ref)
}

func (c config) gherkinService() *gherkin.GherkinService {
	return gherkin.NewGherkinService(gherkin.Deps{
		Agent: c.agent(), Pkg: c.pkg, FeatureDir: c.features, JourneyDir: c.generated, Force: c.force,
	})
}

func (c config) gherkin(ctx context.Context, ref string) (string, error) {
	if err := proc.Require("claude"); err != nil {
		return "", err
	}
	doc, err := c.ingest(ctx, ref)
	if err != nil {
		return "", err
	}
	p, err := c.gherkinService().Generate(ctx, doc)
	if err == nil {
		fmt.Fprintf(os.Stderr, "gherkin: %s\n", p)
	}
	return p, err
}

// convert returns the directory holding the feature's journeys.
func (c config) convert(ctx context.Context, feature string) (string, error) {
	if err := proc.Require("claude"); err != nil {
		return "", err
	}
	res, err := c.gherkinService().ToJourneys(ctx, feature)
	if err != nil {
		return "", err
	}
	failed := 0
	for _, r := range res {
		switch {
		case r.Err != nil:
			failed++
			fmt.Fprintf(os.Stderr, "convert: FAILED %s: %v\n", r.Path, r.Err)
		case r.Skipped:
			fmt.Fprintf(os.Stderr, "convert: kept   %s (use -force to regenerate)\n", r.Path)
		default:
			fmt.Fprintf(os.Stderr, "convert: wrote  %s\n", r.Path)
		}
	}
	if failed == len(res) {
		return "", fmt.Errorf("no journey could be generated from %s", feature)
	}
	slug := strings.TrimSuffix(filepath.Base(feature), filepath.Ext(feature))
	return filepath.Join(c.generated, slug), nil
}

func (c config) run(ctx context.Context, serials string, patterns []string) (int, error) {
	if err := proc.Require("adb", "claude", "android"); err != nil {
		return 0, err
	}
	paths, err := expand(patterns)
	if err != nil {
		return 0, err
	}

	var lanes []runner.Lane
	for _, serial := range strings.Split(serials, ",") {
		serial = strings.TrimSpace(serial)
		if serial == "" {
			continue
		}
		dev := adb.Device{Serial: serial}
		activity, restore, err := dev.Prepare(ctx, c.pkg)
		if err != nil {
			return 0, err
		}
		defer restore()
		lanes = append(lanes, runner.Lane{Serial: serial, Device: dev, Activity: activity, Env: dev.Env(ctx, c.pkg)})
	}
	if len(lanes) == 0 {
		return 0, errors.New("no device serial given")
	}

	svc := runner.NewRunnerService(runner.Deps{
		Agent: c.agent(), Lanes: lanes, Pkg: c.pkg, Model: c.model,
		ReportDir: c.reports, MaxHeal: c.maxHeal, NoCache: c.noCache, Out: os.Stdout,
	})
	start := time.Now()
	results, err := svc.Run(ctx, paths)
	header := fmt.Sprintf("Summary (%s on %s, model=%s, max-heal=%d, %s)", c.pkg, serials, c.model, c.maxHeal, time.Since(start).Round(time.Second))
	if runner.Summary(os.Stdout, header, results) > 0 {
		return 1, err
	}
	return 0, err
}

// expand resolves globs and directories to journey files. No pattern means
// *.journey.xml in the current directory.
func expand(patterns []string) ([]string, error) {
	if len(patterns) == 0 {
		patterns = []string{"*.journey.xml"}
	}
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	for _, pat := range patterns {
		if st, err := os.Stat(pat); err == nil && st.IsDir() {
			var found []string
			_ = filepath.WalkDir(pat, func(p string, d fs.DirEntry, err error) error {
				if err == nil && !d.IsDir() && strings.HasSuffix(p, ".journey.xml") {
					found = append(found, p)
				}
				return nil
			})
			sort.Strings(found)
			for _, p := range found {
				add(p)
			}
			continue
		}
		m, err := filepath.Glob(pat)
		if err != nil {
			return nil, err
		}
		for _, p := range m {
			add(p)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no journeys match %s", strings.Join(patterns, " "))
	}
	return out, nil
}

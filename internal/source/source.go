// Package source turns a PRD reference into normalized markdown on disk.
// The reference can be a local file (md, txt, docx, pdf…) or a link (Notion,
// Confluence, any URL). Links go through Claude with the matching MCP tools.
// Resolvers form an ordered registry that sources.yaml can extend, so a new MCP
// is a config entry, not a code change. This context does not interpret the
// PRD; generating Gherkin from it is the gherkin context's job.
package source

import (
	"crypto/sha256"
	"encoding/hex"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
)

// Document is an ingested PRD.
type Document struct {
	Slug string
	// Path is the normalized markdown file.
	Path string
	// Hash is the sha256 of the markdown content.
	Hash string
	// Ref is the original reference the user passed.
	Ref string
}

var (
	nonSlug   = regexp.MustCompile(`[^a-z0-9]+`)
	notionID  = regexp.MustCompile(`-?[0-9a-f]{32}$`)
	isURLLike = regexp.MustCompile(`^https?://`)
	allDigits = regexp.MustCompile(`^\d+$`)
)

// IsURL reports whether ref is an http(s) link.
func IsURL(ref string) bool { return isURLLike.MatchString(ref) }

// Slug derives a short, filesystem-safe name from a file path or URL.
func Slug(ref string) string {
	base := ref
	if IsURL(ref) {
		if u, err := url.Parse(ref); err == nil {
			p := strings.Trim(u.Path, "/")
			base = u.Host
			if p != "" {
				parts := strings.Split(p, "/")
				base = parts[len(parts)-1]
				// Confluence: /wiki/spaces/X/pages/<id>/<Title>
				if len(parts) >= 2 && allDigits.MatchString(base) {
					base = parts[len(parts)-2] + "-" + base
				}
			}
		}
	} else {
		base = strings.TrimSuffix(filepath.Base(ref), filepath.Ext(ref))
	}
	s := strings.ToLower(base)
	s = notionID.ReplaceAllString(s, "")
	s = strings.Trim(nonSlug.ReplaceAllString(s, "-"), "-")
	if len(s) > 60 {
		s = strings.Trim(s[:60], "-")
	}
	if s == "" {
		s = "prd-" + hash([]byte(ref))[:8]
	}
	return s
}

func hash(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

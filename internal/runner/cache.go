package runner

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// promptVersion changes whenever the run prompt changes, so a prompt fix
// invalidates every cached pass automatically.
var promptVersion = func() string {
	h := sha256.Sum256([]byte(runPrompt("J", "S", "P", "R", "L")))
	return hex.EncodeToString(h[:8])
}()

// cacheKey is everything a pass depends on: the exact journey, where it ran
// (app version, device model, OS), which model judged it, and how.
func (s *RunnerService) cacheKey(journeyXML []byte, l Lane) string {
	h := sha256.New()
	for _, part := range [][]byte{journeyXML, []byte(l.Env), []byte(s.d.Pkg), []byte(s.d.Model), []byte(promptVersion)} {
		h.Write(part)
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// cacheEntry is the on-disk record of a pass.
type cacheEntry struct {
	Key    string    `json:"key"`
	Status Status    `json:"status"`
	Heals  int       `json:"heals"`
	Serial string    `json:"serial"`
	At     time.Time `json:"at"`
}

// resultCache stores one JSON file per journey under <reports>/.cache, so
// parallel lanes never write the same file.
type resultCache struct{ dir string }

func (c resultCache) path(name string) string { return filepath.Join(c.dir, name+".json") }

// lookup returns the cached pass for name if the key matches and the pass's
// report still exists.
func (c resultCache) lookup(name, key, report string) (cacheEntry, bool) {
	b, err := os.ReadFile(c.path(name))
	if err != nil {
		return cacheEntry{}, false
	}
	var e cacheEntry
	if json.Unmarshal(b, &e) != nil || e.Key != key || !e.Status.Passed() {
		return cacheEntry{}, false
	}
	if _, err := os.Stat(report); err != nil {
		return cacheEntry{}, false
	}
	return e, true
}

func (c resultCache) store(name, key string, r Result) {
	if os.MkdirAll(c.dir, 0o755) != nil {
		return
	}
	b, _ := json.MarshalIndent(cacheEntry{Key: key, Status: r.Status, Heals: r.Heals, Serial: r.Serial, At: time.Now()}, "", "  ")
	_ = os.WriteFile(c.path(name), b, 0o644)
}

func (c resultCache) forget(name string) { _ = os.Remove(c.path(name)) }

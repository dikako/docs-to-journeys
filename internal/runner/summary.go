package runner

import (
	"fmt"
	"io"
	"time"
)

// Summary prints the result table and returns how many did not pass.
func Summary(w io.Writer, header string, results []Result) int {
	failed, cached := 0, 0
	fmt.Fprintf(w, "\n%s %s %s\n", sep, header, sep)
	for _, r := range results {
		if !r.Status.Passed() {
			failed++
		}
		if r.Status == Cached {
			cached++
		}
		fmt.Fprintf(w, "%-14s %6s  heals=%d  %-14s %s\n", r.Status, r.Elapsed.Round(time.Second), r.Heals, r.Serial, r.Name)
	}
	fmt.Fprintln(w, sep)
	fmt.Fprintf(w, "%d run, %d cached, %d not passed\n", len(results), cached, failed)
	if cached > 0 {
		fmt.Fprintln(w, "(cached = unchanged journey already passed on this app version; use -no-cache to re-run)")
	}
	return failed
}

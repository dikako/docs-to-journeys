package journey

import "fmt"

// Verdict is the guard's decision. A rejected heal is a normal outcome, so it
// is a value, not an error.
type Verdict struct {
	OK     bool
	Reason string
}

// FlowPreserved decides whether healed still tests what original tested:
//   - the first locked actions (those that already passed) are unchanged;
//   - the ordered list of assertions is unchanged.
//
// Non-assertion actions after the locked prefix may be reworded, inserted or
// removed so the flow matches the real UI.
func FlowPreserved(original, healed Journey, locked int) Verdict {
	if healed.IsZero() || len(healed.actions) == 0 {
		return Verdict{Reason: "healed journey is empty"}
	}
	if locked > len(original.actions) {
		locked = len(original.actions)
	}
	if locked > len(healed.actions) {
		return Verdict{Reason: fmt.Sprintf("healed journey dropped passed actions (has %d, %d passed)", len(healed.actions), locked)}
	}
	for i := 0; i < locked; i++ {
		if original.actions[i].Text() != healed.actions[i].Text() {
			return Verdict{Reason: fmt.Sprintf("passed action %d changed: %q -> %q", i+1, original.actions[i], healed.actions[i])}
		}
	}
	oa, ha := original.Assertions(), healed.Assertions()
	if len(oa) != len(ha) {
		return Verdict{Reason: fmt.Sprintf("assertion count changed: %d -> %d", len(oa), len(ha))}
	}
	for i := range oa {
		if oa[i].Text() != ha[i].Text() {
			return Verdict{Reason: fmt.Sprintf("assertion %d changed: %q -> %q", i+1, oa[i], ha[i])}
		}
	}
	return Verdict{OK: true}
}

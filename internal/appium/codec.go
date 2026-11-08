package appium

import (
	"encoding/json"
	"fmt"
	"strings"
)

// stepRow is the JSON the run agent writes, one object per executed action.
// Tags live here, not on the domain types.
type stepRow struct {
	Step    int    `json:"step"`
	Action  string `json:"action"`
	Input   string `json:"input,omitempty"`
	Element string `json:"element,omitempty"`
	By      string `json:"by,omitempty"`
	Value   string `json:"value,omitempty"`
}

// Decode validates the agent's step JSON against the journey's actions.
// Unknown actions become Other; rows with an unknown strategy keep their
// action but lose the locator; duplicate names for different locators are
// made unique.
func Decode(data []byte, journey, slug string, actions []string, failedAt int) (Script, error) {
	var rows []stepRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return Script{}, fmt.Errorf("steps json: %w", err)
	}
	sc := Script{Journey: journey, Slug: slug, Total: len(actions), FailedAt: failedAt}
	byName := map[string]Locator{}
	for _, r := range rows {
		if r.Step < 1 || r.Step > len(actions) {
			continue
		}
		st := Step{Index: r.Step, Text: actions[r.Step-1], Action: Action(strings.ToLower(r.Action)), Input: r.Input}
		if !knownActions[st.Action] {
			st.Action = Other
		}
		if by := Strategy(r.By); knownStrategies[by] && r.Value != "" {
			l := Locator{Name: constName(r.Element), By: by, Value: r.Value}
			l = uniqueName(byName, l)
			byName[l.Name] = l
			st.Locator = l
		}
		// Appium's sendKeys focuses the field itself, so a tap right before
		// typing into the same element is noise.
		if n := len(sc.Steps); n > 0 && st.Action == Type {
			if prev := sc.Steps[n-1]; prev.Action == Tap && prev.Index == st.Index && prev.Locator == st.Locator {
				sc.Steps = sc.Steps[:n-1]
			}
		}
		sc.Steps = append(sc.Steps, st)
	}
	return sc, nil
}

func uniqueName(byName map[string]Locator, l Locator) Locator {
	base := l.Name
	for n := 2; ; n++ {
		prev, taken := byName[l.Name]
		if !taken || (prev.By == l.By && prev.Value == l.Value) {
			return l
		}
		l.Name = fmt.Sprintf("%s_%d", base, n)
	}
}

// DecodeSession reads steps recorded during an exploratory session. There is
// no journey action list: each row is its own step, described by its action
// and element.
func DecodeSession(data []byte, title, slug string) (Script, error) {
	var rows []stepRow
	if err := json.Unmarshal(data, &rows); err != nil {
		return Script{}, fmt.Errorf("steps json: %w", err)
	}
	texts := make([]string, len(rows))
	for i := range rows {
		rows[i].Step = i + 1
		texts[i] = describe(rows[i])
	}
	renumbered, _ := json.Marshal(rows)
	sc, err := Decode(renumbered, title, slug, texts, 0)
	sc.Kind = "Session"
	return sc, err
}

func describe(r stepRow) string {
	d := r.Action
	if r.Input != "" {
		d += fmt.Sprintf(" %q", r.Input)
	}
	if r.Element != "" {
		d += " " + r.Element
	}
	return strings.TrimSpace(d)
}

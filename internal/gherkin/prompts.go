package gherkin

import (
	"fmt"
	"strings"
)

func generatePrompt(prdPath, out, pkg, prevErr string) string {
	p := fmt.Sprintf(`Read the product requirements document at %s and write a Gherkin feature file to %s.
The feature will be executed as end-to-end UI tests on the Android app %s by an agent that only sees the screen.

Rules:
- Exactly one "Feature:" with a short description of the user value.
- Use "Background:" for preconditions shared by every scenario (for example opening the app and logging in), written as UI steps.
- One "Scenario:" per acceptance criterion or user flow in the PRD. Use "Scenario Outline:" with "Examples:" only for the same flow with different data.
- Cover the happy path first, then the negative and edge cases the PRD states explicitly. Do not invent requirements.
- Steps are concrete UI interactions with concrete test data from the PRD (e.g. When I type "standard_user" in the Username field).
- Every "Then" step is an observable assertion on screen (text, screen, element, or state that is visible).
- Use the field and button labels the PRD uses. Do not guess implementation details.
- Plain Gherkin only: no code fences, no step definitions.
Write the file and reply with one line: the number of scenarios.`, prdPath, out, pkg)
	if prevErr != "" {
		p += "\n\nYour previous file did not parse: " + prevErr + "\nFix it and overwrite the file."
	}
	return p
}

// actionStyle shows the target format, taken from the hand-written journeys.
const actionStyle = `Tap "Hamburger" menu
Tap "Log In" menu
Type "standard_user" in the Username field
Tap the "LOGIN" button
Verify that the "PRODUCTS" screen is shown with at least one product listed
Scroll down and tap the "FINISH" button`

func translatePrompt(sc Scenario, pkg, prevErr string) string {
	var steps strings.Builder
	for i, st := range sc.Steps {
		fmt.Fprintf(&steps, "%d. [%s] %s\n", i+1, st.Kind, st.Text)
		if st.Arg != "" {
			for _, l := range strings.Split(st.Arg, "\n") {
				steps.WriteString("      " + l + "\n")
			}
		}
	}
	p := fmt.Sprintf(`Convert this Gherkin scenario into a journey for an agent that drives the Android app %s through its UI.

Feature: %s
Scenario: %s
Steps (Background included, kind in brackets):
%s
Rules:
- Output imperative UI actions, one interaction per action, in this style:
%s
- Given/When steps become Tap / Type / Scroll / Select / Swipe / Wait actions. Split a step into several actions if it needs several interactions. Do not mention launching or installing the app; it is already open.
- Every [Then] step becomes exactly one action that starts with "Verify". Keep its meaning; do not weaken it. No other action may start with Verify, Check, Ensure, Assert, or Confirm.
- Keep every quoted value and label exactly as written in the step.
- Expand data tables into one action per row/field.
Reply with JSON only, no code fence: {"description": "<one sentence>", "actions": ["...", "..."]}`, pkg, sc.Feature, sc.Name, steps.String(), indent(actionStyle))
	if prevErr != "" {
		p += "\n\nYour previous answer was rejected: " + prevErr
	}
	return p
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}

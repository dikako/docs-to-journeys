package explore

import (
	"fmt"
	"time"
)

// findingsHeader seeds findings.md so it exists, states the session, and
// shows the agent the exact format.
func findingsHeader(c Charter, pkg string) string {
	return fmt.Sprintf(`# Exploratory findings: %s

App: %s. Each finding below is a bug or risk found during an exploratory session.
For regression tests: every finding is one scenario. Its steps reproduce the issue,
and its expected result is the assertion.

`, firstLine(c.Text()), pkg)
}

func explorePrompt(c Charter, o Options, serial, pkg string, deadline time.Time, dir, findings, steps string) string {
	p := fmt.Sprintf(`You are an expert exploratory tester. Run a time-boxed exploratory session on the Android app %[1]s on device %[2]s, using the android-cli skill. Prefer 'android layout' to inspect screens.

CHARTER (your mission):
%[3]s

TIME BOX: stop exploring at %[4]s. Check the time with the 'date' command every few actions. After the deadline you may spend at most 2 minutes finishing your notes, then stop.

The app is already open. Never uninstall the app, change device settings, or clear app data yourself.

HOW TO RUN COMMANDS (every command you run is recorded as a test step, so keep them readable):
- One device interaction per command, using adb or android directly, e.g. adb -s %[2]s shell input tap 540 820
- Give every command a short description of the step (what you are doing, e.g. "Type emoji into Username").
- No helper scripts, shell functions, or variables. Inspecting with 'android layout' may be piped through grep.
- Keep every file you create inside %[6]s (screenshots too); never write to /tmp.

HOW TO EXPLORE: vary inputs and paths, guided by the charter. Useful heuristics:
- input: empty, whitespace only, very long (300+ chars), emoji and non-Latin text, special characters, leading/trailing spaces, numbers where text is expected
- boundaries: min/max values, zero, negative, one past the limit
- navigation: back button mid-flow, going back and forth, repeating an action quickly, re-entering a screen
- state: what is kept or lost after leaving and returning, double submit
- feedback: missing or unclear error messages, wrong text, broken layout, crashes or freezes

RECORD AS YOU GO (the session can be cut off at any time, so never batch this up):
1. After each issue, immediately append it to %[5]s in exactly this format:

## Finding <N>: <short title>
- Severity: critical | high | medium | low
- Steps to reproduce:
  1. <step, using the exact labels on screen>
  2. ...
- Expected: <what should happen>
- Actual: <what happened>
- Screenshot: screenshots/finding-<N>.png

   Then save that screenshot to %[6]s/screenshots/finding-<N>.png (for example: adb -s %[2]s exec-out screencap -p > that path).
2. Every few actions, rewrite %[7]s as a JSON array of everything you did so far, in order, one object per interaction:
   {"action": "tap|type|verify|scroll|swipe|back|wait|other", "input": "<typed text, only for type>", "element": "<UPPER_SNAKE_NAME>", "by": "accessibilityId|id|androidUIAutomator|xpath", "value": "<locator value>"}
   Choose "by" in this order: accessibilityId, id, androidUIAutomator, xpath. Omit element/by/value for back and wait. Valid JSON only.

Only log real problems or clear risks; do not log things that work as expected.
When you stop, reply with one line: SUMMARY: <N findings, areas covered, areas not reached>.`,
		pkg, serial, c.Text(), deadline.Format("15:04:05 MST"), findings, dir, steps)
	if o.Prompt != "" {
		p += "\n\nADDITIONAL INSTRUCTIONS FROM THE TESTER:\n" + o.Prompt
	}
	return p
}

package runner

import (
	"fmt"
	"regexp"
	"strings"
)

func runPrompt(journeyPath, serial, pkg, report, locators string) string {
	return fmt.Sprintf(`Run the journey in %[1]s on device %[2]s using the android-cli skill.
The app %[3]s is already launched and in the foreground; do not reinstall, clear, or relaunch it.
Prefer 'android layout' to locate elements; take a screenshot only for visual assertions or when the element is not in the layout.
Match labels case-insensitively and ignore small wording differences (e.g. "LOGIN" matches a "Login" button). Judge a Verify action by its meaning: it passes when the screen shows what it describes.
Stop at the first failed action.
Write results to %[4]s: exactly one "### Action: <text>" header per <action> in the journey, with <text> copied verbatim from the journey (do not split, merge, or reword actions), ending in ✅ if it passed or ❌ if it failed. Actions not run get a header with no mark. Do not include HTML comments or legend lines.
For every element you tap, type into, or verify, read its attributes from 'android layout' and record an Appium Java locator.
Choose the strategy in this order: AppiumBy.accessibilityId (content-desc), AppiumBy.id (full resource-id), AppiumBy.androidUIAutomator, and AppiumBy.xpath only if nothing else is unique.
Do not use dynamic text or index-based selectors unless no other option exists.
Write the locators to %[5]s, one line per element, in exactly this form and nothing else:
public static final By UPPER_SNAKE_NAME = AppiumBy.<strategy>("<value>");`, journeyPath, serial, pkg, report, locators)
}

func healPrompt(journeyPath, serial, pkg, report, failText string, failIdx, attempt, max int) string {
	where := fmt.Sprintf("at the action %q", failText)
	locked := "Do not change any action before the failing one; those already passed."
	if failIdx >= 0 {
		where = fmt.Sprintf("at action %d: %q", failIdx+1, failText)
		if failIdx > 0 {
			locked = fmt.Sprintf("Do not change actions 1-%d; they already passed.", failIdx)
		}
	}
	return fmt.Sprintf(`The journey %[1]s failed %[2]s. Report: %[3]s. This is heal attempt %[4]d of %[5]d.
Device %[6]s is still on the failing screen with the app %[7]s open. Use the android-cli skill and inspect the screen with 'android layout'.

First decide who is wrong:
- If the failing action actually succeeds on screen (the run judged it too strictly, e.g. only letter case differs), change nothing and reply with one line: RERUN: <reason>.
- If the app is wrong (crash, error dialog, missing feature, or a Verify action whose expectation is genuinely false), change nothing and reply with one line: APP_BUG: <reason>.
- Otherwise the journey's steps don't match the app's real flow. Fix the journey so it follows the exact flow of the app:
  - You may operate the app (tap, type, scroll, go back) to discover the exact path from the failing action to the next Verify action and beyond.
  - From the failing action onward you may reword, insert, or delete non-Verify actions so each action is exactly one real interaction using the label shown on screen (for example the real button text).
  - If a screen has required fields the journey does not fill, add Type actions with plausible test data. If the journey fills a field that does not exist, remove that action.
  - %[8]s
  - Never add, remove, reword, or reorder an action that starts with Verify, Check, Ensure, Assert, or Confirm. Those are the test's intent.
  - Keep the same XML format and edit only %[1]s.
  Then reply with one line: HEALED: <what changed>.`, journeyPath, where, report, attempt, max, serial, pkg, locked)
}

var (
	appBugRe = regexp.MustCompile(`(?m)^\W*APP_BUG:\s*(.+)$`)
	rerunRe  = regexp.MustCompile(`(?m)^\W*RERUN:\s*(.+)$`)
)

// appBug extracts the agent's APP_BUG reason, if it reported one.
func appBug(reply string) (string, bool) { return verdictLine(appBugRe, reply) }

// rerun extracts the agent's RERUN reason: the journey is fine and the
// failure was a misjudged check.
func rerun(reply string) (string, bool) { return verdictLine(rerunRe, reply) }

func verdictLine(re *regexp.Regexp, reply string) (string, bool) {
	m := re.FindStringSubmatch(reply)
	if m == nil {
		return "", false
	}
	return strings.TrimSpace(m[1]), true
}

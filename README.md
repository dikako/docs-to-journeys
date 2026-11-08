# docs-to-journeys

[![CI](https://github.com/dikako/docs-to-journeys/actions/workflows/ci.yml/badge.svg)](https://github.com/dikako/docs-to-journeys/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/dikako/docs-to-journeys?sort=semver)](https://github.com/dikako/docs-to-journeys/releases)
[![Go Reference](https://pkg.go.dev/badge/github.com/dikako/docs-to-journeys.svg)](https://pkg.go.dev/github.com/dikako/docs-to-journeys)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

**AI-generated, self-healing Android UI tests from your product docs.**
docs-to-journeys turns a PRD or spec (Markdown, Word, PDF, Notion, Confluence, …) into Gherkin (BDD) scenarios, then into
[Android Journeys](https://developer.android.com/tools/agents/android-cli/journeys). An AI agent (Claude Code + Android CLI)
runs these on a real device or emulator. When a journey's steps don't match the app's real flow, the runner **self-heals**
the journey and runs it again. It never changes the journey's assertions.

```
PRD/doc ──ingest──▶ prd.md ──gherkin──▶ .feature ──convert──▶ *.journey.xml ──run + heal──▶ reports/
```

Each run produces:
- a pass/fail report per action;
- an Appium Java locator for every element touched;
- a screenshot on failure;
- when healing happened, a diff of what changed.

> **What are journeys?** A journey is an XML file of plain-English steps that an AI agent performs and checks on an Android device. It's Google's format, and this project generates and runs files in it. See the official docs:
> [Android CLI support for Journeys](https://developer.android.com/tools/agents/android-cli/journeys) ·
> [Journeys for Android Studio](https://developer.android.com/studio/gemini/journeys) ·
> [Android CLI](https://developer.android.com/tools/agents/android-cli)

## Who is this for?

| You are… | Use it to… |
|---|---|
| **QA engineer** | turn a PRD or ticket into runnable acceptance tests in minutes, without writing code. Re-run them on every build, and get bug evidence (report + screenshot) when the app is wrong. |
| **Android developer** | smoke-test your feature on an emulator before opening a PR; keep UI tests working after a redesign (self-healing); get ready-made Appium locators for your own test code. |
| **QA lead / PM** | check that what was built matches the PRD: every acceptance criterion becomes a scenario with a pass/fail result. |

### Good fit and not a fit

| ✅ Good fit | ❌ Not a fit (use something else) |
|---|---|
| Acceptance tests generated from requirements | Unit tests or logic tests (use JUnit/Robolectric) |
| Smoke and regression runs on each build | Performance, battery, or network profiling |
| UI flows that change often (healing keeps tests current) | Pixel-perfect visual checks (use screenshot testing) |
| Checking the app against the PRD | Multi-finger gestures, long-press, rotation (see the [Journeys limits](https://developer.android.com/studio/gemini/journeys)) |
| Bootstrapping Appium locators | Hard, fully deterministic CI gates: an AI judges each step, so expect occasional flakiness |
| Turning exploratory findings into regression tests ([see below](#exploratory-testing)) | iOS or web apps |

A run costs agent time and tokens: about 1 minute per short journey, and several minutes when healing kicks in.
The [cache](#speed-caching-and-parallel-devices) skips journeys that already passed, so repeat runs are cheap.

### Exploratory testing?

**Not by itself: docs-to-journeys does scripted testing.** Every journey follows steps derived from a document, and healing only adapts *how* a step is done, never *what* is checked. It won't wander off to look for bugs nobody described.

It does three exploratory-adjacent things:
- **Docs vs. reality:** run journeys generated from a PRD, and every `HEALED` diff shows where the real app differs from what the document says (renamed buttons, extra screens, missing fields).
- **Flow discovery:** to heal, the agent explores the app to find the real path to the next check. The heal log (`reports/<name>.heal.<n>.log`) records what it found.
- **Bug triage:** `APP_BUG` results flag places where the agent decided the app, not the test, is wrong.

**Recommended loop for exploratory testing:** explore with Claude Code directly, then use this tool to lock in what you found.

```bash
# 1. Exploratory session: charter-based, with a time box, notes as you go
claude "Using the android-cli skill, run a 20-minute exploratory session on com.your.app on emulator-5554.
Charter: explore checkout with edge-case input (empty fields, very long text, emoji, back navigation mid-flow).
Log every issue to exploratory-notes.md: title, steps to reproduce, expected vs actual, screenshot path."

# 2. Turn the findings into regression tests
./run_journeys.sh gherkin exploratory-notes.md      # each issue → a scenario (review it!)
./run_journeys.sh convert features/exploratory-notes.feature
./run_journeys.sh run emulator-5554 generated/exploratory-notes
```

Bugs you find by hand then become journeys that fail until each bug is fixed, and pass (or come back as `CACHED`) after.

## Requirements

| Tool | Why |
|---|---|
| Go 1.27+ | builds the CLI |
| [Claude Code](https://claude.com/claude-code) CLI (`claude`), logged in | the agent that writes Gherkin, converts it, and drives the device |
| Android CLI (`android`) + `adb` | device control and UI inspection |
| An online device or emulator with the app installed | default app: Sauce Labs My Demo App, `com.saucelabs.mydemoapp.android` |
| macOS `textutil` (built in) | only for `.docx/.doc/.rtf/.html/.odt` sources |

### Setup

```bash
# Android CLI (https://developer.android.com/tools/agents/android-cli)
curl -fsSL https://dl.google.com/android/cli/latest/darwin_arm64/install.sh | bash
android info
adb devices                      # note your serial, e.g. emulator-5554

# Agent plugin that teaches Claude the android CLI
claude plugin marketplace add android/android-cli-plugin
claude plugin install android-cli@android-cli-plugin

# Optional: Notion / Confluence sources. Run `claude`, then /mcp, and authenticate
# the claude.ai Notion and/or Atlassian Rovo connectors once.
```

## Quick start

```bash
git clone https://github.com/dikako/docs-to-journeys.git && cd docs-to-journeys

# 1. Run the sample journeys (no PRD needed)
./run_journeys.sh run emulator-5554 examples/journeys

# 2. Full pipeline from an example PRD
./run_journeys.sh all examples/login-prd.md    emulator-5554
./run_journeys.sh all examples/checkout-prd.md emulator-5554
```

`run_journeys.sh` builds the Go CLI into `bin/journeys`, both on first use and whenever the sources change. It then forwards all arguments. You can run it from any folder: `reports/`, `work/`, `features/` and `generated/` are created in the current directory, and those folders are git-ignored.

### Testing your own app

1. Install your app on the device or emulator (`adb install app-debug.apk`).
2. Write a PRD or acceptance criteria in Markdown. Start from [`examples/checkout-prd.md`](examples/checkout-prd.md) and see [Writing a PRD that converts well](#writing-a-prd-that-converts-well).
3. Run with your package name:

```bash
./run_journeys.sh gherkin -pkg com.your.app docs/my-feature.md            # review features/my-feature.feature
./run_journeys.sh convert -pkg com.your.app features/my-feature.feature
./run_journeys.sh run     -pkg com.your.app emulator-5554 generated/my-feature
```

Tips:
- Use a **debug or test build** with test accounts, and turn off things like OTP and captcha. The agent can't read your SMS.
- Every journey starts from a **fresh app** (`pm clear` + launch). Put login and setup steps in the PRD's preconditions, which become the Gherkin `Background`.
- Commit `features/*.feature` and the journeys you keep. They're your test suite; `reports/` is not.

### Install with Go

```bash
go install github.com/dikako/docs-to-journeys/cmd/journeys@latest
journeys version
journeys all prd.md emulator-5554
```

Add `sources.yaml` from this repo to your working folder if you need extra source kinds.

### Install from a release (no Go needed)

Download the archive for your platform from the [Releases](https://github.com/dikako/docs-to-journeys/releases) page and unpack it.
Builds are available for macOS arm64/amd64 and Linux amd64/arm64.

```bash
tar -xzf docs-to-journeys_v1.0.0_darwin_arm64.tar.gz
cd docs-to-journeys_v1.0.0_darwin_arm64
./bin/journeys version
./bin/journeys all examples/login-prd.md emulator-5554   # same commands as run_journeys.sh
```

Each archive contains `bin/journeys`, `README.md`, `sources.yaml` and `examples/`. Check downloads against `checksums.txt`.
On macOS, if Gatekeeper blocks the unsigned binary, run `xattr -d com.apple.quarantine bin/journeys`.

## Commands

```bash
./run_journeys.sh ingest  <source>                  # → work/<slug>/prd.md
./run_journeys.sh gherkin <source>                  # → features/<slug>.feature   (review/edit this)
./run_journeys.sh convert features/<slug>.feature   # → generated/<slug>/<scenario>.journey.xml
./run_journeys.sh run     <serials> [glob|dir|file]...
./run_journeys.sh all     <source> <serials>        # all stages in order
./run_journeys.sh <serial> [glob]                   # short form of `run`
```

Recommended workflow:
1. Run `gherkin` and review the `.feature` file. It's the readable contract between the PRD and the tests.
2. Run `convert`, then `run`.
3. Commit the feature files and any healed journeys you want to keep.

| Flag | Env | Default | Meaning |
|---|---|---|---|
| `-pkg` | `PKG` | `com.saucelabs.mydemoapp.android` | app under test |
| `-model` | `MODEL` | `sonnet` | Claude model for every agent call |
| `-timeout` | `JOURNEY_TIMEOUT` | `600` | seconds per agent call |
| `-max-heal` | `MAX_HEAL` | `3` | heal → re-run cycles per failing journey (`0` disables healing) |
| `-no-cache` | `NO_CACHE=1` | off | re-run journeys even if an identical run already passed |
| `-force` | `FORCE=1` | off | ignore every cache, regenerate features and journeys (implies `-no-cache`) |
| `-reports` | `REPORT_DIR` | `reports` | output folder |
| `-work` / `-features` / `-generated` | `WORK_DIR` / `FEATURE_DIR` / `GENERATED_DIR` | `work` / `features` / `generated` | pipeline folders |
| `-sources` | `SOURCES_FILE` | repo `sources.yaml` | extra source kinds |

Flags go after the command: `./run_journeys.sh run -max-heal 1 emulator-5554 examples/journeys/login.journey.xml`.
`sources.yaml` defaults to the copy at the repo root. Set `SOURCES_FILE` to use another one.

## Sources (auto-detected)

| Source | How it's read |
|---|---|
| `.md .markdown .txt .feature` | as is |
| `.docx .doc .rtf .html .odt` | macOS `textutil` |
| `.pdf` | Claude reads the PDF |
| `notion.so` / `notion.site` link | claude.ai Notion MCP |
| `*.atlassian.net/wiki/…` or `/confluence/` link | claude.ai Atlassian Rovo MCP (read-only) |
| any other `http(s)` link | WebFetch (public pages only) |

**Add another MCP** (Google Docs, Jira, Linear, …) by adding an entry to [`sources.yaml`](sources.yaml). No code change is needed:

```yaml
sources:
  - name: gdocs
    match: '^https://docs\.google\.com/document/'
    allowed_tools: [mcp__claude_ai_Google_Docs, Write]
    hint: run `claude`, then /mcp and authenticate claude.ai Google Docs
```

## Writing a PRD that converts well

See [`examples/login-prd.md`](examples/login-prd.md) and [`examples/checkout-prd.md`](examples/checkout-prd.md).
- **Acceptance criteria:** use a numbered list. Each item becomes one scenario.
- **Labels:** use the real screen titles, button labels, and field names.
- **Test data:** give concrete values, ideally in a table.
- **Preconditions:** state them (e.g. how to log in). They become the `Background`.
- **Outcomes:** make them observable on screen, e.g. "shows Thank you for your order", not "order is saved".

## Journey format

```xml
<journey name="Login with standard user">
    <description>A valid user can log in and land on the products list.</description>
    <actions>
        <action>Tap "Hamburger" menu</action>
        <action>Type "standard_user" in the Username field</action>
        <action>Tap the "LOGIN" button</action>
        <action>Verify that the "PRODUCTS" screen is shown with at least one product listed</action>
    </actions>
</journey>
```

This is the journey format from Android's [Journeys](https://developer.android.com/tools/agents/android-cli/journeys).
Android Studio also has an editor for this format ([Journeys for Android Studio](https://developer.android.com/studio/gemini/journeys)).
The "Tips for writing journeys" on that page also apply to steps generated here, for example: one clear action per step, and include the success criteria.

Each `<action>` is one UI interaction in plain English. An action that starts with
`Verify / Check / Ensure / Assert / Confirm` is an **assertion**, and healing never touches assertions.

## Self-healing

When a journey fails, the healer agent looks at the failing screen and returns one of three verdicts.

| Verdict | Meaning | What happens |
|---|---|---|
| `APP_BUG: …` | the app is wrong (crash, missing feature, false assertion) | no edit; status `APP_BUG` |
| `RERUN: …` | the step actually succeeded; the check was too strict | journey unchanged; re-run |
| edits the journey | the steps don't match the real UI flow | guard check → re-run from a clean app |

The healer may operate the app to discover the exact flow. From the failing action onward, it may reword, insert, or delete **non-assertion** actions: for example, add missing required fields, or use the real button label.

The guard (`internal/journey/guard.go`) rejects the heal and restores the file in either case:
- any action that already passed changed;
- any assertion was added, removed, reworded, or reordered.

Example from a real run: [`examples/journeys/checkout.journey.xml`](examples/journeys/checkout.journey.xml) was healed in 1 cycle.
- The old First/Last Name + CONTINUE steps became the app's real flow: shipping form → To Payment → payment form → Review Order → Place Order.
- All 3 Verify actions are unchanged.

### Statuses

| Status | Meaning |
|---|---|
| `PASS` | passed first time |
| `CACHED` | skipped: this exact journey already passed on the same app build, device model, and OS (see [Speed](#speed-caching-and-parallel-devices)) |
| `HEALED` | passed after ≥1 heal; see `reports/<name>.heal.diff` |
| `FAIL` | still failing after `-max-heal` cycles, or the healer couldn't fix it |
| `APP_BUG` | the healer judged the app to be wrong |
| `HEAL_REJECTED` | the guard rejected the heal; the journey was restored |
| `TIMEOUT` / `ERROR` / `NO_REPORT` / `NO_RESULT` | agent or infrastructure problem; see `reports/<name>.log` |

The exit code is `0` when every journey passed or healed, `1` when any did not, and `2` on a usage or setup error.

### Triage: what to do with each result

| Result | QA action | Dev action |
|---|---|---|
| `PASS` / `CACHED` | nothing | nothing |
| `HEALED` | review `reports/<name>.heal.diff`. If the new steps are right, commit the healed journey. If the app *should* have matched the PRD, file a bug or update the PRD | the UI changed from the spec; confirm it was intended |
| `APP_BUG` | file a bug with `reports/<name>.md` (steps + verdict) and `reports/<name>.png` | reproduce with the steps in the report |
| `FAIL` | read `reports/<name>.md` (which action failed and why) and the heal logs; fix the PRD/feature or the journey wording | check whether a recent change broke the flow |
| `HEAL_REJECTED` | the healer tried to change an assertion or a passed step. Read the heal log: the assertion is probably really failing | treat like `APP_BUG` until proven otherwise |
| `TIMEOUT` / `ERROR` | infrastructure: device offline, agent not logged in, journey too long. See `reports/<name>.log` | — |

## Outputs

| Path | Content |
|---|---|
| `reports/<name>.md` | per-action ✅ / ❌ report |
| `reports/<name>.locators.java` | `public static final By X = AppiumBy...;` per element |
| `reports/<name>.png` | screenshot when not passed |
| `reports/<name>.log`, `<name>.heal.<n>.log` | agent transcripts |
| `reports/<name>.heal.diff` | original → healed journey |
| `reports/.cache/<name>.json` | fingerprint of the last pass (used to skip unchanged journeys) |
| `work/<slug>/prd.md` | normalized PRD (links cached here) |
| `features/<slug>.feature` | generated Gherkin, stamped with the PRD hash |
| `generated/<slug>/*.journey.xml` | generated journeys |

## Speed: caching and parallel devices

Most of the time goes into running journeys on the device (about 1 minute per short journey, several minutes with healing). Two features cut it down.

**1. Result cache: skip what already passed.** After a journey passes (PASS or HEALED), its fingerprint is saved in `reports/.cache/<name>.json`. The fingerprint covers:
- the exact journey file;
- the app's `versionName`, `versionCode`, and install time;
- the device model and Android version;
- the Claude model;
- the run-prompt version.

On the next run, a journey with the same fingerprint is reported as `CACHED` in under a second, and its last report and locators are kept. Anything that changes re-runs automatically: editing the journey, installing a new build, using another device type, or upgrading this tool. Failures are never cached. Use `-no-cache` to re-run everything.

Measured on an emulator: `login` took 1m15s the first time and 0.3s when cached.

**2. Parallel devices: spread journeys across several devices.** Pass a comma-separated list of serials:

```bash
./run_journeys.sh run emulator-5554,emulator-5556,R58M123ABC generated/checkout-prd
```

Each device takes the next journey from a shared queue. With N devices the run is up to N× faster. Output is printed per journey as each one finishes, and the summary keeps the input order and shows which device ran each journey. All devices need the app installed.

The other stages are cached too (`-force` resets them):
- link sources are fetched once into `work/<slug>/prd.md`;
- a feature is regenerated only when the PRD content changes;
- existing journeys are never overwritten, so healed journeys survive regeneration.

## Run in CI (sketch)

The CLI exits `0` when every journey passed, so it works as a CI step. You need an emulator, the app APK, and a Claude API key (`ANTHROPIC_API_KEY` as a secret, so `claude -p` runs non-interactively).
Here's an outline for GitHub Actions; adapt it to your pipeline:

```yaml
- uses: reactivecircus/android-emulator-runner@v2
  env:
    ANTHROPIC_API_KEY: ${{ secrets.ANTHROPIC_API_KEY }}
  with:
    api-level: 34
    script: |
      adb install app/build/outputs/apk/debug/app-debug.apk
      ./run_journeys.sh run -pkg com.your.app emulator-5554 journeys/
- uses: actions/upload-artifact@v4
  if: always()
  with: { name: journey-reports, path: reports/ }
```

Install the Android CLI and Claude Code (plus the `android-cli` plugin) in an earlier step, as in [Setup](#setup).
For pull requests, run a small smoke subset, and keep the full suite for nightly runs: it's AI-driven, so it takes minutes, not seconds.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `device … is not online` | `adb devices`; start the emulator; pass the right serial |
| `… is not installed` | install the app or pass `-pkg` |
| `notion source: could not fetch …` | run `claude` → `/mcp` → authenticate Notion; check `work/<slug>/ingest.log` |
| `no source kind matches` | use a supported file type or add a kind to `sources.yaml` |
| `TIMEOUT` | raise `-timeout`; long journeys plus healing take minutes |
| A journey shows `CACHED` but you want it re-run | `-no-cache`, or delete `reports/.cache/<name>.json` |
| `journeys … would share reports/…` | two journeys have the same file name in different folders; rename one |
| Generated journey looks wrong | edit the `.feature` file, then `convert -force` |
| Agent gets stuck on login (OTP, captcha) | use a test build or account without them; the agent can't read SMS or solve captchas |
| Results differ between runs (flaky) | make steps more specific, and add success criteria to each step ("…this opens the cart"); try `-model opus` for tricky screens |

## Project layout

```
cmd/journeys/        CLI entry point (flags, subcommands, wiring)
internal/common/     shared kernel: proc (exec), claude (headless CLI client), adb
internal/source/     document ingestion + source-kind registry (sources.yaml)
internal/journey/    journey model, XML codec, heal guard
internal/gherkin/    PRD → feature, feature → journeys (cucumber parser expands Background/Outline)
internal/runner/     run + heal loop, parallel device lanes, result cache, report parsing, summary
examples/            example PRDs + sample journeys
run_journeys.sh      launcher (builds and runs the CLI)
sources.yaml         extra document source kinds
```

Each `internal/` package is one bounded context, with files named by role (`service.go`, `prompts.go`, …).
Interfaces are declared by the package that uses them, so the heal loop is tested with fake agent and device.

## Development

```bash
make check     # gofmt + go vet + go test; no device or Claude calls needed (CI runs this)
make build     # bin/journeys
```

- **Change the agent's instructions:** edit `internal/gherkin/prompts.go` or `internal/runner/prompts.go`.
- **Change what counts as a safe heal:** edit `internal/journey/guard.go` and add a case to `journey_test.go`.
- **Module path:** `github.com/dikako/docs-to-journeys`.

### Releasing

Push a version tag. [`.github/workflows/release.yml`](.github/workflows/release.yml) then:
1. runs `make check`;
2. builds the four platform archives with the version stamped in (`journeys version`);
3. publishes a GitHub release with generated notes and `checksums.txt`.

```bash
git tag v1.0.0 && git push origin v1.0.0       # release
git tag v1.1.0-rc.1 && git push origin v1.1.0-rc.1   # tags with a "-" are marked pre-release
```

### Why Go instead of a bash script?

Bash suits linear glue: call `adb` and `claude` in order, then grep a status. This pipeline needs more than that:
- structured parsing (Gherkin, XML, reports);
- a pluggable source registry;
- a heal → guard → re-run state machine;
- content-hash caching;
- portable timeouts and Ctrl-C cleanup;
- unit tests.

So the Go CLI holds all the logic, and `run_journeys.sh` is a thin launcher.

## License

[MIT](LICENSE) © dikako

## Keywords

Android UI testing · AI test automation · Android Journeys · Android CLI · Claude Code · Gherkin / BDD · PRD to test cases · exploratory testing ·
self-healing tests · test generation from requirements · Appium locators · Notion / Confluence to tests · QA automation

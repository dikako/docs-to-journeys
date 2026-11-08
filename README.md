# docs-to-journeys

Turn a product document (Markdown, Word, PDF, Notion, Confluence, …) into Gherkin, then into
Android UI journeys that an AI agent runs on a real device or emulator. When a journey's steps
don't match the app's real flow, the runner **self-heals** the journey and runs it again. It
never changes the journey's assertions.

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
git clone <this repo> docs-to-journeys && cd docs-to-journeys

# 1. Run the sample journeys (no PRD needed)
./run_journeys.sh run emulator-5554 examples/journeys

# 2. Full pipeline from an example PRD
./run_journeys.sh all examples/login-prd.md    emulator-5554
./run_journeys.sh all examples/checkout-prd.md emulator-5554
```

`run_journeys.sh` builds the Go CLI into `bin/journeys`, both on first use and whenever the sources change. It then forwards all arguments. You can run it from any folder: `reports/`, `work/`, `features/` and `generated/` are created in the current directory, and those folders are git-ignored.

### Install from a release (no Go needed)

Download the archive for your platform from the GitHub **Releases** page and unpack it.
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
- **Module path:** the module is `docs-to-journeys`. To make it `go install`-able from GitHub, run `go mod edit -module github.com/<owner>/docs-to-journeys`, then update the import paths to match.

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

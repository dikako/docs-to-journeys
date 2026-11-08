// Package adb drives one Android device through the adb CLI. It is part of the
// shared kernel and holds no test logic.
package adb

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/dikako/docs-to-journeys/internal/common/proc"
)

var animKeys = []string{"window_animation_scale", "transition_animation_scale", "animator_duration_scale"}

// Device is one adb-connected device.
type Device struct {
	Serial string
}

func (d Device) out(ctx context.Context, args ...string) (string, error) {
	s, err := proc.Output(ctx, "adb", append([]string{"-s", d.Serial}, args...)...)
	return strings.TrimSpace(strings.ReplaceAll(s, "\r", "")), err
}

// Prepare checks the device and app, resolves the launcher activity and turns
// animations off. The returned restore func puts the animation scales back.
func (d Device) Prepare(ctx context.Context, pkg string) (activity string, restore func(), err error) {
	if st, _ := d.out(ctx, "get-state"); st != "device" {
		return "", nil, fmt.Errorf("device %s is not online", d.Serial)
	}
	if p, _ := d.out(ctx, "shell", "pm", "path", pkg); p == "" {
		return "", nil, fmt.Errorf("%s is not installed on %s", pkg, d.Serial)
	}
	res, err := d.out(ctx, "shell", "cmd", "package", "resolve-activity", "--brief", pkg)
	if err != nil {
		return "", nil, err
	}
	lines := strings.Split(res, "\n")
	activity = strings.TrimSpace(lines[len(lines)-1])
	if !strings.Contains(activity, "/") {
		return "", nil, fmt.Errorf("could not resolve launcher activity for %s (got: %s)", pkg, activity)
	}

	orig := make([]string, len(animKeys))
	for i, k := range animKeys {
		orig[i], _ = d.out(ctx, "shell", "settings", "get", "global", k)
		_, _ = d.out(ctx, "shell", "settings", "put", "global", k, "0")
	}
	restore = func() {
		// Use a fresh context: the run context may already be cancelled.
		bg := context.Background()
		for i, k := range animKeys {
			v := orig[i]
			if v == "" || v == "null" {
				v = "1"
			}
			_, _ = d.out(bg, "shell", "settings", "put", "global", k, v)
		}
	}
	return activity, restore, nil
}

// ClearAndLaunch wipes app data and starts the launcher activity, waiting for it.
func (d Device) ClearAndLaunch(ctx context.Context, pkg, activity string) error {
	if _, err := d.out(ctx, "shell", "pm", "clear", pkg); err != nil {
		return fmt.Errorf("pm clear: %w", err)
	}
	if _, err := d.out(ctx, "shell", "am", "start", "-W", "-n", activity); err != nil {
		return fmt.Errorf("am start: %w", err)
	}
	return nil
}

// Screencap writes a PNG of the current screen to path.
func (d Device) Screencap(ctx context.Context, path string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = proc.Run(ctx, proc.Cmd{Name: "adb", Args: []string{"-s", d.Serial, "exec-out", "screencap", "-p"}, Stdout: f})
	return err
}

// Env describes what a test result depends on besides the test itself: the
// installed app version and the device model and OS. Missing values are left
// as "?" so the string is still stable.
func (d Device) Env(ctx context.Context, pkg string) string {
	get := func(args ...string) string {
		v, err := d.out(ctx, args...)
		if err != nil || v == "" {
			return "?"
		}
		return v
	}
	// lastUpdateTime catches reinstalls of a debug build with the same version.
	fields := map[string]string{"versionName": "?", "versionCode": "?", "lastUpdateTime": "?"}
	if dump, err := d.out(ctx, "shell", "dumpsys", "package", pkg); err == nil {
		for _, l := range strings.Split(dump, "\n") {
			l = strings.TrimSpace(l)
			if v, ok := strings.CutPrefix(l, "lastUpdateTime="); ok && fields["lastUpdateTime"] == "?" {
				fields["lastUpdateTime"] = v // value contains a space
				continue
			}
			for _, f := range strings.Fields(l) {
				k, v, ok := strings.Cut(f, "=")
				if cur, want := fields[k]; ok && want && cur == "?" {
					fields[k] = v
				}
			}
		}
	}
	version := fields["versionName"] + "(" + fields["versionCode"] + ")"
	updated := fields["lastUpdateTime"]
	return fmt.Sprintf("app=%s %s updated=%s model=%s android=%s", pkg, version, updated,
		get("shell", "getprop", "ro.product.model"), get("shell", "getprop", "ro.build.version.release"))
}

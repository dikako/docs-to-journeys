// Package proc runs external commands with a context deadline and an optional
// log file. It is part of the shared kernel and imports no bounded context.
package proc

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"time"
)

// Result is the outcome of a finished command.
type Result struct {
	Stdout   string
	ExitCode int
	TimedOut bool
}

// Cmd describes one command invocation.
type Cmd struct {
	Name string
	Args []string
	// LogPath, when set, receives stdout and stderr. It is truncated first.
	LogPath string
	// Stdout, when set, also receives stdout (e.g. a screenshot file).
	Stdout io.Writer
}

// Run executes c and waits for it. A non-zero exit is reported in
// Result.ExitCode, not as an error. err is only for failures to start the
// process or open the log.
func Run(ctx context.Context, c Cmd) (Result, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	var out bytes.Buffer
	stdout := []io.Writer{&out}
	var stderr io.Writer = io.Discard
	if c.LogPath != "" {
		f, err := os.Create(c.LogPath)
		if err != nil {
			return Result{}, err
		}
		defer f.Close()
		stdout = append(stdout, f)
		stderr = f
	}
	if c.Stdout != nil {
		// Raw stdout (possibly binary) goes only to the caller's writer.
		stdout = []io.Writer{c.Stdout}
	}
	cmd.Stdout = io.MultiWriter(stdout...)
	cmd.Stderr = stderr

	err := cmd.Run()
	res := Result{Stdout: out.String()}
	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = true
		res.ExitCode = 124
		return res, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		res.ExitCode = exitErr.ExitCode()
		return res, nil
	}
	return res, err
}

// Output runs a short command and returns its stdout, failing on non-zero exit.
func Output(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		if stderr.Len() > 0 {
			return string(out), errors.New(err.Error() + ": " + stderr.String())
		}
		return string(out), err
	}
	return string(out), nil
}

// Require returns an error naming the first command missing from PATH.
func Require(names ...string) error {
	for _, n := range names {
		if _, err := exec.LookPath(n); err != nil {
			return errors.New(n + " not found in PATH")
		}
	}
	return nil
}

// Stream runs c and calls onLine for every stdout line as it arrives, so a
// caller can persist output live (it survives a timeout kill). Lines can be
// large; there is no length limit. Stderr goes to c.LogPath if set.
func Stream(ctx context.Context, c Cmd, onLine func(line []byte)) (Result, error) {
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	// After a kill, child processes may still hold stdout open; stop waiting
	// for them after a grace period.
	cmd.WaitDelay = 5 * time.Second
	if c.LogPath != "" {
		f, err := os.Create(c.LogPath)
		if err != nil {
			return Result{}, err
		}
		defer f.Close()
		cmd.Stderr = f
	}
	lw := &lineWriter{onLine: onLine}
	cmd.Stdout = lw
	err := cmd.Run()
	lw.flush()

	if ctx.Err() == context.DeadlineExceeded {
		return Result{ExitCode: 124, TimedOut: true}, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return Result{ExitCode: exitErr.ExitCode()}, nil
	}
	if errors.Is(err, exec.ErrWaitDelay) {
		return Result{}, nil
	}
	return Result{}, err
}

// lineWriter splits a byte stream into lines.
type lineWriter struct {
	buf    []byte
	onLine func([]byte)
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		i := bytes.IndexByte(w.buf, '\n')
		if i < 0 {
			return len(p), nil
		}
		w.emit(w.buf[:i])
		w.buf = w.buf[i+1:]
	}
}

func (w *lineWriter) flush() {
	w.emit(w.buf)
	w.buf = nil
}

func (w *lineWriter) emit(line []byte) {
	line = bytes.TrimRight(line, "\r")
	if len(bytes.TrimSpace(line)) > 0 {
		w.onLine(append([]byte(nil), line...))
	}
}

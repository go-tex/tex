package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunReadsAFileArgument(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "doc.tex")
	if err := os.WriteFile(p, []byte(`\begin{document}\section{Hello}\end{document}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	var out, errBuf bytes.Buffer
	if code := run([]string{p}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("run = %d, want 0 (stderr: %q)", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "Hello") {
		t.Errorf("rendered output does not mention the section title: %q", out.String())
	}
	if errBuf.Len() != 0 {
		t.Errorf("stderr = %q, want empty", errBuf.String())
	}
}

func TestRunReadsStdinWhenGivenNoArguments(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run(nil, strings.NewReader(`\begin{document}\section{FromStdin}\end{document}`), &out, &errBuf); code != 0 {
		t.Fatalf("run = %d, want 0 (stderr: %q)", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "FromStdin") {
		t.Errorf("rendered output does not mention the section title: %q", out.String())
	}
}

func TestRunReportsAnUnreadableFile(t *testing.T) {
	var out, errBuf bytes.Buffer
	missing := filepath.Join(t.TempDir(), "does-not-exist.tex")
	if code := run([]string{missing}, strings.NewReader(""), &out, &errBuf); code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.HasPrefix(errBuf.String(), "gotex: ") {
		t.Errorf("stderr = %q, want it to start with %q", errBuf.String(), "gotex: ")
	}
	if out.Len() != 0 {
		t.Errorf("stdout = %q, want empty when the input could not be read", out.String())
	}
}

// failingReader fails on the first Read, which is how a broken pipe on stdin
// reaches run.
type failingReader struct{}

func (failingReader) Read([]byte) (int, error) { return 0, errors.New("pipe broke") }

func TestRunReportsAnUnreadableStdin(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run(nil, failingReader{}, &out, &errBuf); code != 1 {
		t.Fatalf("run = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "pipe broke") {
		t.Errorf("stderr = %q, want it to carry the reader's error", errBuf.String())
	}
}

// TestMainExitsWithRunsCode covers main itself through the osExit seam, so the
// wiring between main and run is asserted rather than assumed.
func TestMainExitsWithRunsCode(t *testing.T) {
	origExit, origArgs, origStdin := osExit, os.Args, os.Stdin
	t.Cleanup(func() { osExit, os.Args, os.Stdin = origExit, origArgs, origStdin })

	got := -1
	osExit = func(code int) { got = code }

	missing := filepath.Join(t.TempDir(), "nope.tex")
	os.Args = []string{"gotex", missing}
	main()
	if got != 1 {
		t.Errorf("main exited %d for an unreadable file, want 1", got)
	}

	dir := t.TempDir()
	p := filepath.Join(dir, "ok.tex")
	if err := os.WriteFile(p, []byte(`\begin{document}\section{Ok}\end{document}`), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	os.Args = []string{"gotex", p}
	main()
	if got != 0 {
		t.Errorf("main exited %d for a readable file, want 0", got)
	}
}

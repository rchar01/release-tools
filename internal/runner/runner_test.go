package runner

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestIsExecutable(t *testing.T) {
	r := OSRunner{}
	dir := t.TempDir()
	path := filepath.Join(dir, "tool")
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && r.IsExecutable(path) {
		t.Fatal("non-executable file reported executable")
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if !r.IsExecutable(path) {
		t.Fatal("executable file reported non-executable")
	}
	if r.IsExecutable(dir) {
		t.Fatal("directory reported executable")
	}
}

func TestAttachedRunnerCommand(t *testing.T) {
	r := AttachedRunner{Stdout: os.Stdout, Stderr: os.Stderr}
	cmd := r.Command("/tmp", "tool", "arg")
	if cmd.Dir != "/tmp" {
		t.Fatalf("Dir = %q, want /tmp", cmd.Dir)
	}
	if cmd.Name != "tool" {
		t.Fatalf("Name = %q, want tool", cmd.Name)
	}
	if len(cmd.Args) != 1 || cmd.Args[0] != "arg" {
		t.Fatalf("Args = %#v, want [arg]", cmd.Args)
	}
	if cmd.Stdout != os.Stdout {
		t.Fatal("Stdout was not wired")
	}
	if cmd.Stderr != os.Stderr {
		t.Fatal("Stderr was not wired")
	}
}

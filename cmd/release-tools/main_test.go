package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadConfigFileKeepsEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".release-tools.env")
	if err := os.WriteFile(config, []byte("RELEASE_PROJECT=file\nRELEASE_OWNER=owner\nRELEASE_TOOLS_VERSION=\"v2.1.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := newApp([]string{"RELEASE_REPO_ROOT=" + dir, "RELEASE_PROJECT=env"}, ioDiscard(), ioDiscard())
	if err != nil {
		t.Fatal(err)
	}

	if got := a.env["RELEASE_PROJECT"]; got != "env" {
		t.Fatalf("RELEASE_PROJECT = %q, want env", got)
	}
	if got := a.env["RELEASE_TOOLS_VERSION"]; got != "v2.1.0" {
		t.Fatalf("RELEASE_TOOLS_VERSION = %q, want v2.1.0", got)
	}
}

func TestLoadConfigFileRejectsUnsupportedKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".release-tools.env"), []byte("VERSION=v1.2.3\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := newApp([]string{"RELEASE_REPO_ROOT=" + dir}, ioDiscard(), ioDiscard())
	if err == nil {
		t.Fatal("expected unsupported key error")
	}
}

func TestExtractNewsSection(t *testing.T) {
	content := `# News

## v2.1.0 - 2026-06-11

- add Go CLI
- keep command compatibility

## v2.0.0 - 2026-06-11

- previous
`

	got := extractNewsSection(content, "v2.1.0")
	want := "- add Go CLI\n- keep command compatibility"
	if got != want {
		t.Fatalf("section = %q, want %q", got, want)
	}
}

func TestOptionalVersionArgumentRejectsTooManyArgs(t *testing.T) {
	a := &app{env: map[string]string{}}
	_, err := a.optionalVersionArg("notes", []string{"v1.0.0", "v1.0.1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func ioDiscard() *bytes.Buffer {
	return &bytes.Buffer{}
}

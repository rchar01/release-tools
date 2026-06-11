package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfigFileKeepsEnvironmentOverride(t *testing.T) {
	dir := t.TempDir()
	config := filepath.Join(dir, ".release-tools.env")
	if err := os.WriteFile(config, []byte("RELEASE_PROJECT=file\nRELEASE_OWNER=owner\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	a, err := newApp([]string{"RELEASE_REPO_ROOT=" + dir, "RELEASE_PROJECT=env"}, ioDiscard(), ioDiscard())
	if err != nil {
		t.Fatal(err)
	}

	if got := a.env["RELEASE_PROJECT"]; got != "env" {
		t.Fatalf("RELEASE_PROJECT = %q, want env", got)
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

## v2.2.0 - 2026-06-11

- add Go CLI
- keep command compatibility

## v2.1.0 - 2026-06-11

- previous
`

	got := extractNewsSection(content, "v2.2.0")
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

func TestCloneTagUsesFullHistoryAndDetachesAnnotatedTag(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	clone := filepath.Join(dir, "clone")
	if err := os.Mkdir(repo, 0o755); err != nil {
		t.Fatal(err)
	}
	runGit(t, repo, "init", "--quiet")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	runGit(t, repo, "config", "user.name", "Test User")
	writeFile(t, filepath.Join(repo, "file.txt"), "one\n")
	runGit(t, repo, "add", "file.txt")
	runGit(t, repo, "commit", "--quiet", "-m", "first")
	runGit(t, repo, "tag", "v1.0.0")
	writeFile(t, filepath.Join(repo, "file.txt"), "two\n")
	runGit(t, repo, "commit", "--quiet", "-am", "second")
	runGit(t, repo, "tag", "-a", "v2.0.0", "-m", "v2.0.0")

	a := &app{repoRoot: repo, stdout: ioDiscard(), stderr: ioDiscard()}
	if err := a.cloneTag("v2.0.0", clone); err != nil {
		t.Fatal(err)
	}

	if got := strings.TrimSpace(gitOutput(t, clone, "rev-parse", "--is-shallow-repository")); got != "false" {
		t.Fatalf("shallow repository = %q, want false", got)
	}
	if got := strings.TrimSpace(gitOutput(t, clone, "tag", "--list", "v1.0.0")); got != "v1.0.0" {
		t.Fatalf("previous tag = %q, want v1.0.0", got)
	}
	if got, want := strings.TrimSpace(gitOutput(t, clone, "rev-parse", "HEAD")), strings.TrimSpace(gitOutput(t, repo, "rev-parse", "refs/tags/v2.0.0^{}")); got != want {
		t.Fatalf("HEAD = %q, want %q", got, want)
	}
}

func ioDiscard() *bytes.Buffer {
	return &bytes.Buffer{}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s failed: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitOutput(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("git %s failed: %v", strings.Join(args, " "), err)
	}
	return string(output)
}

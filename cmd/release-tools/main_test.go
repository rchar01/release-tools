package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

## v3.0.0 - 2026-06-12

- add Go CLI
- keep command compatibility

## v2.2.0 - 2026-06-11

- previous
`

	got := extractNewsSection(content, "v3.0.0")
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

func TestVersionCommandPrintsReleaseToolsVersion(t *testing.T) {
	previous := releaseToolsVersion
	releaseToolsVersion = "test-version"
	t.Cleanup(func() { releaseToolsVersion = previous })

	var stdout bytes.Buffer
	a := &app{env: map[string]string{}, stdout: &stdout, stderr: ioDiscard()}
	if err := a.run([]string{"--version"}); err != nil {
		t.Fatal(err)
	}
	if got := stdout.String(); got != "release-tools test-version\n" {
		t.Fatalf("version output = %q", got)
	}
}

func TestHelpVersionAndCompletionDoNotLoadConfig(t *testing.T) {
	previous := releaseToolsVersion
	releaseToolsVersion = "test-version"
	t.Cleanup(func() { releaseToolsVersion = previous })

	for _, args := range [][]string{
		{"--help"},
		{"help"},
		{"-v"},
		{"--version"},
		{"version"},
		{"completion", "bash"},
	} {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			environ := []string{"RELEASE_CONFIG_FILE=/missing/release-tools.env"}
			if err := executeCLI(t.Context(), environ, &stdout, &stderr, args); err != nil {
				t.Fatalf("executeCLI() error = %v, stderr = %q", err, stderr.String())
			}
			if stdout.Len() == 0 {
				t.Fatal("expected command output")
			}
		})
	}
}

func TestUnknownCommandReportsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	err := executeCLI(t.Context(), nil, &stdout, &stderr, []string{"wat"})
	if err == nil {
		t.Fatal("expected unknown command error")
	}
	if !strings.Contains(stderr.String(), "[ERROR]") {
		t.Fatalf("stderr = %q, want [ERROR]", stderr.String())
	}
	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("error = %q, want unknown command", err.Error())
	}
}

func TestParseGoreleaserVersion(t *testing.T) {
	output := `goreleaser: Release engineering, simplified.

GitVersion:    v2.16.0
GitCommit:     unknown
`
	if got := parseGoreleaserVersion(output); got != "v2.16.0" {
		t.Fatalf("parseGoreleaserVersion = %q, want v2.16.0", got)
	}
}

func TestResolveTokenMapsForgeNativeEnvironment(t *testing.T) {
	a := &app{env: map[string]string{"RELEASE_FORGE": "github", "GITHUB_TOKEN": "github-token"}}
	token, err := a.resolveToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "github-token" {
		t.Fatalf("token = %q, want github-token", token)
	}
	if got := a.goreleaserTokenEnv(); got != "GITHUB_TOKEN" {
		t.Fatalf("token env = %q, want GITHUB_TOKEN", got)
	}

	a = &app{env: map[string]string{"RELEASE_FORGE": "gitlab", "RELEASE_TOKEN": "release-token", "GITLAB_TOKEN": "gitlab-token"}}
	token, err = a.resolveToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "release-token" {
		t.Fatalf("token = %q, want release-token", token)
	}
	if got := a.goreleaserTokenEnv(); got != "GITLAB_TOKEN" {
		t.Fatalf("token env = %q, want GITLAB_TOKEN", got)
	}
}

func TestReleaseForgeAliasesUseGiteaCompatibility(t *testing.T) {
	for _, forgeName := range []string{"codeberg", "forgejo"} {
		t.Run(forgeName, func(t *testing.T) {
			a := &app{env: map[string]string{"RELEASE_FORGE": forgeName}}
			forge, err := a.releaseForge()
			if err != nil {
				t.Fatal(err)
			}
			if forge != forgeGitea {
				t.Fatalf("releaseForge() = %q, want %q", forge, forgeGitea)
			}
			if got := a.releaseForgeName(); got != forgeName {
				t.Fatalf("releaseForgeName() = %q, want %q", got, forgeName)
			}
			if got := a.goreleaserTokenEnv(); got != "GITEA_TOKEN" {
				t.Fatalf("goreleaserTokenEnv() = %q, want GITEA_TOKEN", got)
			}
			if got := a.releaseAPIURL(); got != "https://codeberg.org/api/v1" {
				t.Fatalf("releaseAPIURL() = %q, want Codeberg API URL", got)
			}
		})
	}
}

func TestResolveTokenReadsTokenFile(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	writeFile(t, tokenFile, "file-token\r\n")

	a := &app{env: map[string]string{"RELEASE_FORGE": "gitea", "RELEASE_TOKEN_FILE": tokenFile}}
	token, err := a.resolveToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "file-token" {
		t.Fatalf("token = %q, want file-token", token)
	}
}

func TestResolveTokenEnvironmentPrecedesTokenFile(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	writeFile(t, tokenFile, "file-token\n")

	a := &app{env: map[string]string{
		"RELEASE_FORGE":      "gitea",
		"GITEA_TOKEN":        "native-token",
		"RELEASE_TOKEN_FILE": tokenFile,
	}}
	token, err := a.resolveToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "native-token" {
		t.Fatalf("token = %q, want native-token", token)
	}

	a.env["RELEASE_TOKEN"] = "release-token"
	token, err = a.resolveToken()
	if err != nil {
		t.Fatal(err)
	}
	if token != "release-token" {
		t.Fatalf("token = %q, want release-token", token)
	}
}

func TestResolveOptionalTokenDoesNotReadTokenFile(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "token")
	writeFile(t, tokenFile, "file-token\n")

	a := &app{env: map[string]string{"RELEASE_FORGE": "gitea", "RELEASE_TOKEN_FILE": tokenFile}}
	if token, ok := a.resolveOptionalToken(); ok {
		t.Fatalf("optional token = %q, want no token", token)
	}
}

func TestExpandTokenFilePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		t.Skip("home directory unavailable")
	}

	tests := map[string]string{
		"~/token":       filepath.Join(home, "token"),
		"$HOME/token":   filepath.Join(home, "token"),
		"${HOME}/token": filepath.Join(home, "token"),
		"/tmp/token":    "/tmp/token",
	}
	for input, want := range tests {
		if got := expandTokenFilePath(input); got != want {
			t.Fatalf("expandTokenFilePath(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestUpdateGitHubReleaseBody(t *testing.T) {
	notesFile := filepath.Join(t.TempDir(), "notes.md")
	writeFile(t, notesFile, "hello github\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/repo/releases/tags/v1.0.0":
			if got := r.Header.Get("Authorization"); got != "Bearer token" {
				t.Fatalf("authorization = %q", got)
			}
			_, _ = w.Write([]byte(`{"id":42}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/owner/repo/releases/42":
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if got := payload["body"]; got != "hello github\n" {
				t.Fatalf("body = %q", got)
			}
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()

	a := &app{env: map[string]string{
		"RELEASE_FORGE":     "github",
		"RELEASE_API_URL":   server.URL,
		"RELEASE_OWNER":     "owner",
		"RELEASE_REPO":      "repo",
		"RELEASE_BODY_MODE": "patch",
	}, stdout: ioDiscard(), stderr: ioDiscard()}
	if err := a.updateReleaseBody("v1.0.0", notesFile, "token"); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateGitLabReleaseBody(t *testing.T) {
	notesFile := filepath.Join(t.TempDir(), "notes.md")
	writeFile(t, notesFile, "hello gitlab\n")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.EscapedPath() != "/projects/owner%2Frepo/releases/v1.0.0" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.EscapedPath())
		}
		if got := r.Header.Get("PRIVATE-TOKEN"); got != "token" {
			t.Fatalf("PRIVATE-TOKEN = %q", got)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if got := payload["description"]; got != "hello gitlab\n" {
			t.Fatalf("description = %q", got)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	a := &app{env: map[string]string{
		"RELEASE_FORGE":     "gitlab",
		"RELEASE_API_URL":   server.URL,
		"RELEASE_OWNER":     "owner",
		"RELEASE_REPO":      "repo",
		"RELEASE_BODY_MODE": "patch",
	}, stdout: ioDiscard(), stderr: ioDiscard()}
	if err := a.updateReleaseBody("v1.0.0", notesFile, "token"); err != nil {
		t.Fatal(err)
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

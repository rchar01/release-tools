package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"codeberg.org/rch/release-tools/internal/runner"
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

	got := extractNewsSection(content, "v3.0.0", "news-md")
	want := "- add Go CLI\n- keep command compatibility"
	if got != want {
		t.Fatalf("section = %q, want %q", got, want)
	}
}

func TestGenerateMarkdownNewsWritesExactReleaseNotes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "NEWS.md"), `# News

## v1.2.3 - 2026-07-02

- add Markdown release note
- keep existing behavior

## v1.2.2 - 2026-06-01

- previous release
`)

	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"RELEASE_NOTES_MODE":   "news-md",
			"RELEASE_NOTES_SOURCE": "NEWS.md",
		},
	}
	notesFile, err := a.generateNotes("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(notesFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "- add Markdown release note\n- keep existing behavior\n"
	if got := string(content); got != want {
		t.Fatalf("generated notes = %q, want %q", got, want)
	}
}

func TestGenerateGNUNewsWritesExactReleaseNotes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "NEWS.md"), `release-tools NEWS -- history of user-visible changes.

* Noteworthy changes in release 1.2.3 (2026-07-02)

** New features

  - Add GNU release note parsing.
  - Keep Markdown release notes unchanged.

* Noteworthy changes in release 1.2.2 (2026-06-01)

** New features

  - Previous release.
`)

	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"RELEASE_NOTES_MODE":   "gnu-news",
			"RELEASE_NOTES_SOURCE": "NEWS.md",
		},
	}
	notesFile, err := a.generateNotes("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(notesFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "** New features\n\n  - Add GNU release note parsing.\n  - Keep Markdown release notes unchanged.\n"
	if got := string(content); got != want {
		t.Fatalf("generated notes = %q, want %q", got, want)
	}
}

func TestGenerateGNUNewsMatchesVPrefixedHeading(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "NEWS.md"), `release-tools NEWS -- history of user-visible changes.

* Noteworthy changes in release v1.2.3 (2026-07-02)

** New features

  - Match GNU headings with v-prefixed versions.

* Noteworthy changes in release v1.2.2 (2026-06-01)

** New features

  - Previous release.
`)

	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"RELEASE_NOTES_MODE":   "gnu-news",
			"RELEASE_NOTES_SOURCE": "NEWS.md",
		},
	}
	notesFile, err := a.generateNotes("v1.2.3")
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(notesFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "** New features\n\n  - Match GNU headings with v-prefixed versions.\n"
	if got := string(content); got != want {
		t.Fatalf("generated notes = %q, want %q", got, want)
	}
}

func TestExtractGNUNewsSectionMatchesSupportedHeadings(t *testing.T) {
	content := strings.Join([]string{
		"* Noteworthy changes in release v1.2.3 (2026-07-02)",
		"",
		"** New features",
		"",
		"  - Add GNU NEWS parsing.",
		"",
		"* Noteworthy changes in release v1.2.2 (2026-06-01)",
		"",
		"  - Previous.",
		"",
	}, "\r\n")
	got := extractNewsSection(content, "1.2.3", "gnu-news")
	want := "** New features\n\n  - Add GNU NEWS parsing."
	if got != want {
		t.Fatalf("section = %q, want %q", got, want)
	}

	content = strings.Replace(content, "release v1.2.3", "release 1.2.3", 1)
	got = extractNewsSection(content, "v1.2.3", "gnu-news")
	if got != want {
		t.Fatalf("section without v prefix = %q, want %q", got, want)
	}
}

func TestDoctorAcceptsGNUNewsMode(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), "version: 2\n")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "* Noteworthy changes in release 1.2.3 (2026-07-02)\n")

	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_PROJECT":      "demo",
			"RELEASE_OWNER":        "owner",
			"RELEASE_NOTES_MODE":   "gnu-news",
			"GORELEASER_CONFIG":    ".goreleaser.yaml",
			"GORELEASER_BIN":       "/bin/true",
			"RELEASE_REQUIRE_GO":   "0",
			"RELEASE_BODY_MODE":    "none",
			"RELEASE_NOTES_SOURCE": "NEWS.md",
		},
		stdout: ioDiscard(),
		stderr: ioDiscard(),
	}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseArtifactsDefaultToBinaries(t *testing.T) {
	a := &app{env: map[string]string{}}
	artifacts, err := a.releaseArtifacts()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(artifacts, ","); got != "binaries" {
		t.Fatalf("artifacts = %q, want binaries", got)
	}
	enabled, err := a.chartsEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if enabled {
		t.Fatal("charts should not be enabled by default")
	}
}

func TestReleaseArtifactsParseCommaWhitespaceAndDeduplicate(t *testing.T) {
	a := &app{env: map[string]string{"RELEASE_ARTIFACTS": " charts, binaries , charts "}}
	artifacts, err := a.releaseArtifacts()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(artifacts, ","); got != "charts,binaries" {
		t.Fatalf("artifacts = %q, want charts,binaries", got)
	}
	enabled, err := a.chartsEnabled()
	if err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("charts should be enabled")
	}
}

func TestReleaseArtifactsRejectInvalidValues(t *testing.T) {
	for _, value := range []string{"containers", "binaries,,charts", "", "   "} {
		t.Run(value, func(t *testing.T) {
			a := &app{env: map[string]string{"RELEASE_ARTIFACTS": value}}
			if _, err := a.releaseArtifacts(); err == nil {
				t.Fatal("expected artifact parsing error")
			}
		})
	}
}

func TestDoctorReportsArtifacts(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), "version: 2\n")
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	var stdout bytes.Buffer
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_PROJECT":             "demo",
			"RELEASE_OWNER":               "owner",
			"RELEASE_ARTIFACTS":           "binaries, charts",
			"RELEASE_HELM_CHART_DIRS":     "charts/demo",
			"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts",
			"RELEASE_NOTES_MODE":          "none",
			"RELEASE_BODY_MODE":           "none",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: &fakeCommandRunner{combinedOutput: []byte("GitVersion: v2.16.0\n")},
		stdout:   &stdout,
		stderr:   ioDiscard(),
	}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "[INFO] Artifacts: binaries, charts\n") {
		t.Fatalf("doctor output = %q, want artifact line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[INFO] Helm chart dirs: charts/demo\n") {
		t.Fatalf("doctor output = %q, want chart dir line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[INFO] Helm OCI repository: oci://registry.example/charts\n") {
		t.Fatalf("doctor output = %q, want OCI repository line", stdout.String())
	}
}

func TestCheckRunsHelmForCharts(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_ARTIFACTS":       "binaries,charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
			"GORELEASER_BIN":          "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.check(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/tools/goreleaser --config .goreleaser.yaml check",
		"/fake/helm dependency update --skip-refresh charts/demo",
		"/fake/helm lint charts/demo",
	}
	if got := commandStrings(fake.runCommands); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestSnapshotPackagesHelmCharts(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	fake := &fakeCommandRunner{output: []byte("v1.2.3\n")}
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
			"GORELEASER_BIN":          "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.snapshot(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/tools/goreleaser --config .goreleaser.yaml release --snapshot --skip=publish --clean",
		"/fake/helm package charts/demo --version 1.2.3 --app-version 1.2.3 --destination dist/charts",
	}
	if got := commandStrings(fake.runCommands); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist", "charts")); err != nil {
		t.Fatalf("dist/charts was not created: %v", err)
	}
}

func TestPublishPackagesHelmChartsBeforeGoreleaser(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"VERSION":                 "v1.2.3",
			"RELEASE_FORGE":           "gitea",
			"RELEASE_TOKEN":           "token",
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
			"RELEASE_NOTES_MODE":      "news-md",
			"RELEASE_NOTES_SOURCE":    "NEWS.md",
			"RELEASE_BODY_MODE":       "none",
			"GORELEASER_BIN":          "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err != nil {
		t.Fatal(err)
	}
	got := commandStrings(fake.runCommands)
	if len(got) != 2 {
		t.Fatalf("commands = %#v, want 2 commands", got)
	}
	if got[0] != "/fake/helm package charts/demo --version 1.2.3 --app-version 1.2.3 --destination dist/charts" {
		t.Fatalf("first command = %q, want helm package", got[0])
	}
	if !strings.HasPrefix(got[1], "/tools/goreleaser --config .goreleaser.yaml release --clean --release-notes ") {
		t.Fatalf("second command = %q, want goreleaser publish", got[1])
	}
}

func TestPublishPushesHelmChartsToOCIAfterGoreleaser(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"VERSION":                     "v1.2.3",
			"RELEASE_FORGE":               "gitea",
			"RELEASE_TOKEN":               "token",
			"RELEASE_ARTIFACTS":           "charts",
			"RELEASE_HELM_CHART_DIRS":     "charts/demo",
			"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts",
			"RELEASE_NOTES_MODE":          "news-md",
			"RELEASE_NOTES_SOURCE":        "NEWS.md",
			"RELEASE_BODY_MODE":           "none",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err != nil {
		t.Fatal(err)
	}
	got := commandStrings(fake.runCommands)
	if len(got) != 3 {
		t.Fatalf("commands = %#v, want 3 commands", got)
	}
	if got[0] != "/fake/helm package charts/demo --version 1.2.3 --app-version 1.2.3 --destination dist/charts" {
		t.Fatalf("first command = %q, want helm package", got[0])
	}
	if !strings.HasPrefix(got[1], "/tools/goreleaser --config .goreleaser.yaml release --clean --release-notes ") {
		t.Fatalf("second command = %q, want goreleaser publish", got[1])
	}
	if got[2] != "/fake/helm push dist/charts/demo-1.2.3.tgz oci://registry.example/charts" {
		t.Fatalf("third command = %q, want helm OCI push", got[2])
	}
}

func TestPublishPatchesReleaseBodyBeforeFailingHelmOCIPush(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	patched := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/demo/releases/tags/v1.2.3":
			_, _ = w.Write([]byte(`{"id":42}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/owner/demo/releases/42":
			patched = true
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 0 && cmd.Args[0] == "push" {
			return errors.New("helm push failed")
		}
		return nil
	}
	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"VERSION":                     "v1.2.3",
			"RELEASE_FORGE":               "gitea",
			"RELEASE_API_URL":             server.URL,
			"RELEASE_OWNER":               "owner",
			"RELEASE_PROJECT":             "demo",
			"RELEASE_TOKEN":               "token",
			"RELEASE_ARTIFACTS":           "charts",
			"RELEASE_HELM_CHART_DIRS":     "charts/demo",
			"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts",
			"RELEASE_NOTES_MODE":          "news-md",
			"RELEASE_NOTES_SOURCE":        "NEWS.md",
			"RELEASE_BODY_MODE":           "patch",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err == nil {
		t.Fatal("expected helm push error")
	}
	if !patched {
		t.Fatal("release body was not patched before helm push failed")
	}
}

func TestPublishPushesDiscoveredHelmPackagePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	chartDir := filepath.Join(dir, "charts", "demo")
	if err := os.MkdirAll(chartDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(chartDir, "Chart.yaml"), "apiVersion: v2\nname: published-name # inline comment\nversion: 0.0.0\n")
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"VERSION":                     "v1.2.3",
			"RELEASE_FORGE":               "gitea",
			"RELEASE_TOKEN":               "token",
			"RELEASE_ARTIFACTS":           "charts",
			"RELEASE_HELM_CHART_DIRS":     "charts/demo",
			"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts",
			"RELEASE_NOTES_MODE":          "news-md",
			"RELEASE_NOTES_SOURCE":        "NEWS.md",
			"RELEASE_BODY_MODE":           "none",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err != nil {
		t.Fatal(err)
	}
	got := commandStrings(fake.runCommands)
	if got[len(got)-1] != "/fake/helm push dist/charts/published-name-1.2.3.tgz oci://registry.example/charts" {
		t.Fatalf("last command = %q, want discovered helm package path", got[len(got)-1])
	}
}

func TestPublishStopsBeforeGoreleaserWhenHelmPackageFails(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 0 && cmd.Args[0] == "package" {
			return errors.New("helm package failed")
		}
		return nil
	}
	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"VERSION":                 "v1.2.3",
			"RELEASE_FORGE":           "gitea",
			"RELEASE_TOKEN":           "token",
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
			"RELEASE_NOTES_MODE":      "news-md",
			"RELEASE_NOTES_SOURCE":    "NEWS.md",
			"RELEASE_BODY_MODE":       "none",
			"GORELEASER_BIN":          "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err == nil {
		t.Fatal("expected helm package error")
	}
	for _, cmd := range fake.runCommands {
		if cmd.Name == "/tools/goreleaser" {
			t.Fatalf("goreleaser ran after helm package failure: %#v", commandStrings(fake.runCommands))
		}
	}
}

func TestPublishTagPackagesHelmChartsFromClone(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	clone := filepath.Join(dir, ".tmp", "release-v1.2.3")
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "git" && len(cmd.Args) > 0 && cmd.Args[0] == "clone" {
			writeChart(t, filepath.Join(clone, "charts", "demo"), "demo")
			writeFile(t, filepath.Join(clone, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
			writeFile(t, filepath.Join(clone, ".goreleaser.yaml"), "version: 2\n")
		}
		return nil
	}
	a := &app{
		repoRoot: repo,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"RELEASE_FORGE":           "gitea",
			"RELEASE_TOKEN":           "token",
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
			"RELEASE_NOTES_MODE":      "news-md",
			"RELEASE_NOTES_SOURCE":    "NEWS.md",
			"RELEASE_BODY_MODE":       "none",
			"GORELEASER_BIN":          "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publishTag("v1.2.3"); err != nil {
		t.Fatal(err)
	}
	var helmPackage, goreleaserPublish *runner.Command
	helmIndex, goreleaserIndex := -1, -1
	for i := range fake.runCommands {
		cmd := &fake.runCommands[i]
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 0 && cmd.Args[0] == "package" {
			helmPackage = cmd
			helmIndex = i
		}
		if cmd.Name == "/tools/goreleaser" {
			goreleaserPublish = cmd
			goreleaserIndex = i
		}
	}
	if helmPackage == nil {
		t.Fatalf("commands = %#v, want helm package", commandStrings(fake.runCommands))
	}
	if helmPackage.Dir != clone {
		t.Fatalf("helm package dir = %q, want clone %q", helmPackage.Dir, clone)
	}
	if goreleaserPublish == nil {
		t.Fatalf("commands = %#v, want goreleaser publish", commandStrings(fake.runCommands))
	}
	if goreleaserPublish.Dir != clone {
		t.Fatalf("goreleaser dir = %q, want clone %q", goreleaserPublish.Dir, clone)
	}
	if helmIndex > goreleaserIndex {
		t.Fatalf("helm package command ran after goreleaser: %#v", commandStrings(fake.runCommands))
	}
}

func TestPublishTagPushesHelmChartsFromClone(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	clone := filepath.Join(dir, ".tmp", "release-v1.2.3")
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "git" && len(cmd.Args) > 0 && cmd.Args[0] == "clone" {
			writeChart(t, filepath.Join(clone, "charts", "demo"), "demo")
			writeFile(t, filepath.Join(clone, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
			writeFile(t, filepath.Join(clone, ".goreleaser.yaml"), "version: 2\n")
		}
		return nil
	}
	a := &app{
		repoRoot: repo,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"RELEASE_FORGE":               "gitea",
			"RELEASE_TOKEN":               "token",
			"RELEASE_ARTIFACTS":           "charts",
			"RELEASE_HELM_CHART_DIRS":     "charts/demo",
			"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts",
			"RELEASE_NOTES_MODE":          "news-md",
			"RELEASE_NOTES_SOURCE":        "NEWS.md",
			"RELEASE_BODY_MODE":           "none",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publishTag("v1.2.3"); err != nil {
		t.Fatal(err)
	}
	var goreleaserIndex, pushIndex = -1, -1
	for i := range fake.runCommands {
		cmd := fake.runCommands[i]
		if cmd.Name == "/tools/goreleaser" {
			goreleaserIndex = i
		}
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 0 && cmd.Args[0] == "push" {
			pushIndex = i
			if cmd.Dir != clone {
				t.Fatalf("helm push dir = %q, want clone %q", cmd.Dir, clone)
			}
			wantArgs := "push dist/charts/demo-1.2.3.tgz oci://registry.example/charts"
			if strings.Join(cmd.Args, " ") != wantArgs {
				t.Fatalf("helm push args = %#v, want %s", cmd.Args, wantArgs)
			}
		}
	}
	if goreleaserIndex == -1 || pushIndex == -1 {
		t.Fatalf("commands = %#v, want goreleaser and helm push", commandStrings(fake.runCommands))
	}
	if pushIndex < goreleaserIndex {
		t.Fatalf("helm push ran before goreleaser: %#v", commandStrings(fake.runCommands))
	}
}

func TestPublishTagPatchesReleaseBodyBeforeFailingHelmOCIPush(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	clone := filepath.Join(dir, ".tmp", "release-v1.2.3")
	patched := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/repos/owner/demo/releases/tags/v1.2.3":
			_, _ = w.Write([]byte(`{"id":42}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/repos/owner/demo/releases/42":
			patched = true
			_, _ = w.Write([]byte(`{}`))
		default:
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "git" && len(cmd.Args) > 0 && cmd.Args[0] == "clone" {
			writeChart(t, filepath.Join(clone, "charts", "demo"), "demo")
			writeFile(t, filepath.Join(clone, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
			writeFile(t, filepath.Join(clone, ".goreleaser.yaml"), "version: 2\n")
		}
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 0 && cmd.Args[0] == "push" {
			return errors.New("helm push failed")
		}
		return nil
	}
	a := &app{
		repoRoot: repo,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"RELEASE_FORGE":               "gitea",
			"RELEASE_API_URL":             server.URL,
			"RELEASE_OWNER":               "owner",
			"RELEASE_PROJECT":             "demo",
			"RELEASE_TOKEN":               "token",
			"RELEASE_ARTIFACTS":           "charts",
			"RELEASE_HELM_CHART_DIRS":     "charts/demo",
			"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts",
			"RELEASE_NOTES_MODE":          "news-md",
			"RELEASE_NOTES_SOURCE":        "NEWS.md",
			"RELEASE_BODY_MODE":           "patch",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publishTag("v1.2.3"); err == nil {
		t.Fatal("expected helm push error")
	}
	if !patched {
		t.Fatal("release body was not patched before helm push failed")
	}
}

func TestPublishTagStopsBeforeGoreleaserWhenHelmPackageFails(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	clone := filepath.Join(dir, ".tmp", "release-v1.2.3")
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "git" && len(cmd.Args) > 0 && cmd.Args[0] == "clone" {
			writeChart(t, filepath.Join(clone, "charts", "demo"), "demo")
			writeFile(t, filepath.Join(clone, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
			writeFile(t, filepath.Join(clone, ".goreleaser.yaml"), "version: 2\n")
		}
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 0 && cmd.Args[0] == "package" {
			return errors.New("helm package failed")
		}
		return nil
	}
	a := &app{
		repoRoot: repo,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"RELEASE_FORGE":           "gitea",
			"RELEASE_TOKEN":           "token",
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
			"RELEASE_NOTES_MODE":      "news-md",
			"RELEASE_NOTES_SOURCE":    "NEWS.md",
			"RELEASE_BODY_MODE":       "none",
			"GORELEASER_BIN":          "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publishTag("v1.2.3"); err == nil {
		t.Fatal("expected helm package error")
	}
	for _, cmd := range fake.runCommands {
		if cmd.Name == "/tools/goreleaser" {
			t.Fatalf("goreleaser ran after helm package failure: %#v", commandStrings(fake.runCommands))
		}
	}
}

func TestCheckRunsHelmForMultipleCharts(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "api"), "api")
	writeChart(t, filepath.Join(dir, "charts", "worker"), "worker")
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/api, charts/worker",
			"GORELEASER_BIN":          "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.check(); err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/tools/goreleaser --config .goreleaser.yaml check",
		"/fake/helm dependency update --skip-refresh charts/api",
		"/fake/helm lint charts/api",
		"/fake/helm dependency update --skip-refresh charts/worker",
		"/fake/helm lint charts/worker",
	}
	if got := commandStrings(fake.runCommands); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
}

func TestValidateChartConfigRequiresChartDirs(t *testing.T) {
	a := &app{env: map[string]string{"RELEASE_ARTIFACTS": "charts"}}
	if err := a.validateChartConfig(); err == nil {
		t.Fatal("expected missing chart dirs error")
	}
}

func TestValidateChartConfigRejectsChartDirsOutsideRepo(t *testing.T) {
	for _, dir := range []string{"/charts/demo", "../charts/demo"} {
		t.Run(dir, func(t *testing.T) {
			a := &app{env: map[string]string{
				"RELEASE_ARTIFACTS":       "charts",
				"RELEASE_HELM_CHART_DIRS": dir,
			}}
			if err := a.validateChartConfig(); err == nil {
				t.Fatal("expected chart dir path error")
			}
		})
	}
}

func TestValidateChartConfigRejectsSymlinkOutsideRepo(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside")
	writeChart(t, outside, "outside")
	if err := os.MkdirAll(filepath.Join(dir, "charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(dir, "charts", "linked")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/linked",
		},
	}
	if err := a.validateChartConfig(); err == nil {
		t.Fatal("expected symlink escape error")
	}
}

func TestValidateChartConfigAllowsSymlinkedRepoRoot(t *testing.T) {
	realRepo := filepath.Join(t.TempDir(), "repo")
	writeChart(t, filepath.Join(realRepo, "charts", "demo"), "demo")
	repoLink := filepath.Join(t.TempDir(), "repo-link")
	if err := os.Symlink(realRepo, repoLink); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	a := &app{
		repoRoot: repoLink,
		env: map[string]string{
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
		},
	}
	if err := a.validateChartConfig(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateChartConfigReadsChartYAML(t *testing.T) {
	dir := t.TempDir()
	chartDir := filepath.Join(dir, "charts", "demo")
	if err := os.MkdirAll(filepath.Join(chartDir, "Chart.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_ARTIFACTS":       "charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
		},
	}
	if err := a.validateChartConfig(); err == nil {
		t.Fatal("expected unreadable Chart.yaml error")
	}
}

func TestValidateChartConfigRejectsUnsupportedVersionSources(t *testing.T) {
	for key, value := range map[string]string{
		"RELEASE_HELM_VERSION_FROM":     "chart",
		"RELEASE_HELM_APP_VERSION_FROM": "chart",
	} {
		t.Run(key, func(t *testing.T) {
			a := &app{env: map[string]string{key: value}}
			if err := a.validateChartConfig(); err == nil {
				t.Fatal("expected unsupported version source error")
			}
		})
	}
}

func TestValidateChartConfigRejectsInvalidOCIRepository(t *testing.T) {
	for _, env := range []map[string]string{
		{"RELEASE_ARTIFACTS": "charts", "RELEASE_HELM_CHART_DIRS": "charts/demo", "RELEASE_HELM_OCI_REPOSITORY": "https://registry.example/charts"},
		{"RELEASE_ARTIFACTS": "charts", "RELEASE_HELM_CHART_DIRS": "charts/demo", "RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts with-space"},
		{"RELEASE_ARTIFACTS": "binaries", "RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts"},
	} {
		a := &app{env: env}
		if err := a.validateChartConfig(); err == nil {
			t.Fatal("expected OCI repository validation error")
		}
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

func TestDoctorUsesInjectedRunnerForGoreleaserVersion(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), "version: 2\n")
	fake := &fakeCommandRunner{combinedOutput: []byte("GitVersion: v2.16.0\n")}
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_PROJECT":    "demo",
			"RELEASE_OWNER":      "owner",
			"RELEASE_NOTES_MODE": "none",
			"RELEASE_BODY_MODE":  "none",
			"GORELEASER_BIN":     "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	if len(fake.combinedOutputCommands) != 1 {
		t.Fatalf("combined output commands = %d, want 1", len(fake.combinedOutputCommands))
	}
	cmd := fake.combinedOutputCommands[0]
	if cmd.Name != "/tools/goreleaser" {
		t.Fatalf("Name = %q, want /tools/goreleaser", cmd.Name)
	}
	if len(cmd.Args) != 1 || cmd.Args[0] != "--version" {
		t.Fatalf("Args = %#v, want [--version]", cmd.Args)
	}
}

func TestRunGoreleaserUsesInjectedRunner(t *testing.T) {
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: "/repo",
		env: map[string]string{
			"GORELEASER_BIN":    "/tools/goreleaser",
			"GORELEASER_CONFIG": "release.yml",
			"RELEASE_FORGE":     "github",
			"GITHUB_TOKEN":      "github-token",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.runGoreleaser("check"); err != nil {
		t.Fatal(err)
	}
	if len(fake.runCommands) != 1 {
		t.Fatalf("run commands = %d, want 1", len(fake.runCommands))
	}
	cmd := fake.runCommands[0]
	if cmd.Dir != "/repo" {
		t.Fatalf("Dir = %q, want /repo", cmd.Dir)
	}
	if cmd.Name != "/tools/goreleaser" {
		t.Fatalf("Name = %q, want /tools/goreleaser", cmd.Name)
	}
	wantArgs := []string{"--config", "release.yml", "check"}
	if strings.Join(cmd.Args, "\x00") != strings.Join(wantArgs, "\x00") {
		t.Fatalf("Args = %#v, want %#v", cmd.Args, wantArgs)
	}
	if !envContains(cmd.Env, "GITHUB_TOKEN=github-token") {
		t.Fatalf("Env does not contain mapped GitHub token")
	}
}

func TestResolveTagUsesInjectedRunner(t *testing.T) {
	fake := &fakeCommandRunner{output: []byte("v1.2.3\n")}
	a := &app{repoRoot: "/repo", env: map[string]string{}, commands: fake}
	tag, err := a.resolveTag("")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v1.2.3" {
		t.Fatalf("tag = %q, want v1.2.3", tag)
	}
	if len(fake.outputCommands) != 1 {
		t.Fatalf("output commands = %d, want 1", len(fake.outputCommands))
	}
	cmd := fake.outputCommands[0]
	if cmd.Name != "git" || strings.Join(cmd.Args, " ") != "-C /repo describe --tags --exact-match" {
		t.Fatalf("command = %s %#v", cmd.Name, cmd.Args)
	}
}

func TestVerifyTagExistsUsesInjectedRunner(t *testing.T) {
	fake := &fakeCommandRunner{}
	a := &app{repoRoot: "/repo", commands: fake}
	if err := a.verifyTagExists("v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if len(fake.runCommands) != 1 {
		t.Fatalf("run commands = %d, want 1", len(fake.runCommands))
	}
	cmd := fake.runCommands[0]
	want := "-C /repo rev-parse -q --verify refs/tags/v1.2.3"
	if cmd.Name != "git" || strings.Join(cmd.Args, " ") != want {
		t.Fatalf("command = %s %#v", cmd.Name, cmd.Args)
	}
}

func TestCloneTagUsesInjectedRunner(t *testing.T) {
	fake := &fakeCommandRunner{}
	a := &app{repoRoot: "/repo", commands: fake, stdout: ioDiscard(), stderr: ioDiscard()}
	if err := a.cloneTag("v1.2.3", "/clone"); err != nil {
		t.Fatal(err)
	}
	if len(fake.runCommands) != 2 {
		t.Fatalf("run commands = %d, want 2", len(fake.runCommands))
	}
	clone := fake.runCommands[0]
	wantClone := "clone --quiet file:///repo/.git /clone"
	if clone.Name != "git" || strings.Join(clone.Args, " ") != wantClone {
		t.Fatalf("clone command = %s %#v", clone.Name, clone.Args)
	}
	checkout := fake.runCommands[1]
	wantCheckout := "checkout --quiet --detach refs/tags/v1.2.3"
	if checkout.Dir != "/clone" || checkout.Name != "git" || strings.Join(checkout.Args, " ") != wantCheckout {
		t.Fatalf("checkout command = dir %q name %s args %#v", checkout.Dir, checkout.Name, checkout.Args)
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

func writeChart(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "Chart.yaml"), "apiVersion: v2\nname: "+name+"\nversion: 0.0.0\n")
}

func commandStrings(commands []runner.Command) []string {
	out := make([]string, 0, len(commands))
	for _, command := range commands {
		out = append(out, strings.TrimSpace(command.Name+" "+strings.Join(command.Args, " ")))
	}
	return out
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

type fakeCommandRunner struct {
	runCommands            []runner.Command
	outputCommands         []runner.Command
	combinedOutputCommands []runner.Command
	output                 []byte
	combinedOutput         []byte
	onRun                  func(runner.Command) error
}

func (f *fakeCommandRunner) Run(cmd runner.Command) error {
	f.runCommands = append(f.runCommands, cmd)
	if f.onRun != nil {
		if err := f.onRun(cmd); err != nil {
			return err
		}
	}
	if err := fakeHelmPackage(cmd); err != nil {
		return err
	}
	return nil
}

func fakeHelmPackage(cmd runner.Command) error {
	if cmd.Name != "/fake/helm" || len(cmd.Args) == 0 || cmd.Args[0] != "package" {
		return nil
	}
	chart := cmd.Args[1]
	version := ""
	destination := ""
	for i := 2; i < len(cmd.Args); i++ {
		switch cmd.Args[i] {
		case "--version":
			if i+1 < len(cmd.Args) {
				version = cmd.Args[i+1]
				i++
			}
		case "--destination":
			if i+1 < len(cmd.Args) {
				destination = cmd.Args[i+1]
				i++
			}
		}
	}
	if version == "" || destination == "" {
		return nil
	}
	name := fakeChartName(filepath.Join(cmd.Dir, chart, "Chart.yaml"))
	if name == "" {
		name = filepath.Base(chart)
	}
	if err := os.MkdirAll(filepath.Join(cmd.Dir, destination), 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(cmd.Dir, destination, name+"-"+version+".tgz"), []byte("chart"), 0o644)
}

func fakeChartName(chartFile string) string {
	content, err := os.ReadFile(chartFile)
	if err != nil {
		return ""
	}
	for _, raw := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		name, ok := strings.CutPrefix(line, "name:")
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if beforeComment, _, ok := strings.Cut(name, "#"); ok {
			name = beforeComment
		}
		return strings.Trim(strings.TrimSpace(name), "'\"")
	}
	return ""
}

func (f *fakeCommandRunner) Output(cmd runner.Command) ([]byte, error) {
	f.outputCommands = append(f.outputCommands, cmd)
	return f.output, nil
}

func (f *fakeCommandRunner) CombinedOutput(cmd runner.Command) ([]byte, error) {
	f.combinedOutputCommands = append(f.combinedOutputCommands, cmd)
	return f.combinedOutput, nil
}

func (f *fakeCommandRunner) LookPath(file string) (string, error) {
	return "/fake/" + file, nil
}

func (f *fakeCommandRunner) IsExecutable(string) bool {
	return true
}

func envContains(env []string, want string) bool {
	for _, entry := range env {
		if entry == want {
			return true
		}
	}
	return false
}

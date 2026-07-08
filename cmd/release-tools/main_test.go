package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
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

func TestLoadConfigFileRejectsOCIPlaintextPassword(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".release-tools.env"), []byte("RELEASE_HELM_OCI_PASSWORD=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := newApp([]string{"RELEASE_REPO_ROOT=" + dir}, ioDiscard(), ioDiscard())
	if err == nil {
		t.Fatal("expected unsupported key error")
	}
}

func TestLoadConfigFileRejectsClassicPlaintextToken(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".release-tools.env"), []byte("RELEASE_HELM_CLASSIC_TOKEN=secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := newApp([]string{"RELEASE_REPO_ROOT=" + dir}, ioDiscard(), ioDiscard())
	if err == nil {
		t.Fatal("expected unsupported key error")
	}
}

func TestUsageSupportedConfigKeysMatchAllowlist(t *testing.T) {
	content, err := os.ReadFile(filepath.Join("..", "..", "docs", "usage.md"))
	if err != nil {
		t.Fatal(err)
	}
	keys := markdownBulletCodeValues(string(content), "Supported `.release-tools.env` keys:", "Supported environment-only variables:")
	want := sortedAllowedConfigKeys()
	if strings.Join(keys, "\n") != strings.Join(want, "\n") {
		t.Fatalf("usage config keys = %#v, want %#v", keys, want)
	}
}

func TestExampleEnvFilesUseSupportedConfigKeys(t *testing.T) {
	for _, file := range []string{".release-tools.env", "chart-release.env"} {
		t.Run(file, func(t *testing.T) {
			content, err := os.ReadFile(filepath.Join("..", "..", "examples", file))
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range envExampleKeys(string(content)) {
				if !allowedConfigKeys[key] {
					t.Fatalf("%s uses unsupported config key %s", file, key)
				}
			}
		})
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
			"RELEASE_HELM_CLASSIC_URL":    "https://forge.example/api/packages/owner/helm",
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
	if !strings.Contains(stdout.String(), "[INFO] Helm classic URL: https://forge.example/api/packages/owner/helm\n") {
		t.Fatalf("doctor output = %q, want classic URL line", stdout.String())
	}
}

func TestDoctorReportsHelmProvenance(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), "version: 2\n")
	writeFile(t, filepath.Join(dir, "release-keyring.gpg"), "keyring")
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	var stdout bytes.Buffer
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_PROJECT":          "demo",
			"RELEASE_OWNER":            "owner",
			"RELEASE_ARTIFACTS":        "charts",
			"RELEASE_HELM_CHART_DIRS":  "charts/demo",
			"RELEASE_HELM_PROVENANCE":  "true",
			"RELEASE_HELM_GPG_KEY":     "maintainer@example.org",
			"RELEASE_HELM_GPG_KEYRING": "release-keyring.gpg",
			"RELEASE_NOTES_MODE":       "none",
			"RELEASE_BODY_MODE":        "none",
			"GORELEASER_BIN":           "/tools/goreleaser",
		},
		commands: &fakeCommandRunner{combinedOutput: []byte("GitVersion: v2.16.0\n")},
		stdout:   &stdout,
		stderr:   ioDiscard(),
	}
	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "[INFO] Helm provenance: true\n") {
		t.Fatalf("doctor output = %q, want provenance line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[INFO] Helm GPG key: maintainer@example.org\n") {
		t.Fatalf("doctor output = %q, want GPG key line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[INFO] Helm GPG keyring: "+filepath.Join(dir, "release-keyring.gpg")+"\n") {
		t.Fatalf("doctor output = %q, want GPG keyring line", stdout.String())
	}
}

func TestValidateHelmProvenanceConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "release-keyring.gpg"), "keyring")
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")

	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{
			name: "invalid bool",
			env:  map[string]string{"RELEASE_HELM_PROVENANCE": "maybe"},
			want: "RELEASE_HELM_PROVENANCE must be a boolean value",
		},
		{
			name: "requires charts",
			env:  map[string]string{"RELEASE_HELM_PROVENANCE": "true"},
			want: "RELEASE_HELM_PROVENANCE, RELEASE_HELM_GPG_KEY, and RELEASE_HELM_GPG_KEYRING require RELEASE_ARTIFACTS to include charts",
		},
		{
			name: "key requires provenance",
			env: map[string]string{
				"RELEASE_ARTIFACTS":        "charts",
				"RELEASE_HELM_CHART_DIRS":  "charts/demo",
				"RELEASE_HELM_GPG_KEY":     "maintainer@example.org",
				"RELEASE_HELM_GPG_KEYRING": "release-keyring.gpg",
			},
			want: "RELEASE_HELM_GPG_KEY and RELEASE_HELM_GPG_KEYRING require RELEASE_HELM_PROVENANCE=true",
		},
		{
			name: "missing key",
			env: map[string]string{
				"RELEASE_ARTIFACTS":        "charts",
				"RELEASE_HELM_CHART_DIRS":  "charts/demo",
				"RELEASE_HELM_PROVENANCE":  "true",
				"RELEASE_HELM_GPG_KEYRING": "release-keyring.gpg",
			},
			want: "RELEASE_HELM_GPG_KEY is required when RELEASE_HELM_PROVENANCE=true",
		},
		{
			name: "missing keyring",
			env: map[string]string{
				"RELEASE_ARTIFACTS":       "charts",
				"RELEASE_HELM_CHART_DIRS": "charts/demo",
				"RELEASE_HELM_PROVENANCE": "true",
				"RELEASE_HELM_GPG_KEY":    "maintainer@example.org",
			},
			want: "RELEASE_HELM_GPG_KEYRING is required when RELEASE_HELM_PROVENANCE=true",
		},
		{
			name: "unreadable keyring",
			env: map[string]string{
				"RELEASE_ARTIFACTS":        "charts",
				"RELEASE_HELM_CHART_DIRS":  "charts/demo",
				"RELEASE_HELM_PROVENANCE":  "true",
				"RELEASE_HELM_GPG_KEY":     "maintainer@example.org",
				"RELEASE_HELM_GPG_KEYRING": "missing.gpg",
			},
			want: "RELEASE_HELM_GPG_KEYRING is not readable:",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			a := &app{repoRoot: dir, env: tt.env}
			if err := a.validateChartConfig(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestDetectGoreleaserContainerConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), `version: 2

dockers:
  - image_templates:
      - example/app:latest
    use: buildx

dockers_v2:
  - images:
      - example/app

docker_manifests:
  - name_template: example/app:latest
    use: podman

docker_signs:
  - artifacts: manifests
    cmd: notation
`)
	a := &app{repoRoot: dir, env: map[string]string{}}

	config, err := a.detectGoreleaserContainerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(config.keys, ","); got != "dockers,dockers_v2,docker_manifests,docker_signs" {
		t.Fatalf("keys = %q, want all container keys", got)
	}
	if got := strings.Join(config.toolNames(), ","); got != "docker,notation,podman" {
		t.Fatalf("tools = %q, want docker,notation,podman", got)
	}
	if got := strings.Join(config.toolKeys("docker"), ","); got != "dockers,dockers_v2" {
		t.Fatalf("docker keys = %q, want dockers,dockers_v2", got)
	}
	if got := strings.Join(config.toolKeys("podman"), ","); got != "docker_manifests" {
		t.Fatalf("podman keys = %q, want docker_manifests", got)
	}
	if got := strings.Join(config.toolKeys("notation"), ","); got != "docker_signs" {
		t.Fatalf("notation keys = %q, want docker_signs", got)
	}
}

func TestDetectGoreleaserContainerConfigIgnoresDisabledBlocks(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), `version: 2
dockers: [] # disabled
docker_signs: null # disabled
`)
	a := &app{repoRoot: dir, env: map[string]string{}}

	config, err := a.detectGoreleaserContainerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if config.enabled() {
		t.Fatalf("container config = %#v, want disabled", config)
	}
}

func TestDetectGoreleaserContainerConfigSkipsDynamicSigningCommand(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), `version: 2
docker_signs:
  - artifacts: manifests
    cmd: "{{ .Env.SIGN_CMD }}"
`)
	a := &app{repoRoot: dir, env: map[string]string{}}

	config, err := a.detectGoreleaserContainerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(config.keys, ","); got != "docker_signs" {
		t.Fatalf("keys = %q, want docker_signs", got)
	}
	if got := strings.Join(config.toolNames(), ","); got != "" {
		t.Fatalf("tools = %q, want no static tool for dynamic signing command", got)
	}
}

func TestDetectGoreleaserContainerConfigHandlesSigningCommandForms(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), `version: 2
docker_signs:
  - artifacts: manifests
    cmd: "FOO=1 cosign sign --yes"
  - artifacts: images
    cmd: "sh -c 'notation sign --yes'"
  - artifacts: checksums
    cmd: |
      cosign sign --yes
`)
	a := &app{repoRoot: dir, env: map[string]string{}}

	config, err := a.detectGoreleaserContainerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(config.toolNames(), ","); got != "cosign,notation" {
		t.Fatalf("tools = %q, want cosign,notation", got)
	}
	if got := strings.Join(config.toolKeys("cosign"), ","); got != "docker_signs" {
		t.Fatalf("cosign keys = %q, want docker_signs", got)
	}
	if got := strings.Join(config.toolKeys("notation"), ","); got != "docker_signs" {
		t.Fatalf("notation keys = %q, want docker_signs", got)
	}
}

func TestDetectGoreleaserContainerConfigKeepsMixedEntryDefaults(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), `version: 2
dockers:
  - image_templates:
      - example/app:podman
    use: podman
  - image_templates:
      - example/app:docker
docker_signs:
  - artifacts: manifests
    cmd: notation
  - artifacts: images
`)
	a := &app{repoRoot: dir, env: map[string]string{}}

	config, err := a.detectGoreleaserContainerConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(config.toolNames(), ","); got != "cosign,docker,notation,podman" {
		t.Fatalf("tools = %q, want cosign,docker,notation,podman", got)
	}
	for _, tool := range []string{"cosign", "notation", "podman"} {
		if got := strings.Join(config.toolKeys(tool), ","); got != map[string]string{"cosign": "docker_signs", "notation": "docker_signs", "podman": "dockers"}[tool] {
			t.Fatalf("%s keys = %q", tool, got)
		}
	}
	if got := strings.Join(config.toolKeys("docker"), ","); got != "dockers" {
		t.Fatalf("docker keys = %q, want dockers", got)
	}
}

func TestDoctorReportsGoreleaserContainerConfig(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), `version: 2
dockers:
  - image_templates:
      - example/app:latest
docker_signs:
  - artifacts: manifests
`)
	var stdout bytes.Buffer
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_PROJECT":    "demo",
			"RELEASE_OWNER":      "owner",
			"RELEASE_NOTES_MODE": "none",
			"RELEASE_BODY_MODE":  "none",
			"GORELEASER_BIN":     "/tools/goreleaser",
		},
		commands: &fakeCommandRunner{combinedOutput: []byte("GitVersion: v2.16.0\n")},
		stdout:   &stdout,
		stderr:   ioDiscard(),
	}

	if err := a.doctor(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "[INFO] GoReleaser container config: dockers, docker_signs\n") {
		t.Fatalf("doctor output = %q, want container config line", stdout.String())
	}
	if !strings.Contains(stdout.String(), "[INFO] GoReleaser container tools: cosign, docker\n") {
		t.Fatalf("doctor output = %q, want container tools line", stdout.String())
	}
}

func TestEnsureToolsRequiresDetectedContainerTools(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), `version: 2
dockers:
  - image_templates:
      - example/app:latest
docker_signs:
  - artifacts: manifests
`)
	a := &app{
		repoRoot: dir,
		env:      map[string]string{"GORELEASER_BIN": "/tools/goreleaser"},
		commands: &fakeCommandRunner{lookPathErrors: map[string]error{"cosign": errors.New("missing")}},
	}

	err := a.ensureTools()
	if err == nil {
		t.Fatal("expected missing cosign error")
	}
	if !strings.Contains(err.Error(), "cosign is required because GoReleaser config uses docker_signs") {
		t.Fatalf("error = %q, want cosign requirement", err)
	}
}

func TestEnsureToolsChecksContainersWhenChartsAreEnabled(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, ".goreleaser.yaml"), `version: 2
docker_signs:
  - artifacts: manifests
`)
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_ARTIFACTS":       "binaries,charts",
			"RELEASE_HELM_CHART_DIRS": "charts/demo",
			"GORELEASER_BIN":          "/tools/goreleaser",
		},
		commands: &fakeCommandRunner{lookPathErrors: map[string]error{"cosign": errors.New("missing")}},
	}

	err := a.ensureTools()
	if err == nil {
		t.Fatal("expected missing cosign error")
	}
	if !strings.Contains(err.Error(), "cosign is required because GoReleaser config uses docker_signs") {
		t.Fatalf("error = %q, want cosign requirement", err)
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
	manifest := readReleaseManifest(t, filepath.Join(dir, "dist", "release-manifest.json"))
	if manifest.SchemaVersion != 1 {
		t.Fatalf("schema_version = %d, want 1", manifest.SchemaVersion)
	}
	if manifest.Release.Tag != "v1.2.3" || manifest.Release.Version != "1.2.3" {
		t.Fatalf("release = %#v, want v1.2.3 / 1.2.3", manifest.Release)
	}
	if len(manifest.Artifacts.HelmCharts) != 1 {
		t.Fatalf("helm charts = %d, want 1", len(manifest.Artifacts.HelmCharts))
	}
	chart := manifest.Artifacts.HelmCharts[0]
	if chart.Name != "demo" || chart.Version != "1.2.3" || chart.Path != "dist/charts/demo-1.2.3.tgz" {
		t.Fatalf("chart = %#v, want demo manifest entry", chart)
	}
	sha, err := fileSHA256(filepath.Join(dir, "dist", "charts", "demo-1.2.3.tgz"))
	if err != nil {
		t.Fatal(err)
	}
	if chart.SHA256 != sha {
		t.Fatalf("chart sha = %q, want %q", chart.SHA256, sha)
	}
}

func TestSnapshotSignsHelmChartsWithProvenance(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "release-keyring.gpg"), "keyring")
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	fake := &fakeCommandRunner{output: []byte("v1.2.3\n")}
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"RELEASE_ARTIFACTS":        "charts",
			"RELEASE_HELM_CHART_DIRS":  "charts/demo",
			"RELEASE_HELM_PROVENANCE":  "true",
			"RELEASE_HELM_GPG_KEY":     "maintainer@example.org",
			"RELEASE_HELM_GPG_KEYRING": "release-keyring.gpg",
			"GORELEASER_BIN":           "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.snapshot(); err != nil {
		t.Fatal(err)
	}
	want := "/fake/helm package charts/demo --version 1.2.3 --app-version 1.2.3 --destination dist/charts --sign --key maintainer@example.org --keyring " + filepath.Join(dir, "release-keyring.gpg")
	if got := commandStrings(fake.runCommands)[1]; got != want {
		t.Fatalf("helm package command = %q, want %q", got, want)
	}
	manifest := readReleaseManifest(t, filepath.Join(dir, "dist", "release-manifest.json"))
	chart := manifest.Artifacts.HelmCharts[0]
	if chart.ProvenancePath != "dist/charts/demo-1.2.3.tgz.prov" {
		t.Fatalf("provenance path = %q, want dist/charts/demo-1.2.3.tgz.prov", chart.ProvenancePath)
	}
	sha, err := fileSHA256(filepath.Join(dir, "dist", "charts", "demo-1.2.3.tgz.prov"))
	if err != nil {
		t.Fatal(err)
	}
	if chart.ProvenanceSHA256 != sha {
		t.Fatalf("provenance sha = %q, want %q", chart.ProvenanceSHA256, sha)
	}
}

func TestSnapshotWritesGoReleaserArtifactManifest(t *testing.T) {
	dir := t.TempDir()
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "/tools/goreleaser" {
			if err := os.MkdirAll(filepath.Join(dir, "dist"), 0o755); err != nil {
				return err
			}
			writeFile(t, filepath.Join(dir, "dist", "release-tools"), "binary")
			writeFile(t, filepath.Join(dir, "dist", "release-tools.tar.gz"), "archive")
			writeFile(t, filepath.Join(dir, "dist", "artifacts.json"), `[
{"name":"metadata.json","path":"dist/metadata.json","type":"Metadata"},
{"name":"release-tools","path":"dist/release-tools","type":"Binary","goos":"linux","goarch":"amd64","target":"linux_amd64_v1","extra":{"Checksum":"sha256:abc123"}},
{"name":"release-tools.tar.gz","path":"dist/release-tools.tar.gz","type":"Archive","target":"linux_amd64_v1","extra":{}}
]`)
		}
		return nil
	}
	a := &app{
		repoRoot: dir,
		env: map[string]string{
			"GORELEASER_BIN": "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.snapshot(); err != nil {
		t.Fatal(err)
	}
	manifest := readReleaseManifest(t, filepath.Join(dir, "dist", "release-manifest.json"))
	artifacts := manifest.Artifacts.GoReleaser
	if len(artifacts) != 2 {
		t.Fatalf("goreleaser artifacts = %d, want 2", len(artifacts))
	}
	if artifacts[0].Name != "release-tools.tar.gz" || artifacts[0].Type != "Archive" || artifacts[0].SHA256 == "" {
		t.Fatalf("archive artifact = %#v, want computed SHA-256", artifacts[0])
	}
	if artifacts[1].Name != "release-tools" || artifacts[1].Type != "Binary" || artifacts[1].SHA256 != "abc123" {
		t.Fatalf("binary artifact = %#v, want GoReleaser checksum", artifacts[1])
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
	assertPublishHelmPackageCommand(t, got[0], "charts/demo", "1.2.3", dir)
	if !strings.HasPrefix(got[1], "/tools/goreleaser --config .goreleaser.yaml release --clean --release-notes ") {
		t.Fatalf("second command = %q, want goreleaser publish", got[1])
	}
}

func TestPublishPersistsHelmProvenance(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "release-keyring.gpg"), "keyring")
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"VERSION":                  "v1.2.3",
			"RELEASE_FORGE":            "gitea",
			"RELEASE_TOKEN":            "token",
			"RELEASE_ARTIFACTS":        "charts",
			"RELEASE_HELM_CHART_DIRS":  "charts/demo",
			"RELEASE_HELM_PROVENANCE":  "true",
			"RELEASE_HELM_GPG_KEY":     "maintainer@example.org",
			"RELEASE_HELM_GPG_KEYRING": "release-keyring.gpg",
			"RELEASE_NOTES_MODE":       "news-md",
			"RELEASE_NOTES_SOURCE":     "NEWS.md",
			"RELEASE_BODY_MODE":        "none",
			"GORELEASER_BIN":           "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist", "charts", "demo-1.2.3.tgz.prov")); err != nil {
		t.Fatalf("persisted provenance missing: %v", err)
	}
	manifest := readReleaseManifest(t, filepath.Join(dir, "dist", "release-manifest.json"))
	chart := manifest.Artifacts.HelmCharts[0]
	if chart.ProvenancePath != "dist/charts/demo-1.2.3.tgz.prov" {
		t.Fatalf("manifest provenance path = %q, want stable provenance path", chart.ProvenancePath)
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
	assertPublishHelmPackageCommand(t, got[0], "charts/demo", "1.2.3", dir)
	if !strings.HasPrefix(got[1], "/tools/goreleaser --config .goreleaser.yaml release --clean --release-notes ") {
		t.Fatalf("second command = %q, want goreleaser publish", got[1])
	}
	if !isHelmPushCommand(got[2], "demo-1.2.3.tgz", "oci://registry.example/charts") {
		t.Fatalf("third command = %q, want helm OCI push", got[2])
	}
	manifest := readReleaseManifest(t, filepath.Join(dir, "dist", "release-manifest.json"))
	chart := manifest.Artifacts.HelmCharts[0]
	if chart.OCIRef != "oci://registry.example/charts/demo:1.2.3" {
		t.Fatalf("OCI ref = %q, want chart ref", chart.OCIRef)
	}
	if chart.Path != "dist/charts/demo-1.2.3.tgz" {
		t.Fatalf("chart path = %q, want stable chart path", chart.Path)
	}
	if _, err := os.Stat(filepath.Join(dir, "dist", "charts", "demo-1.2.3.tgz")); err != nil {
		t.Fatalf("persisted chart package missing: %v", err)
	}
}

func TestPublishSignsOCIHelmChartsByDigest(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 0 && cmd.Args[0] == "push" {
			_, _ = fmt.Fprintln(cmd.Stdout, "Pushed: registry.example/charts/demo:1.2.3")
			_, _ = fmt.Fprintln(cmd.Stdout, "Digest: sha256:abc123")
		}
		return nil
	}
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
			"RELEASE_HELM_OCI_SIGNER":     "cosign",
			"RELEASE_HELM_OCI_SIGN_ARGS":  "--key cosign.key",
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
	if len(got) != 4 {
		t.Fatalf("commands = %#v, want package, goreleaser, helm push, cosign", got)
	}
	if got[3] != "/fake/cosign sign --yes --key cosign.key registry.example/charts/demo@sha256:abc123" {
		t.Fatalf("sign command = %q, want digest cosign sign", got[3])
	}
	manifest := readReleaseManifest(t, filepath.Join(dir, "dist", "release-manifest.json"))
	chart := manifest.Artifacts.HelmCharts[0]
	if chart.OCIRef != "registry.example/charts/demo:1.2.3" {
		t.Fatalf("oci ref = %q, want pushed ref", chart.OCIRef)
	}
	if chart.OCIDigest != "sha256:abc123" || chart.OCIDigestRef != "registry.example/charts/demo@sha256:abc123" {
		t.Fatalf("digest fields = %q / %q, want immutable digest ref", chart.OCIDigest, chart.OCIDigestRef)
	}
	if chart.OCISigner != "cosign" || chart.OCISignedRef != "registry.example/charts/demo@sha256:abc123" {
		t.Fatalf("signature fields = %q / %q, want cosign digest ref", chart.OCISigner, chart.OCISignedRef)
	}
}

func TestPublishFailsSigningWhenHelmPushDigestMissing(t *testing.T) {
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
			"RELEASE_HELM_OCI_SIGNER":     "notation",
			"RELEASE_NOTES_MODE":          "news-md",
			"RELEASE_NOTES_SOURCE":        "NEWS.md",
			"RELEASE_BODY_MODE":           "none",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	err := a.publish()
	if err == nil || !strings.Contains(err.Error(), "cannot sign by immutable digest") {
		t.Fatalf("publish error = %v, want missing digest signing error", err)
	}
	for _, cmd := range fake.runCommands {
		if cmd.Name == "/fake/notation" {
			t.Fatalf("notation ran without digest: %#v", commandStrings(fake.runCommands))
		}
	}
}

func TestParseHelmPushOutput(t *testing.T) {
	ref, digest := parseHelmPushOutput("Pushed: localhost:5000/owner/charts/demo:1.2.3\nDigest: sha256:deadbeef\n")
	if ref != "localhost:5000/owner/charts/demo:1.2.3" || digest != "sha256:deadbeef" {
		t.Fatalf("parse = %q / %q, want pushed ref and digest", ref, digest)
	}
	if got := helmOCIDigestRef(ref, digest); got != "localhost:5000/owner/charts/demo@sha256:deadbeef" {
		t.Fatalf("digest ref = %q, want host port safe digest ref", got)
	}
}

func TestPublishLogsIntoHelmOCIWithTemporaryRegistryConfig(t *testing.T) {
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
			"RELEASE_HELM_OCI_USERNAME":   "robot",
			"RELEASE_HELM_OCI_PASSWORD":   "registry-token",
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
	if len(got) != 4 {
		t.Fatalf("commands = %#v, want 4 commands", got)
	}
	assertPublishHelmPackageCommand(t, got[0], "charts/demo", "1.2.3", dir)
	login := fake.runCommands[1]
	if login.Name != "/fake/helm" || strings.Join(login.Args[:7], " ") != "registry login registry.example --username robot --password-stdin --registry-config" {
		t.Fatalf("login command = %q, want helm registry login", got[1])
	}
	registryConfig := login.Args[7]
	if !strings.HasPrefix(registryConfig, filepath.Join(dir, ".tmp", "helm-registry-")) || filepath.Base(registryConfig) != "config.json" {
		t.Fatalf("registry config = %q, want unique temp config", registryConfig)
	}
	if !strings.HasPrefix(got[2], "/tools/goreleaser --config .goreleaser.yaml release --clean --release-notes ") {
		t.Fatalf("third command = %q, want goreleaser publish", got[2])
	}
	if !isHelmPushCommand(got[3], "demo-1.2.3.tgz", "oci://registry.example/charts") || !strings.HasSuffix(got[3], " --registry-config "+registryConfig) {
		t.Fatalf("fourth command = %q, want helm OCI push with registry config %q", got[3], registryConfig)
	}
	stdin, err := io.ReadAll(login.Stdin)
	if err != nil {
		t.Fatal(err)
	}
	if string(stdin) != "registry-token\n" {
		t.Fatalf("login stdin = %q, want registry token", stdin)
	}
	if _, err := os.Stat(filepath.Dir(registryConfig)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("temporary Helm registry config dir was not removed: %v", err)
	}
}

func TestPublishPushesHelmChartsWithPlainHTTP(t *testing.T) {
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
			"RELEASE_HELM_OCI_PLAIN_HTTP": "1",
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
	if !isHelmPushCommand(got[2], "demo-1.2.3.tgz", "oci://registry.example/charts") || !strings.HasSuffix(got[2], " --plain-http") {
		t.Fatalf("third command = %q, want helm OCI plain HTTP push", got[2])
	}
}

func TestPublishLogsIntoHelmOCIWithPlainHTTP(t *testing.T) {
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
			"RELEASE_HELM_OCI_USERNAME":   "robot",
			"RELEASE_HELM_OCI_PASSWORD":   "registry-token",
			"RELEASE_HELM_OCI_PLAIN_HTTP": "1",
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
	login := fake.runCommands[1]
	if login.Args[len(login.Args)-1] != "--plain-http" {
		t.Fatalf("login args = %#v, want --plain-http", login.Args)
	}
	push := fake.runCommands[3]
	if !strings.Contains(" "+strings.Join(push.Args, " ")+" ", " --plain-http ") {
		t.Fatalf("push args = %#v, want --plain-http", push.Args)
	}
}

func TestResolveHelmOCIAuthReadsPasswordFile(t *testing.T) {
	passwordFile := filepath.Join(t.TempDir(), "registry-password")
	writeFile(t, passwordFile, "file-token\n")
	a := &app{env: map[string]string{
		"RELEASE_HELM_OCI_REPOSITORY":    "oci://registry.example/charts",
		"RELEASE_HELM_OCI_USERNAME":      "robot",
		"RELEASE_HELM_OCI_PASSWORD_FILE": passwordFile,
	}}

	auth, err := a.resolveHelmOCIAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.username != "robot" || auth.password != "file-token" {
		t.Fatalf("auth = %#v, want username robot and file token", auth)
	}
}

func TestResolveHelmClassicAuthReadsTokenFile(t *testing.T) {
	tokenFile := filepath.Join(t.TempDir(), "classic-token")
	writeFile(t, tokenFile, "file-token\n")
	a := &app{env: map[string]string{
		"RELEASE_HELM_CLASSIC_URL":        "https://forge.example/api/packages/owner/helm",
		"RELEASE_HELM_CLASSIC_USERNAME":   "robot",
		"RELEASE_HELM_CLASSIC_TOKEN_FILE": tokenFile,
	}}

	auth, err := a.resolveHelmClassicAuth()
	if err != nil {
		t.Fatal(err)
	}
	if auth.username != "robot" || auth.token != "file-token" {
		t.Fatalf("auth = %#v, want username robot and file-token", auth)
	}
}

func TestPublishStopsBeforeGoreleaserWhenHelmOCILoginFails(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 1 && cmd.Args[0] == "registry" && cmd.Args[1] == "login" {
			return errors.New("helm login failed")
		}
		return nil
	}
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
			"RELEASE_HELM_OCI_USERNAME":   "robot",
			"RELEASE_HELM_OCI_PASSWORD":   "registry-token",
			"RELEASE_NOTES_MODE":          "news-md",
			"RELEASE_NOTES_SOURCE":        "NEWS.md",
			"RELEASE_BODY_MODE":           "none",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err == nil {
		t.Fatal("expected helm login error")
	}
	for _, cmd := range fake.runCommands {
		if cmd.Name == "/tools/goreleaser" {
			t.Fatalf("goreleaser ran after helm login failure: %#v", commandStrings(fake.runCommands))
		}
	}
}

func TestPublishUploadsHelmChartsToClassicRegistry(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	uploaded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/packages/owner/helm/api/charts" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "robot" || password != "classic-token" {
			t.Fatalf("BasicAuth = %q/%q/%v, want robot classic-token", username, password, ok)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "chart" {
			t.Fatalf("body = %q, want raw chart package", body)
		}
		uploaded = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"VERSION":                       "v1.2.3",
			"RELEASE_FORGE":                 "gitea",
			"RELEASE_TOKEN":                 "token",
			"RELEASE_ARTIFACTS":             "charts",
			"RELEASE_HELM_CHART_DIRS":       "charts/demo",
			"RELEASE_HELM_CLASSIC_URL":      server.URL + "/api/packages/owner/helm",
			"RELEASE_HELM_CLASSIC_USERNAME": "robot",
			"RELEASE_HELM_CLASSIC_TOKEN":    "classic-token",
			"RELEASE_NOTES_MODE":            "news-md",
			"RELEASE_NOTES_SOURCE":          "NEWS.md",
			"RELEASE_BODY_MODE":             "none",
			"GORELEASER_BIN":                "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err != nil {
		t.Fatal(err)
	}
	if !uploaded {
		t.Fatal("classic Helm chart was not uploaded")
	}
	got := commandStrings(fake.runCommands)
	if len(got) != 2 || !strings.HasPrefix(got[1], "/tools/goreleaser --config .goreleaser.yaml release --clean --release-notes ") {
		t.Fatalf("commands = %#v, want package then goreleaser", got)
	}
	manifest := readReleaseManifest(t, filepath.Join(dir, "dist", "release-manifest.json"))
	chart := manifest.Artifacts.HelmCharts[0]
	if chart.ClassicURL != server.URL+"/api/packages/owner/helm" {
		t.Fatalf("classic URL = %q, want registry URL", chart.ClassicURL)
	}
	if chart.ClassicUploadURL != server.URL+"/api/packages/owner/helm/api/charts" {
		t.Fatalf("classic upload URL = %q, want api/charts URL", chart.ClassicUploadURL)
	}
}

func TestPublishStopsBeforeGoreleaserWhenClassicTokenMissing(t *testing.T) {
	dir := t.TempDir()
	writeChart(t, filepath.Join(dir, "charts", "demo"), "demo")
	writeFile(t, filepath.Join(dir, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
	fake := &fakeCommandRunner{}
	a := &app{
		repoRoot: dir,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"VERSION":                       "v1.2.3",
			"RELEASE_FORGE":                 "gitea",
			"RELEASE_TOKEN":                 "token",
			"RELEASE_ARTIFACTS":             "charts",
			"RELEASE_HELM_CHART_DIRS":       "charts/demo",
			"RELEASE_HELM_CLASSIC_URL":      "https://forge.example/api/packages/owner/helm",
			"RELEASE_HELM_CLASSIC_USERNAME": "robot",
			"RELEASE_NOTES_MODE":            "news-md",
			"RELEASE_NOTES_SOURCE":          "NEWS.md",
			"RELEASE_BODY_MODE":             "none",
			"GORELEASER_BIN":                "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publish(); err == nil {
		t.Fatal("expected missing classic token error")
	}
	for _, cmd := range fake.runCommands {
		if cmd.Name == "/tools/goreleaser" {
			t.Fatalf("goreleaser ran after classic token failure: %#v", commandStrings(fake.runCommands))
		}
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
	if !isHelmPushCommand(got[len(got)-1], "published-name-1.2.3.tgz", "oci://registry.example/charts") {
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
			if len(cmd.Args) != 3 || cmd.Args[0] != "push" || filepath.Base(cmd.Args[1]) != "demo-1.2.3.tgz" || !filepath.IsAbs(cmd.Args[1]) || cmd.Args[2] != "oci://registry.example/charts" {
				t.Fatalf("helm push args = %#v, want absolute chart package and OCI repository", cmd.Args)
			}
		}
	}
	if goreleaserIndex == -1 || pushIndex == -1 {
		t.Fatalf("commands = %#v, want goreleaser and helm push", commandStrings(fake.runCommands))
	}
	if pushIndex < goreleaserIndex {
		t.Fatalf("helm push ran before goreleaser: %#v", commandStrings(fake.runCommands))
	}
	manifest := readReleaseManifest(t, filepath.Join(repo, "dist", "release-manifest.json"))
	if manifest.Release.Tag != "v1.2.3" || manifest.Release.Version != "1.2.3" {
		t.Fatalf("manifest release = %#v, want v1.2.3 / 1.2.3", manifest.Release)
	}
	chart := manifest.Artifacts.HelmCharts[0]
	if chart.OCIRef != "oci://registry.example/charts/demo:1.2.3" {
		t.Fatalf("manifest OCI ref = %q, want chart ref", chart.OCIRef)
	}
	if chart.Path != "dist/charts/demo-1.2.3.tgz" {
		t.Fatalf("manifest chart path = %q, want copied publish-tag package path", chart.Path)
	}
	if _, err := os.Stat(filepath.Join(repo, "dist", "charts", "demo-1.2.3.tgz")); err != nil {
		t.Fatalf("publish-tag chart package was not copied back: %v", err)
	}
}

func TestPublishTagCopiesBinaryOnlyManifestFromClone(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	clone := filepath.Join(dir, ".tmp", "release-v1.2.3")
	fake := &fakeCommandRunner{}
	fake.onRun = func(cmd runner.Command) error {
		if cmd.Name == "git" && len(cmd.Args) > 0 && cmd.Args[0] == "clone" {
			if err := os.MkdirAll(clone, 0o755); err != nil {
				return err
			}
			writeFile(t, filepath.Join(clone, "NEWS.md"), "# News\n\n## v1.2.3 - 2026-07-02\n\n- release\n")
			writeFile(t, filepath.Join(clone, ".goreleaser.yaml"), "version: 2\n")
		}
		if cmd.Name == "/tools/goreleaser" {
			if err := os.MkdirAll(filepath.Join(clone, "dist"), 0o755); err != nil {
				return err
			}
			writeFile(t, filepath.Join(clone, "dist", "release-tools"), "binary")
			writeFile(t, filepath.Join(clone, "dist", "artifacts.json"), `[
{"name":"release-tools","path":"dist/release-tools","type":"Binary","goos":"linux","goarch":"amd64","target":"linux_amd64_v1","extra":{"Checksum":"sha256:def456"}}
]`)
		}
		return nil
	}
	a := &app{
		repoRoot: repo,
		tmpDir:   filepath.Join(dir, ".tmp"),
		env: map[string]string{
			"RELEASE_FORGE":        "gitea",
			"RELEASE_TOKEN":        "token",
			"RELEASE_NOTES_MODE":   "news-md",
			"RELEASE_NOTES_SOURCE": "NEWS.md",
			"RELEASE_BODY_MODE":    "none",
			"GORELEASER_BIN":       "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publishTag("v1.2.3"); err != nil {
		t.Fatal(err)
	}
	manifest := readReleaseManifest(t, filepath.Join(repo, "dist", "release-manifest.json"))
	if manifest.Release.Tag != "v1.2.3" || manifest.Release.Version != "1.2.3" {
		t.Fatalf("manifest release = %#v, want v1.2.3 / 1.2.3", manifest.Release)
	}
	if len(manifest.Artifacts.GoReleaser) != 1 || manifest.Artifacts.GoReleaser[0].SHA256 != "def456" {
		t.Fatalf("goreleaser artifacts = %#v, want copied binary metadata", manifest.Artifacts.GoReleaser)
	}
}

func TestPublishTagPatchesReleaseBodyBeforeFailingHelmOCIPush(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	clone := filepath.Join(dir, ".tmp", "release-v1.2.3")
	if err := os.MkdirAll(filepath.Join(repo, "dist", "charts"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(repo, "dist", "release-manifest.json"), `{"stale":true}`)
	writeFile(t, filepath.Join(repo, "dist", "charts", "demo-1.2.3.tgz"), "stale")
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
	if _, err := os.Stat(filepath.Join(repo, "dist", "release-manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("release manifest exists after failed Helm push: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repo, "dist", "charts", "demo-1.2.3.tgz")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("chart package exists after failed Helm push: %v", err)
	}
}

func TestPublishTagStopsBeforeGoreleaserWhenHelmOCILoginFails(t *testing.T) {
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
		if cmd.Name == "/fake/helm" && len(cmd.Args) > 1 && cmd.Args[0] == "registry" && cmd.Args[1] == "login" {
			return errors.New("helm login failed")
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
			"RELEASE_HELM_OCI_USERNAME":   "robot",
			"RELEASE_HELM_OCI_PASSWORD":   "registry-token",
			"RELEASE_NOTES_MODE":          "news-md",
			"RELEASE_NOTES_SOURCE":        "NEWS.md",
			"RELEASE_BODY_MODE":           "none",
			"GORELEASER_BIN":              "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publishTag("v1.2.3"); err == nil {
		t.Fatal("expected helm login error")
	}
	for _, cmd := range fake.runCommands {
		if cmd.Name == "/tools/goreleaser" {
			t.Fatalf("goreleaser ran after helm login failure: %#v", commandStrings(fake.runCommands))
		}
	}
}

func TestPublishTagUploadsHelmChartsToClassicRegistryFromClone(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	clone := filepath.Join(dir, ".tmp", "release-v1.2.3")
	uploaded := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/packages/owner/helm/api/charts" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.String())
		}
		username, password, ok := r.BasicAuth()
		if !ok || username != "robot" || password != "classic-token" {
			t.Fatalf("BasicAuth = %q/%q/%v, want robot classic-token", username, password, ok)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "chart" {
			t.Fatalf("body = %q, want clone chart package", body)
		}
		uploaded = true
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
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
			"RELEASE_FORGE":                 "gitea",
			"RELEASE_TOKEN":                 "token",
			"RELEASE_ARTIFACTS":             "charts",
			"RELEASE_HELM_CHART_DIRS":       "charts/demo",
			"RELEASE_HELM_CLASSIC_URL":      server.URL + "/api/packages/owner/helm",
			"RELEASE_HELM_CLASSIC_USERNAME": "robot",
			"RELEASE_HELM_CLASSIC_TOKEN":    "classic-token",
			"RELEASE_NOTES_MODE":            "news-md",
			"RELEASE_NOTES_SOURCE":          "NEWS.md",
			"RELEASE_BODY_MODE":             "none",
			"GORELEASER_BIN":                "/tools/goreleaser",
		},
		commands: fake,
		stdout:   ioDiscard(),
		stderr:   ioDiscard(),
	}

	if err := a.publishTag("v1.2.3"); err != nil {
		t.Fatal(err)
	}
	if !uploaded {
		t.Fatal("classic Helm chart was not uploaded")
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
		{"RELEASE_HELM_OCI_PLAIN_HTTP": "1"},
		{"RELEASE_ARTIFACTS": "binaries", "RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts", "RELEASE_HELM_OCI_PLAIN_HTTP": "1"},
		{"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts", "RELEASE_HELM_OCI_PLAIN_HTTP": "maybe"},
	} {
		a := &app{env: env}
		if err := a.validateChartConfig(); err == nil {
			t.Fatal("expected OCI repository validation error")
		}
	}
}

func TestValidateChartConfigRejectsPartialOCIAuth(t *testing.T) {
	for _, env := range []map[string]string{
		{"RELEASE_HELM_OCI_USERNAME": "robot", "RELEASE_HELM_OCI_PASSWORD": "token"},
		{"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts", "RELEASE_HELM_OCI_PASSWORD": "token"},
		{"RELEASE_HELM_OCI_REPOSITORY": "oci://registry.example/charts", "RELEASE_HELM_OCI_USERNAME": "robot"},
	} {
		a := &app{env: env}
		if err := a.validateChartConfig(); err == nil {
			t.Fatal("expected OCI auth validation error")
		}
	}
}

func TestValidateChartConfigRejectsInvalidClassicConfig(t *testing.T) {
	for _, env := range []map[string]string{
		{"RELEASE_HELM_CLASSIC_TOKEN": "token"},
		{"RELEASE_HELM_CLASSIC_URL": "https://forge.example/api/packages/owner/helm", "RELEASE_HELM_CLASSIC_TOKEN": "token"},
		{"RELEASE_HELM_CLASSIC_URL": "oci://registry.example/charts"},
		{"RELEASE_HELM_CLASSIC_URL": "https://robot:secret@forge.example/api/packages/owner/helm"},
		{"RELEASE_HELM_CLASSIC_URL": "https://forge.example/api/packages/owner/helm?token=secret"},
		{"RELEASE_HELM_CLASSIC_URL": "https://forge.example/api/packages/owner/helm#secret"},
		{"RELEASE_HELM_CLASSIC_URL": "https://forge.example/api/packages/owner/helm/api/charts"},
		{"RELEASE_ARTIFACTS": "binaries", "RELEASE_HELM_CLASSIC_URL": "https://forge.example/api/packages/owner/helm"},
	} {
		a := &app{env: env}
		if err := a.validateChartConfig(); err == nil {
			t.Fatal("expected classic Helm validation error")
		}
	}
}

func TestHelmClassicUploadURLAppendsAPICharts(t *testing.T) {
	got, err := helmClassicUploadURL("https://forge.example/api/packages/owner/helm/")
	if err != nil {
		t.Fatal(err)
	}
	want := "https://forge.example/api/packages/owner/helm/api/charts"
	if got != want {
		t.Fatalf("upload URL = %q, want %q", got, want)
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
			"GORELEASER_BIN":             "/tools/goreleaser",
			"GORELEASER_CONFIG":          "release.yml",
			"RELEASE_FORGE":              "github",
			"GITHUB_TOKEN":               "github-token",
			"RELEASE_HELM_OCI_PASSWORD":  "oci-secret",
			"RELEASE_HELM_CLASSIC_TOKEN": "classic-secret",
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
	if envContains(cmd.Env, "RELEASE_HELM_OCI_PASSWORD=oci-secret") || envContains(cmd.Env, "RELEASE_HELM_CLASSIC_TOKEN=classic-secret") {
		t.Fatalf("GoReleaser environment contains Helm registry secret")
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

func assertPublishHelmPackageCommand(t *testing.T, got, chart, version, repoRoot string) {
	t.Helper()
	prefix := "/fake/helm package " + chart + " --version " + version + " --app-version " + version + " --destination "
	if !strings.HasPrefix(got, prefix) {
		t.Fatalf("helm package command = %q, want prefix %q", got, prefix)
	}
	destination := strings.TrimPrefix(got, prefix)
	if !filepath.IsAbs(destination) {
		t.Fatalf("helm package destination = %q, want absolute temp dir", destination)
	}
	inside, err := pathInside(repoRoot, destination)
	if err != nil {
		t.Fatal(err)
	}
	if inside {
		t.Fatalf("helm package destination = %q, want outside repo %q", destination, repoRoot)
	}
}

func isHelmPushCommand(got, chartPackage, repository string) bool {
	fields := strings.Fields(got)
	return len(fields) >= 4 && fields[0] == "/fake/helm" && fields[1] == "push" && filepath.Base(fields[2]) == chartPackage && filepath.IsAbs(fields[2]) && fields[3] == repository
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

func readReleaseManifest(t *testing.T, path string) releaseManifest {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest releaseManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		t.Fatal(err)
	}
	return manifest
}

type fakeCommandRunner struct {
	runCommands            []runner.Command
	outputCommands         []runner.Command
	combinedOutputCommands []runner.Command
	output                 []byte
	combinedOutput         []byte
	lookPathErrors         map[string]error
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
	signed := false
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
		case "--sign":
			signed = true
		}
	}
	if version == "" || destination == "" {
		return nil
	}
	name := fakeChartName(filepath.Join(cmd.Dir, chart, "Chart.yaml"))
	if name == "" {
		name = filepath.Base(chart)
	}
	destinationPath := destination
	if !filepath.IsAbs(destinationPath) {
		destinationPath = filepath.Join(cmd.Dir, destinationPath)
	}
	if err := os.MkdirAll(destinationPath, 0o755); err != nil {
		return err
	}
	packagePath := filepath.Join(destinationPath, name+"-"+version+".tgz")
	if err := os.WriteFile(packagePath, []byte("chart"), 0o644); err != nil {
		return err
	}
	if signed {
		return os.WriteFile(packagePath+".prov", []byte("provenance"), 0o644)
	}
	return nil
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
	if err := f.lookPathErrors[file]; err != nil {
		return "", err
	}
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

func sortedAllowedConfigKeys() []string {
	keys := make([]string, 0, len(allowedConfigKeys))
	for key := range allowedConfigKeys {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func markdownBulletCodeValues(content, start, end string) []string {
	inSection := false
	keys := []string{}
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == start {
			inSection = true
			continue
		}
		if inSection && line == end {
			break
		}
		if !inSection || !strings.HasPrefix(line, "- `") {
			continue
		}
		value, _, ok := strings.Cut(strings.TrimPrefix(line, "- `"), "`")
		if ok {
			keys = append(keys, value)
		}
	}
	sort.Strings(keys)
	return keys
}

func envExampleKeys(content string) []string {
	seen := map[string]bool{}
	keys := []string{}
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		line = strings.TrimSpace(strings.TrimPrefix(line, "#"))
		if line == "" || !strings.Contains(line, "=") {
			continue
		}
		key, _, _ := strings.Cut(line, "=")
		key = strings.TrimSpace(key)
		if key == "" || seen[key] {
			continue
		}
		keys = append(keys, key)
		seen[key] = true
	}
	sort.Strings(keys)
	return keys
}

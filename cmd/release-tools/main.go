package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"codeberg.org/rch/release-tools/internal/config"
	"codeberg.org/rch/release-tools/internal/runner"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

var allowedConfigKeys = map[string]bool{
	"RELEASE_FORGE":                   true,
	"RELEASE_PROJECT":                 true,
	"RELEASE_OWNER":                   true,
	"RELEASE_REPO":                    true,
	"RELEASE_API_URL":                 true,
	"RELEASE_ARTIFACTS":               true,
	"RELEASE_HELM_CHART_DIRS":         true,
	"RELEASE_HELM_VERSION_FROM":       true,
	"RELEASE_HELM_APP_VERSION_FROM":   true,
	"RELEASE_HELM_OCI_REPOSITORY":     true,
	"RELEASE_HELM_OCI_USERNAME":       true,
	"RELEASE_HELM_OCI_PASSWORD_FILE":  true,
	"RELEASE_HELM_OCI_PLAIN_HTTP":     true,
	"RELEASE_HELM_OCI_SIGNER":         true,
	"RELEASE_HELM_OCI_SIGN_ARGS":      true,
	"RELEASE_HELM_CLASSIC_URL":        true,
	"RELEASE_HELM_CLASSIC_USERNAME":   true,
	"RELEASE_HELM_CLASSIC_TOKEN_FILE": true,
	"RELEASE_HELM_PROVENANCE":         true,
	"RELEASE_HELM_GPG_KEY":            true,
	"RELEASE_HELM_GPG_KEYRING":        true,
	"RELEASE_NOTES_SOURCE":            true,
	"RELEASE_NOTES_MODE":              true,
	"RELEASE_BODY_MODE":               true,
	"RELEASE_MANIFEST_UPLOAD":         true,
	"GORELEASER_CONFIG":               true,
	"GORELEASER_BIN":                  true,
	"RELEASE_REQUIRE_GO":              true,
	"RELEASE_TOKEN_FILE":              true,
}

type forgeKind string

type appFactory func() (*app, error)

var releaseToolsVersion = "dev"

const (
	forgeGitea  forgeKind = "gitea"
	forgeGitHub forgeKind = "github"
	forgeGitLab forgeKind = "gitlab"

	artifactBinaries = "binaries"
	artifactCharts   = "charts"

	helmVersionFromTag = "tag"
)

type app struct {
	toolkitRoot string
	repoRoot    string
	tmpDir      string
	configFile  string
	env         map[string]string
	commands    runner.Runner
	stdout      io.Writer
	stderr      io.Writer
}

type fileState struct {
	size    int64
	modTime time.Time
}

type helmOCIAuth struct {
	username string
	password string
}

type helmOCIAuthSession struct {
	registryConfig string
	cleanup        func()
}

type helmClassicAuth struct {
	username string
	token    string
}

type releaseManifest struct {
	SchemaVersion int                      `json:"schema_version"`
	Release       releaseManifestRelease   `json:"release"`
	Artifacts     releaseManifestArtifacts `json:"artifacts"`
}

type releaseManifestRelease struct {
	Tag     string `json:"tag"`
	Version string `json:"version"`
}

type releaseManifestArtifacts struct {
	GoReleaser []releaseManifestGoReleaserArtifact `json:"goreleaser,omitempty"`
	HelmCharts []releaseManifestHelmChart          `json:"helm_charts"`
}

type releaseManifestGoReleaserArtifact struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Path   string `json:"path"`
	Target string `json:"target,omitempty"`
	GOOS   string `json:"goos,omitempty"`
	GOARCH string `json:"goarch,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
}

type releaseManifestHelmChart struct {
	Name             string `json:"name"`
	Version          string `json:"version"`
	Path             string `json:"path"`
	SHA256           string `json:"sha256"`
	ProvenancePath   string `json:"provenance_path,omitempty"`
	ProvenanceSHA256 string `json:"provenance_sha256,omitempty"`
	OCIRef           string `json:"oci_ref,omitempty"`
	OCIDigest        string `json:"oci_digest,omitempty"`
	OCIDigestRef     string `json:"oci_digest_ref,omitempty"`
	OCISigner        string `json:"oci_signer,omitempty"`
	OCISignedRef     string `json:"oci_signed_ref,omitempty"`
	ClassicURL       string `json:"classic_url,omitempty"`
	ClassicUploadURL string `json:"classic_upload_url,omitempty"`
}

type helmOCIPushResult struct {
	Package   string
	Ref       string
	Digest    string
	DigestRef string
	Signer    string
	SignedRef string
}

type goreleaserContainerConfig struct {
	keys  []string
	tools map[string]map[string]bool
}

func main() {
	if err := executeCLI(context.Background(), os.Environ(), os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		exitCode := 1
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		}
		os.Exit(exitCode)
	}
}

func executeCLI(ctx context.Context, environ []string, stdout, stderr io.Writer, args []string) error {
	cmd := newRootCommand(func() (*app, error) {
		return newApp(environ, stdout, stderr)
	}, stdout, stderr)
	cmd.SetArgs(args)
	return fang.Execute(
		ctx,
		cmd,
		fang.WithVersion(releaseToolsVersion),
		fang.WithErrorHandler(errorHandler),
		fang.WithColorSchemeFunc(fang.AnsiColorScheme),
	)
}

func errorHandler(w io.Writer, _ fang.Styles, err error) {
	fmt.Fprintf(w, "[ERROR] %s\n", err)
}

func newApp(environ []string, stdout, stderr io.Writer) (*app, error) {
	env := config.EnvironMap(environ)
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	repoRoot := config.Value(env, "RELEASE_REPO_ROOT", pwd)
	tmpDir := config.Value(env, "RELEASE_TMP_DIR", filepath.Join(repoRoot, ".tmp"))
	configFile := config.Value(env, "RELEASE_CONFIG_FILE", filepath.Join(repoRoot, ".release-tools.env"))
	toolkitRoot := resolveToolkitRoot()

	a := &app{
		toolkitRoot: toolkitRoot,
		repoRoot:    repoRoot,
		tmpDir:      tmpDir,
		configFile:  configFile,
		env:         env,
		commands:    runner.OSRunner{},
		stdout:      stdout,
		stderr:      stderr,
	}
	if err := a.loadConfigFile(); err != nil {
		return nil, err
	}
	return a, nil
}

func (a *app) cloneForRepo(repoRoot, tmpDir string) (*app, error) {
	env := make(map[string]string, len(a.env)+2)
	for k, v := range a.env {
		env[k] = v
	}
	env["RELEASE_REPO_ROOT"] = repoRoot
	env["RELEASE_TMP_DIR"] = tmpDir
	if _, ok := env["RELEASE_CONFIG_FILE"]; !ok {
		env["RELEASE_CONFIG_FILE"] = filepath.Join(repoRoot, ".release-tools.env")
	}

	cloned := &app{
		toolkitRoot: a.toolkitRoot,
		repoRoot:    repoRoot,
		tmpDir:      tmpDir,
		configFile:  env["RELEASE_CONFIG_FILE"],
		env:         env,
		commands:    a.commandRunner(),
		stdout:      a.stdout,
		stderr:      a.stderr,
	}
	if err := cloned.loadConfigFile(); err != nil {
		return nil, err
	}
	return cloned, nil
}

func resolveToolkitRoot() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	pwd, err := os.Getwd()
	if err == nil {
		return pwd
	}
	return "."
}

func environMap(environ []string) map[string]string {
	return config.EnvironMap(environ)
}

func envValue(env map[string]string, key, fallback string) string {
	return config.Value(env, key, fallback)
}

func (a *app) loadConfigFile() error {
	return config.LoadFile(a.configFile, a.env, allowedConfigKeys)
}

func (a *app) run(args []string) error {
	cmd := newRootCommand(func() (*app, error) { return a, nil }, a.stdout, a.stderr)
	cmd.SetArgs(args)
	return cmd.Execute()
}

func (a *app) commandRunner() runner.Runner {
	if a.commands != nil {
		return a.commands
	}
	return runner.OSRunner{}
}

func newRootCommand(factory appFactory, stdout, stderr io.Writer) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "release-tools",
		Short: "Standardized GoReleaser workflow helper",
		Long: `release-tools is a thin, opinionated layer that standardizes release
workflows for repositories that use GoReleaser as the build and publishing
engine.`,
		Example: `  release-tools version
  release-tools doctor
  release-tools check
  release-tools snapshot
  release-tools publish-tag v1.2.3`,
		Version:           releaseToolsVersion,
		SilenceUsage:      true,
		SilenceErrors:     true,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := requireNoArgs("release-tools", args); err != nil {
				return err
			}
			return cmd.Help()
		},
	}
	cmd.SetOut(stdout)
	cmd.SetErr(stderr)
	cmd.SetVersionTemplate("release-tools {{.Version}}\n")

	cmd.AddCommand(
		newVersionCommand(stdout),
		newToolsCheckCommand(factory),
		newDoctorCommand(factory),
		newCheckCommand(factory),
		newSnapshotCommand(factory),
		newPublishCommand(factory),
		newPublishTagCommand(factory),
		newNotesCommand(factory, stdout),
	)
	return cmd
}

func newVersionCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:               "version",
		Short:             "Show release-tools version",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			fmt.Fprintf(stdout, "release-tools %s\n", releaseToolsVersion)
			return nil
		},
	}
}

func newToolsCheckCommand(factory appFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "tools-check",
		Short:             "Check required local tools",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			a, err := factory()
			if err != nil {
				return err
			}
			return a.ensureTools()
		},
	}
}

func newDoctorCommand(factory appFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "doctor",
		Short:             "Check release-tools configuration",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			a, err := factory()
			if err != nil {
				return err
			}
			return a.doctor()
		},
	}
}

func newCheckCommand(factory appFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "check",
		Short:             "Validate GoReleaser configuration",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			a, err := factory()
			if err != nil {
				return err
			}
			if err := a.requireReleaseVars(); err != nil {
				return err
			}
			return a.check()
		},
	}
}

func newSnapshotCommand(factory appFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "snapshot",
		Short:             "Build a local snapshot release without publishing",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			a, err := factory()
			if err != nil {
				return err
			}
			if err := a.requireReleaseVars(); err != nil {
				return err
			}
			return a.snapshot()
		},
	}
}

func newPublishCommand(factory appFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "publish",
		Short:             "Publish the current tag",
		Args:              cobra.NoArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(*cobra.Command, []string) error {
			a, err := factory()
			if err != nil {
				return err
			}
			if err := a.requireReleaseVars(); err != nil {
				return err
			}
			return a.publish()
		},
	}
}

func newPublishTagCommand(factory appFactory) *cobra.Command {
	return &cobra.Command{
		Use:               "publish-tag [VERSION]",
		Short:             "Publish a specific existing tag from a clean clone",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(_ *cobra.Command, args []string) error {
			a, err := factory()
			if err != nil {
				return err
			}
			if err := a.requireReleaseVars(); err != nil {
				return err
			}
			version, err := a.requireVersionArgOrEnv("publish-tag", args)
			if err != nil {
				return err
			}
			return a.publishTag(version)
		},
	}
}

func newNotesCommand(factory appFactory, stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:               "notes [VERSION]",
		Short:             "Generate release notes for VERSION or the current tag",
		Args:              cobra.ArbitraryArgs,
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(_ *cobra.Command, args []string) error {
			a, err := factory()
			if err != nil {
				return err
			}
			if err := a.requireReleaseVars(); err != nil {
				return err
			}
			version, err := a.optionalVersionArg("notes", args)
			if err != nil {
				return err
			}
			notesFile, err := a.generateNotes(version)
			if err != nil {
				return err
			}
			fmt.Fprintln(stdout, notesFile)
			return nil
		},
	}
}

func requireNoArgs(command string, args []string) error {
	if len(args) != 0 {
		return fmt.Errorf("%s does not accept arguments", command)
	}
	return nil
}

func (a *app) optionalVersionArg(command string, args []string) (string, error) {
	if len(args) > 1 {
		return "", fmt.Errorf("%s accepts at most one VERSION argument", command)
	}
	if len(args) == 1 {
		return args[0], nil
	}
	return a.env["VERSION"], nil
}

func (a *app) requireVersionArgOrEnv(command string, args []string) (string, error) {
	version, err := a.optionalVersionArg(command, args)
	if err != nil {
		return "", err
	}
	if version == "" {
		return "", fmt.Errorf("VERSION is required, e.g. %s v1.2.3", command)
	}
	return version, nil
}

func (a *app) requireReleaseVars() error {
	if a.env["RELEASE_PROJECT"] == "" {
		return errors.New("RELEASE_PROJECT is required")
	}
	if a.env["RELEASE_OWNER"] == "" {
		return errors.New("RELEASE_OWNER is required")
	}
	return nil
}

func (a *app) check() error {
	if err := a.validateChartConfig(); err != nil {
		return err
	}
	if err := a.validateManifestUploadConfig(); err != nil {
		return err
	}
	if err := a.runGoreleaser("check"); err != nil {
		return err
	}
	return a.runHelmChecks()
}

func (a *app) snapshot() error {
	if err := a.validateChartConfig(); err != nil {
		return err
	}
	if err := a.validateManifestUploadConfig(); err != nil {
		return err
	}
	chartVersion := ""
	chartTag := ""
	if enabled, err := a.chartsEnabled(); err != nil {
		return err
	} else if enabled {
		tag, err := a.resolveTag("")
		if err != nil {
			return err
		}
		chartTag = tag
		chartVersion = chartVersionFromTag(tag)
	}
	if err := a.runGoreleaser("release", "--snapshot", "--skip=publish", "--clean"); err != nil {
		return err
	}
	packages, err := a.runHelmPackages(chartVersion)
	if err != nil {
		return err
	}
	if len(packages) > 0 || a.goreleaserArtifactsExist() {
		return a.writeReleaseManifest(chartTag, chartVersion, packages, nil)
	}
	return nil
}

func (a *app) doctor() error {
	if err := a.requireReleaseVars(); err != nil {
		return err
	}
	config := a.goreleaserConfig()
	notesSource := a.releaseNotesSource()
	notesMode := a.releaseNotesMode()
	bodyMode := a.releaseBodyMode()
	artifacts, err := a.releaseArtifacts()
	if err != nil {
		return err
	}
	if err := a.validateChartConfig(); err != nil {
		return err
	}
	if _, err := a.releaseForge(); err != nil {
		return err
	}

	if _, err := os.Stat(filepath.Join(a.repoRoot, config)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("GoReleaser config not found: %s", filepath.Join(a.repoRoot, config))
		}
		return err
	}

	switch notesMode {
	case "news-md", "gnu-news":
		if _, err := os.Stat(filepath.Join(a.repoRoot, notesSource)); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("release notes source not found: %s", filepath.Join(a.repoRoot, notesSource))
			}
			return err
		}
	case "none":
	default:
		return fmt.Errorf("unsupported RELEASE_NOTES_MODE: %s", notesMode)
	}

	switch bodyMode {
	case "patch", "none":
	default:
		return fmt.Errorf("unsupported RELEASE_BODY_MODE: %s", bodyMode)
	}
	if err := a.validateManifestUploadConfig(); err != nil {
		return err
	}

	if err := a.ensureTools(); err != nil {
		return err
	}
	goreleaserBin, err := a.resolveGoreleaserBin()
	if err != nil {
		return err
	}
	containerConfig, err := a.detectGoreleaserContainerConfig()
	if err != nil {
		return err
	}

	a.log("Repository root: %s", a.repoRoot)
	a.log("Toolkit root: %s", a.toolkitRoot)
	a.log("release-tools version: %s", releaseToolsVersion)
	a.log("Project: %s", a.env["RELEASE_PROJECT"])
	a.log("Forge: %s", a.releaseForgeName())
	a.log("Owner: %s", a.env["RELEASE_OWNER"])
	a.log("Repo: %s", a.releaseRepo())
	a.log("Forge API URL: %s", a.releaseAPIURL())
	a.log("GoReleaser config: %s", config)
	a.log("GoReleaser binary: %s", goreleaserBin)
	a.log("GoReleaser version: not probed by doctor")
	if containerConfig.enabled() {
		a.log("GoReleaser container config: %s", strings.Join(containerConfig.keys, ", "))
		if tools := containerConfig.toolNames(); len(tools) > 0 {
			a.log("GoReleaser container tools: %s", strings.Join(tools, ", "))
		}
	}
	a.log("Artifacts: %s", strings.Join(artifacts, ", "))
	if enabled, err := a.chartsEnabled(); err != nil {
		return err
	} else if enabled {
		dirs, err := a.helmChartDirs()
		if err != nil {
			return err
		}
		a.log("Helm chart dirs: %s", strings.Join(dirs, ", "))
		a.log("Helm version source: %s", a.helmVersionFrom())
		a.log("Helm app version source: %s", a.helmAppVersionFrom())
		a.log("Helm provenance: %t", a.helmProvenance())
		if a.helmProvenance() {
			a.log("Helm GPG key: %s", a.helmGPGKey())
			a.log("Helm GPG keyring: %s", a.helmGPGKeyringPath())
		}
		if repository := a.helmOCIRepository(); repository != "" {
			a.log("Helm OCI repository: %s", repository)
			if signer := a.helmOCISigner(); signer != "none" {
				a.log("Helm OCI signer: %s", signer)
			}
		}
		if classicURL := a.helmClassicURL(); classicURL != "" {
			a.log("Helm classic URL: %s", classicURL)
		}
	}
	a.log("Release notes mode: %s", notesMode)
	a.log("Release body mode: %s", bodyMode)
	a.log("Release manifest upload: %t", a.manifestUpload())
	a.log("release-tools configuration looks valid")
	return nil
}

func (a *app) ensureTools() error {
	if a.env["RELEASE_REQUIRE_GO"] == "1" {
		if _, err := a.commandRunner().LookPath("go"); err != nil {
			return errors.New("go is required")
		}
	}
	if _, err := a.resolveGoreleaserBin(); err != nil {
		return err
	}
	if enabled, err := a.chartsEnabled(); err != nil {
		return err
	} else if enabled {
		if _, err := a.resolveHelmBin(); err != nil {
			return err
		}
		if signer := a.helmOCISigner(); signer != "none" {
			if _, err := a.commandRunner().LookPath(signer); err != nil {
				return fmt.Errorf("%s is required when RELEASE_HELM_OCI_SIGNER=%s", signer, signer)
			}
		}
	}
	return a.ensureContainerTools()
}

func (a *app) ensureContainerTools() error {
	containerConfig, err := a.detectGoreleaserContainerConfig()
	if err != nil {
		return err
	}
	for _, tool := range containerConfig.toolNames() {
		if _, err := a.commandRunner().LookPath(tool); err != nil {
			return fmt.Errorf("%s is required because GoReleaser config uses %s", tool, strings.Join(containerConfig.toolKeys(tool), ", "))
		}
	}
	return nil
}

func (a *app) resolveGoreleaserBin() (string, error) {
	r := a.commandRunner()
	if bin := a.env["GORELEASER_BIN"]; bin != "" {
		if r.IsExecutable(bin) {
			return bin, nil
		}
		return "", fmt.Errorf("GORELEASER_BIN is not executable: %s", bin)
	}
	if bin, err := r.LookPath("goreleaser"); err == nil {
		return bin, nil
	}

	home, _ := os.UserHomeDir()
	candidates := []string{}
	if home != "" {
		candidates = append(candidates,
			filepath.Join(home, ".local/bin/goreleaser"),
			filepath.Join(home, "go/bin/goreleaser"),
			filepath.Join(home, ".local/go/bin/goreleaser"),
		)
	}
	candidates = append(candidates, "/usr/local/bin/goreleaser", "/usr/bin/goreleaser")
	for _, candidate := range candidates {
		if r.IsExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("goreleaser not found. Install it and ensure it is available in PATH or GORELEASER_BIN")
}

func (a *app) resolveHelmBin() (string, error) {
	if bin, err := a.commandRunner().LookPath("helm"); err == nil {
		return bin, nil
	}
	return "", errors.New("helm not found. Install it and ensure it is available in PATH when RELEASE_ARTIFACTS includes charts")
}

func (a *app) runGoreleaser(args ...string) error {
	goreleaserBin, err := a.resolveGoreleaserBin()
	if err != nil {
		return err
	}
	cmdArgs := append([]string{"--config", a.goreleaserConfig()}, args...)
	cmd := runner.Command{Dir: a.repoRoot, Name: goreleaserBin, Args: cmdArgs, Env: a.goreleaserEnviron(), Stdout: a.stdout, Stderr: a.stderr}
	if token, ok := a.resolveOptionalToken(); ok {
		cmd.Env = append(cmd.Env, a.goreleaserTokenEnv()+"="+token)
	}
	return a.commandRunner().Run(cmd)
}

func (a *app) runHelmChecks() error {
	if enabled, err := a.chartsEnabled(); err != nil {
		return err
	} else if !enabled {
		return nil
	}
	helmBin, err := a.resolveHelmBin()
	if err != nil {
		return err
	}
	dirs, err := a.helmChartDirs()
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		if err := a.runHelm(helmBin, "dependency", "update", "--skip-refresh", dir); err != nil {
			return err
		}
		if err := a.runHelm(helmBin, "lint", dir); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) runHelmPackages(version string) ([]string, error) {
	return a.runHelmPackagesTo(version, filepath.Join("dist", "charts"))
}

func (a *app) runHelmPackagesTo(version, destination string) ([]string, error) {
	if enabled, err := a.chartsEnabled(); err != nil {
		return nil, err
	} else if !enabled {
		return nil, nil
	}
	if version == "" {
		resolved, err := a.helmVersion()
		if err != nil {
			return nil, err
		}
		version = resolved
	}
	appVersion := version
	helmBin, err := a.resolveHelmBin()
	if err != nil {
		return nil, err
	}
	dirs, err := a.helmChartDirs()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(a.repoPath(destination), 0o755); err != nil {
		return nil, err
	}
	packages := []string{}
	for _, dir := range dirs {
		before, err := a.helmPackageFiles(destination, version)
		if err != nil {
			return nil, err
		}
		if err := a.runHelm(helmBin, a.helmPackageArgs(dir, version, appVersion, destination)...); err != nil {
			return nil, err
		}
		after, err := a.helmPackageFiles(destination, version)
		if err != nil {
			return nil, err
		}
		chartPackage, err := changedHelmPackage(before, after)
		if err != nil {
			return nil, err
		}
		if a.helmProvenance() {
			if _, err := os.Stat(a.repoPath(helmProvenancePath(chartPackage))); err != nil {
				if errors.Is(err, os.ErrNotExist) {
					return nil, fmt.Errorf("Helm provenance file not found for package: %s", chartPackage)
				}
				return nil, err
			}
		}
		packages = append(packages, chartPackage)
	}
	return packages, nil
}

func (a *app) helmPackageArgs(chartDir, version, appVersion, destination string) []string {
	args := []string{"package", chartDir, "--version", version, "--app-version", appVersion, "--destination", destination}
	if a.helmProvenance() {
		args = append(args, "--sign", "--key", a.helmGPGKey(), "--keyring", a.helmGPGKeyringPath())
	}
	return args
}

func (a *app) helmPackageFiles(destination, version string) (map[string]fileState, error) {
	entries, err := os.ReadDir(a.repoPath(destination))
	if err != nil {
		return nil, err
	}
	packages := map[string]fileState{}
	suffix := "-" + version + ".tgz"
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), suffix) {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return nil, err
		}
		packages[filepath.Join(destination, entry.Name())] = fileState{size: info.Size(), modTime: info.ModTime()}
	}
	return packages, nil
}

func (a *app) helmReleasePackageDir(tag string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "release-tools-helm-charts-"+safePathName(tag)+"-")
	if err != nil {
		return "", func() {}, err
	}
	return dir, func() { _ = os.RemoveAll(dir) }, nil
}

func safePathName(value string) string {
	var b strings.Builder
	for _, r := range value {
		if r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '.' || r == '_' || r == '-' {
			b.WriteRune(r)
			continue
		}
		b.WriteByte('-')
	}
	if b.Len() == 0 {
		return "release"
	}
	return b.String()
}

func (a *app) repoPath(path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(a.repoRoot, path)
}

func changedHelmPackage(before, after map[string]fileState) (string, error) {
	changed := []string{}
	for path, state := range after {
		if previous, ok := before[path]; !ok || previous != state {
			changed = append(changed, path)
		}
	}
	sort.Strings(changed)
	if len(changed) != 1 {
		return "", fmt.Errorf("expected one packaged Helm chart, found %d", len(changed))
	}
	return changed[0], nil
}

func (a *app) writeReleaseManifest(tag, version string, packages []string, ociResults []helmOCIPushResult) error {
	charts, err := a.releaseManifestHelmCharts(version, packages, ociResults)
	if err != nil {
		return err
	}
	if charts == nil {
		charts = []releaseManifestHelmChart{}
	}
	goreleaserArtifacts, err := a.releaseManifestGoReleaserArtifacts()
	if err != nil {
		return err
	}
	manifest := releaseManifest{
		SchemaVersion: 1,
		Release: releaseManifestRelease{
			Tag:     tag,
			Version: version,
		},
		Artifacts: releaseManifestArtifacts{
			GoReleaser: goreleaserArtifacts,
			HelmCharts: charts,
		},
	}
	content, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	content = append(content, '\n')
	manifestPath := filepath.Join(a.repoRoot, "dist", "release-manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(manifestPath, content, 0o644)
}

func (a *app) releaseManifestHelmCharts(version string, packages []string, ociResults []helmOCIPushResult) ([]releaseManifestHelmChart, error) {
	ociByPackage := map[string]helmOCIPushResult{}
	for _, result := range ociResults {
		ociByPackage[manifestPathForRepo(a.repoRoot, result.Package)] = result
	}
	charts := make([]releaseManifestHelmChart, 0, len(packages))
	for _, chartPackage := range packages {
		name := helmPackageName(chartPackage, version)
		sha, err := fileSHA256(a.repoPath(chartPackage))
		if err != nil {
			return nil, err
		}
		chart := releaseManifestHelmChart{
			Name:    name,
			Version: version,
			Path:    manifestPathForRepo(a.repoRoot, chartPackage),
			SHA256:  sha,
		}
		provenancePath := helmProvenancePath(chartPackage)
		if _, err := os.Stat(a.repoPath(provenancePath)); err == nil {
			provenanceSHA, err := fileSHA256(a.repoPath(provenancePath))
			if err != nil {
				return nil, err
			}
			chart.ProvenancePath = manifestPathForRepo(a.repoRoot, provenancePath)
			chart.ProvenanceSHA256 = provenanceSHA
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		if repository := a.helmOCIRepository(); repository != "" && name != "" {
			chart.OCIRef = strings.TrimRight(repository, "/") + "/" + name + ":" + version
		}
		if result, ok := ociByPackage[manifestPathForRepo(a.repoRoot, chartPackage)]; ok {
			if result.Ref != "" {
				chart.OCIRef = result.Ref
			}
			chart.OCIDigest = result.Digest
			chart.OCIDigestRef = result.DigestRef
			chart.OCISigner = result.Signer
			chart.OCISignedRef = result.SignedRef
		}
		if classicURL := a.helmClassicURL(); classicURL != "" {
			chart.ClassicURL = classicURL
			uploadURL, err := helmClassicUploadURL(classicURL)
			if err != nil {
				return nil, err
			}
			chart.ClassicUploadURL = uploadURL
		}
		charts = append(charts, chart)
	}
	sort.Slice(charts, func(i, j int) bool {
		if charts[i].Name != charts[j].Name {
			return charts[i].Name < charts[j].Name
		}
		return charts[i].Path < charts[j].Path
	})
	return charts, nil
}

func (a *app) goreleaserArtifactsExist() bool {
	_, err := os.Stat(filepath.Join(a.repoRoot, "dist", "artifacts.json"))
	return err == nil
}

func (a *app) releaseManifestGoReleaserArtifacts() ([]releaseManifestGoReleaserArtifact, error) {
	artifactsPath := filepath.Join(a.repoRoot, "dist", "artifacts.json")
	content, err := os.ReadFile(artifactsPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	type goreleaserArtifact struct {
		Name   string         `json:"name"`
		Path   string         `json:"path"`
		Type   string         `json:"type"`
		GOOS   string         `json:"goos"`
		GOARCH string         `json:"goarch"`
		Target string         `json:"target"`
		Extra  map[string]any `json:"extra"`
	}
	raw := []goreleaserArtifact{}
	if err := json.Unmarshal(content, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse GoReleaser artifacts metadata: %w", err)
	}
	artifacts := []releaseManifestGoReleaserArtifact{}
	for _, artifact := range raw {
		if artifact.Type == "Metadata" || artifact.Name == "" || artifact.Path == "" {
			continue
		}
		entry := releaseManifestGoReleaserArtifact{
			Name:   artifact.Name,
			Type:   artifact.Type,
			Path:   manifestPathForRepo(a.repoRoot, artifact.Path),
			Target: artifact.Target,
			GOOS:   artifact.GOOS,
			GOARCH: artifact.GOARCH,
			SHA256: goreleaserArtifactSHA256(artifact.Extra),
		}
		if entry.SHA256 == "" {
			if sha, err := fileSHA256(a.repoPath(artifact.Path)); err == nil {
				entry.SHA256 = sha
			} else if !errors.Is(err, os.ErrNotExist) {
				return nil, err
			}
		}
		artifacts = append(artifacts, entry)
	}
	sort.Slice(artifacts, func(i, j int) bool {
		if artifacts[i].Type != artifacts[j].Type {
			return artifacts[i].Type < artifacts[j].Type
		}
		if artifacts[i].Name != artifacts[j].Name {
			return artifacts[i].Name < artifacts[j].Name
		}
		return artifacts[i].Path < artifacts[j].Path
	})
	return artifacts, nil
}

func goreleaserArtifactSHA256(extra map[string]any) string {
	if extra == nil {
		return ""
	}
	checksum, ok := extra["Checksum"].(string)
	if !ok {
		return ""
	}
	checksum = strings.TrimSpace(checksum)
	if value, ok := strings.CutPrefix(checksum, "sha256:"); ok {
		return value
	}
	return ""
}

func (a *app) persistHelmPackages(packages []string) ([]string, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	destination := filepath.Join(a.repoRoot, "dist", "charts")
	if err := os.MkdirAll(destination, 0o755); err != nil {
		return nil, err
	}
	persisted := make([]string, 0, len(packages))
	for _, chartPackage := range packages {
		name := filepath.Base(chartPackage)
		target := filepath.Join(destination, name)
		if err := copyFile(a.repoPath(chartPackage), target); err != nil {
			return nil, err
		}
		provenancePath := helmProvenancePath(chartPackage)
		if _, err := os.Stat(a.repoPath(provenancePath)); err == nil {
			if err := copyFile(a.repoPath(provenancePath), target+".prov"); err != nil {
				return nil, err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		persisted = append(persisted, filepath.Join("dist", "charts", name))
	}
	sort.Strings(persisted)
	return persisted, nil
}

func helmPackageName(chartPackage, version string) string {
	name := filepath.Base(chartPackage)
	return strings.TrimSuffix(name, "-"+version+".tgz")
}

func helmProvenancePath(chartPackage string) string {
	return chartPackage + ".prov"
}

func manifestPathForRepo(repoRoot, path string) string {
	if filepath.IsAbs(path) {
		if rel, err := filepath.Rel(repoRoot, path); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
			return filepath.ToSlash(rel)
		}
		return filepath.ToSlash(path)
	}
	return filepath.ToSlash(path)
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func copyDirectory(source, target string) error {
	entries, err := os.ReadDir(source)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(target, 0o755); err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if err := copyFile(filepath.Join(source, entry.Name()), filepath.Join(target, entry.Name())); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	out, err := os.Create(target)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

func (a *app) prepareHelmOCIAuth(auth *helmOCIAuth) (*helmOCIAuthSession, error) {
	session := &helmOCIAuthSession{cleanup: func() {}}
	if auth == nil {
		return session, nil
	}
	repository := a.helmOCIRepository()
	if repository == "" {
		return session, nil
	}
	helmBin, err := a.resolveHelmBin()
	if err != nil {
		return nil, err
	}
	configPath, cleanup, err := a.helmOCIRegistryConfigPath()
	if err != nil {
		return nil, err
	}
	if err := a.runHelmOCILogin(helmBin, repository, configPath, auth); err != nil {
		cleanup()
		return nil, err
	}
	session.registryConfig = configPath
	session.cleanup = cleanup
	return session, nil
}

func (a *app) runHelmOCIPushes(packages []string, session *helmOCIAuthSession) ([]helmOCIPushResult, error) {
	if len(packages) == 0 {
		return nil, nil
	}
	repository := a.helmOCIRepository()
	if repository == "" {
		return nil, nil
	}
	if _, err := a.helmOCIRepositoryChecked(); err != nil {
		return nil, err
	}
	helmBin, err := a.resolveHelmBin()
	if err != nil {
		return nil, err
	}
	results := make([]helmOCIPushResult, 0, len(packages))
	for _, chartPackage := range packages {
		args := []string{"push", chartPackage, repository}
		if a.helmOCIPlainHTTP() {
			args = append(args, "--plain-http")
		}
		if session != nil && session.registryConfig != "" {
			args = append(args, "--registry-config", session.registryConfig)
		}
		result, err := a.runHelmOCIPush(helmBin, chartPackage, args...)
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func (a *app) runHelmOCIPush(helmBin, chartPackage string, args ...string) (helmOCIPushResult, error) {
	var output bytes.Buffer
	stdout := a.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	stderr := a.stderr
	if stderr == nil {
		stderr = io.Discard
	}
	cmd := runner.Command{
		Dir:    a.repoRoot,
		Name:   helmBin,
		Args:   args,
		Stdout: io.MultiWriter(stdout, &output),
		Stderr: io.MultiWriter(stderr, &output),
	}
	if err := a.commandRunner().Run(cmd); err != nil {
		return helmOCIPushResult{}, err
	}
	ref, digest := parseHelmPushOutput(output.String())
	result := helmOCIPushResult{Package: chartPackage, Ref: ref, Digest: digest}
	if digest != "" {
		result.DigestRef = helmOCIDigestRef(ref, digest)
	}
	return result, nil
}

func parseHelmPushOutput(output string) (string, string) {
	ref := ""
	digest := ""
	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if value, ok := strings.CutPrefix(line, "Pushed:"); ok {
			ref = strings.TrimSpace(value)
			continue
		}
		if value, ok := strings.CutPrefix(line, "Digest:"); ok {
			digest = strings.TrimSpace(value)
		}
	}
	return ref, digest
}

func helmOCIDigestRef(ref, digest string) string {
	if ref == "" || digest == "" {
		return ""
	}
	ref = strings.TrimPrefix(ref, "oci://")
	lastSlash := strings.LastIndex(ref, "/")
	lastColon := strings.LastIndex(ref, ":")
	if lastColon > lastSlash {
		ref = ref[:lastColon]
	}
	return ref + "@" + digest
}

func rebaseHelmOCIPushResults(results []helmOCIPushResult, packages []string) []helmOCIPushResult {
	if len(results) == 0 || len(packages) == 0 {
		return results
	}
	rebased := make([]helmOCIPushResult, 0, len(results))
	for i, result := range results {
		if i < len(packages) {
			result.Package = packages[i]
		}
		rebased = append(rebased, result)
	}
	return rebased
}

func (a *app) runHelmOCISignatures(results []helmOCIPushResult) error {
	signer := a.helmOCISigner()
	if signer == "none" || len(results) == 0 {
		return nil
	}
	bin, err := a.commandRunner().LookPath(signer)
	if err != nil {
		return fmt.Errorf("%s is required when RELEASE_HELM_OCI_SIGNER=%s", signer, signer)
	}
	for i := range results {
		if results[i].DigestRef == "" {
			return fmt.Errorf("helm push did not report an OCI digest for %s; cannot sign by immutable digest", results[i].Package)
		}
		args := a.helmOCISignCommandArgs(results[i].DigestRef)
		cmd := runner.Command{Dir: a.repoRoot, Name: bin, Args: args, Stdout: a.stdout, Stderr: a.stderr}
		if err := a.commandRunner().Run(cmd); err != nil {
			return err
		}
		results[i].Signer = signer
		results[i].SignedRef = results[i].DigestRef
	}
	return nil
}

func (a *app) helmOCISignCommandArgs(digestRef string) []string {
	args := []string{"sign", "--yes"}
	args = append(args, strings.Fields(a.helmOCISignArgs())...)
	return append(args, digestRef)
}

func (a *app) runHelmOCILogin(helmBin, repository, registryConfig string, auth *helmOCIAuth) error {
	host, err := helmOCIRegistryHost(repository)
	if err != nil {
		return err
	}
	args := []string{"registry", "login", host, "--username", auth.username, "--password-stdin", "--registry-config", registryConfig}
	if a.helmOCIPlainHTTP() {
		args = append(args, "--plain-http")
	}
	return a.runHelmWithStdin(helmBin, strings.NewReader(auth.password+"\n"), args...)
}

func (a *app) helmOCIRegistryConfigPath() (string, func(), error) {
	if err := os.MkdirAll(a.tmpDir, 0o700); err != nil {
		return "", func() {}, err
	}
	dir, err := os.MkdirTemp(a.tmpDir, "helm-registry-")
	if err != nil {
		return "", func() {}, err
	}
	return filepath.Join(dir, "config.json"), func() { _ = os.RemoveAll(dir) }, nil
}

func (a *app) runHelmClassicUploads(packages []string, auth *helmClassicAuth) error {
	classicURL := a.helmClassicURL()
	if len(packages) == 0 || classicURL == "" {
		return nil
	}
	if auth == nil {
		return errors.New("RELEASE_HELM_CLASSIC_USERNAME with RELEASE_HELM_CLASSIC_TOKEN or RELEASE_HELM_CLASSIC_TOKEN_FILE is required when RELEASE_HELM_CLASSIC_URL is set")
	}
	uploadURL, err := helmClassicUploadURL(classicURL)
	if err != nil {
		return err
	}
	for _, chartPackage := range packages {
		if err := a.uploadHelmClassicPackage(uploadURL, chartPackage, auth); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) uploadHelmClassicPackage(uploadURL, chartPackage string, auth *helmClassicAuth) error {
	content, err := os.ReadFile(a.repoPath(chartPackage))
	if err != nil {
		return err
	}
	req, err := http.NewRequest(http.MethodPost, uploadURL, bytes.NewReader(content))
	if err != nil {
		return err
	}
	req.SetBasicAuth(auth.username, auth.token)
	req.Header.Set("Content-Type", "application/gzip")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to upload Helm chart %s: %s", chartPackage, resp.Status)
	}
	return nil
}

func (a *app) runHelm(helmBin string, args ...string) error {
	return a.runHelmWithStdin(helmBin, nil, args...)
}

func (a *app) runHelmWithStdin(helmBin string, stdin io.Reader, args ...string) error {
	cmd := runner.Command{Dir: a.repoRoot, Name: helmBin, Args: args, Stdin: stdin, Stdout: a.stdout, Stderr: a.stderr}
	return a.commandRunner().Run(cmd)
}

func (a *app) publish() error {
	tag, err := a.resolveTag("")
	if err != nil {
		return err
	}
	token, err := a.resolveToken()
	if err != nil {
		return err
	}
	if err := a.validateChartConfig(); err != nil {
		return err
	}
	if err := a.validateManifestUploadConfig(); err != nil {
		return err
	}
	if err := a.clearReleaseManifestOutputs(); err != nil {
		return err
	}
	helmOCIAuth, err := a.resolveHelmOCIAuth()
	if err != nil {
		return err
	}
	helmClassicAuth, err := a.resolveHelmClassicAuth()
	if err != nil {
		return err
	}
	notesFile, err := a.generateNotes(tag)
	if err != nil {
		return err
	}
	chartDestination, cleanupCharts, err := a.helmReleasePackageDir(tag)
	if err != nil {
		return err
	}
	defer cleanupCharts()
	packages, err := a.runHelmPackagesTo(chartVersionFromTag(tag), chartDestination)
	if err != nil {
		return err
	}
	helmOCISession, err := a.prepareHelmOCIAuth(helmOCIAuth)
	if err != nil {
		return err
	}
	defer helmOCISession.cleanup()
	if err := a.runGoreleaserWithToken(token, "release", "--clean", "--release-notes", notesFile); err != nil {
		return err
	}
	if err := a.updateReleaseBody(tag, notesFile, token); err != nil {
		return err
	}
	ociResults, err := a.runHelmOCIPushes(packages, helmOCISession)
	if err != nil {
		return err
	}
	if err := a.runHelmOCISignatures(ociResults); err != nil {
		return err
	}
	if err := a.runHelmClassicUploads(packages, helmClassicAuth); err != nil {
		return err
	}
	manifestPackages := []string(nil)
	if len(packages) > 0 {
		manifestPackages, err = a.persistHelmPackages(packages)
		if err != nil {
			return err
		}
	}
	if err := a.writeReleaseManifest(tag, chartVersionFromTag(tag), manifestPackages, rebaseHelmOCIPushResults(ociResults, manifestPackages)); err != nil {
		return err
	}
	return a.uploadReleaseManifestIfEnabled(tag, token)
}

func (a *app) publishTag(tag string) error {
	token, err := a.resolveToken()
	if err != nil {
		return err
	}
	if err := a.verifyTagExists(tag); err != nil {
		return err
	}
	if err := a.validateManifestUploadConfig(); err != nil {
		return err
	}
	if err := a.clearReleaseManifestOutputs(); err != nil {
		return err
	}

	cloneDir := filepath.Join(a.tmpDir, "release-"+tag)
	notesTmpDir := filepath.Join(a.tmpDir, "release-notes-"+tag)
	_ = os.RemoveAll(cloneDir)
	_ = os.RemoveAll(notesTmpDir)
	defer os.RemoveAll(cloneDir)
	defer os.RemoveAll(notesTmpDir)

	if err := os.MkdirAll(a.tmpDir, 0o755); err != nil {
		return err
	}
	a.log("Creating temporary clone for %s", tag)
	if err := a.cloneTag(tag, cloneDir); err != nil {
		return err
	}

	cloneApp, err := a.cloneForRepo(cloneDir, notesTmpDir)
	if err != nil {
		return err
	}
	if err := cloneApp.validateChartConfig(); err != nil {
		return err
	}
	helmOCIAuth, err := cloneApp.resolveHelmOCIAuth()
	if err != nil {
		return err
	}
	helmClassicAuth, err := cloneApp.resolveHelmClassicAuth()
	if err != nil {
		return err
	}
	notesFile, err := cloneApp.generateNotes(tag)
	if err != nil {
		return err
	}
	chartDestination, cleanupCharts, err := cloneApp.helmReleasePackageDir(tag)
	if err != nil {
		return err
	}
	defer cleanupCharts()
	packages, err := cloneApp.runHelmPackagesTo(chartVersionFromTag(tag), chartDestination)
	if err != nil {
		return err
	}
	helmOCISession, err := cloneApp.prepareHelmOCIAuth(helmOCIAuth)
	if err != nil {
		return err
	}
	defer helmOCISession.cleanup()

	a.log("Publishing %s", tag)
	if err := cloneApp.runGoreleaserWithToken(token, "release", "--clean", "--release-notes", notesFile); err != nil {
		return err
	}
	if err := cloneApp.updateReleaseBody(tag, notesFile, token); err != nil {
		return err
	}
	ociResults, err := cloneApp.runHelmOCIPushes(packages, helmOCISession)
	if err != nil {
		return err
	}
	if err := cloneApp.runHelmOCISignatures(ociResults); err != nil {
		return err
	}
	if err := cloneApp.runHelmClassicUploads(packages, helmClassicAuth); err != nil {
		return err
	}
	manifestPackages := []string(nil)
	if len(packages) > 0 {
		var err error
		manifestPackages, err = cloneApp.persistHelmPackages(packages)
		if err != nil {
			return err
		}
	}
	if err := cloneApp.writeReleaseManifest(tag, chartVersionFromTag(tag), manifestPackages, rebaseHelmOCIPushResults(ociResults, manifestPackages)); err != nil {
		return err
	}
	if err := a.copyReleaseOutputsFrom(cloneApp); err != nil {
		return err
	}
	if err := a.uploadReleaseManifestIfEnabled(tag, token); err != nil {
		return err
	}
	a.log("Published %s", tag)
	return nil
}

func (a *app) copyReleaseOutputsFrom(source *app) error {
	if err := copyDirectory(filepath.Join(source.repoRoot, "dist", "charts"), filepath.Join(a.repoRoot, "dist", "charts")); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	content, err := os.ReadFile(filepath.Join(source.repoRoot, "dist", "release-manifest.json"))
	if err != nil {
		return err
	}
	var manifest releaseManifest
	if err := json.Unmarshal(content, &manifest); err != nil {
		return err
	}
	for _, artifact := range manifest.Artifacts.GoReleaser {
		if artifact.Path == "" || filepath.IsAbs(artifact.Path) {
			continue
		}
		sourcePath := source.repoPath(filepath.FromSlash(artifact.Path))
		info, err := os.Stat(sourcePath)
		if err != nil {
			return err
		}
		if info.IsDir() {
			continue
		}
		if err := copyFile(sourcePath, a.repoPath(filepath.FromSlash(artifact.Path))); err != nil {
			return err
		}
	}
	manifestPath := filepath.Join(a.repoRoot, "dist", "release-manifest.json")
	if err := os.MkdirAll(filepath.Dir(manifestPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(manifestPath, content, 0o644)
}

func (a *app) clearReleaseManifestOutputs() error {
	if err := os.Remove(filepath.Join(a.repoRoot, "dist", "release-manifest.json")); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if err := os.RemoveAll(filepath.Join(a.repoRoot, "dist", "charts")); err != nil {
		return err
	}
	return nil
}

func (a *app) cloneTag(tag, cloneDir string) error {
	r := a.commandRunner()
	cloneCommand := runner.Command{
		Dir:    "",
		Name:   "git",
		Args:   []string{"clone", "--quiet", "file://" + filepath.Join(a.repoRoot, ".git"), cloneDir},
		Stdout: a.stdout,
		Stderr: a.stderr,
	}
	if err := r.Run(cloneCommand); err != nil {
		return err
	}
	checkoutCommand := runner.Command{
		Dir:    cloneDir,
		Name:   "git",
		Args:   []string{"checkout", "--quiet", "--detach", "refs/tags/" + tag},
		Stdout: a.stdout,
		Stderr: a.stderr,
	}
	return r.Run(checkoutCommand)
}

func (a *app) runGoreleaserWithToken(token string, args ...string) error {
	goreleaserBin, err := a.resolveGoreleaserBin()
	if err != nil {
		return err
	}
	cmdArgs := append([]string{"--config", a.goreleaserConfig()}, args...)
	cmd := runner.Command{
		Dir:    a.repoRoot,
		Name:   goreleaserBin,
		Args:   cmdArgs,
		Env:    append(a.goreleaserEnviron(), a.goreleaserTokenEnv()+"="+token),
		Stdout: a.stdout,
		Stderr: a.stderr,
	}
	return a.commandRunner().Run(cmd)
}

func (a *app) environ() []string {
	return a.environExcept(nil)
}

func (a *app) goreleaserEnviron() []string {
	return a.environExcept(map[string]bool{
		"RELEASE_HELM_OCI_PASSWORD":  true,
		"RELEASE_HELM_CLASSIC_TOKEN": true,
	})
}

func (a *app) environExcept(exclude map[string]bool) []string {
	merged := config.EnvironMap(os.Environ())
	for key, value := range a.env {
		merged[key] = value
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
		if exclude != nil && exclude[key] {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}

func (a *app) verifyTagExists(tag string) error {
	cmd := runner.Command{Dir: "", Name: "git", Args: []string{"-C", a.repoRoot, "rev-parse", "-q", "--verify", "refs/tags/" + tag}, Stdout: io.Discard, Stderr: io.Discard}
	if err := a.commandRunner().Run(cmd); err != nil {
		return fmt.Errorf("tag %s does not exist locally", tag)
	}
	return nil
}

func (a *app) generateNotes(version string) (string, error) {
	tag, err := a.resolveTag(version)
	if err != nil {
		return "", err
	}
	notesMode := a.releaseNotesMode()
	notesSource := a.releaseNotesSource()

	if err := os.MkdirAll(a.tmpDir, 0o755); err != nil {
		return "", err
	}

	switch notesMode {
	case "news-md", "gnu-news":
		newsFile := filepath.Join(a.repoRoot, notesSource)
		content, err := os.ReadFile(newsFile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("release notes source not found: %s", newsFile)
			}
			return "", err
		}
		section := extractNewsSection(string(content), tag, notesMode)
		if section == "" {
			section = fmt.Sprintf("- No summary entry found in `%s`.\n", notesSource)
		} else if !strings.HasSuffix(section, "\n") {
			section += "\n"
		}
		notesFile := filepath.Join(a.tmpDir, "release-notes-"+tag+".md")
		if err := os.WriteFile(notesFile, []byte(section), 0o644); err != nil {
			return "", err
		}
		return notesFile, nil
	case "none":
		return "", errors.New("release notes generation is disabled for this repository")
	default:
		return "", fmt.Errorf("unsupported RELEASE_NOTES_MODE: %s", notesMode)
	}
}

func (a *app) resolveTag(version string) (string, error) {
	if version != "" {
		return version, nil
	}
	if value := a.env["VERSION"]; value != "" {
		return value, nil
	}
	cmd := runner.Command{Dir: "", Name: "git", Args: []string{"-C", a.repoRoot, "describe", "--tags", "--exact-match"}}
	output, err := a.commandRunner().Output(cmd)
	if err == nil {
		if tag := strings.TrimSpace(string(output)); tag != "" {
			return tag, nil
		}
	}
	return "", errors.New("VERSION is required when the current commit is not an exact tag")
}

func extractNewsSection(content, tag, mode string) string {
	switch mode {
	case "gnu-news":
		return extractGNUNewsSection(content, tag)
	default:
		return extractMarkdownNewsSection(content, tag)
	}
}

func extractMarkdownNewsSection(content, tag string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	startPattern := "## " + tag + " - "
	inSection := false
	section := []string{}
	for _, line := range lines {
		if strings.HasPrefix(line, startPattern) {
			inSection = true
			continue
		}
		if inSection && strings.HasPrefix(line, "## ") {
			break
		}
		if inSection {
			section = append(section, line)
		}
	}
	return trimNewsSection(section)
}

func extractGNUNewsSection(content, tag string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	inSection := false
	section := []string{}
	for _, line := range lines {
		if isGNUNewsReleaseHeading(line) {
			if inSection {
				break
			}
			if gnuNewsHeadingMatchesTag(line, tag) {
				inSection = true
			}
			continue
		}
		if inSection {
			section = append(section, line)
		}
	}
	return trimNewsSection(section)
}

func isGNUNewsReleaseHeading(line string) bool {
	return strings.HasPrefix(line, "* Noteworthy changes in release ")
}

func gnuNewsHeadingMatchesTag(line, tag string) bool {
	version := strings.TrimPrefix(tag, "v")
	versions := map[string]bool{tag: true, version: true, "v" + version: true}
	rest := strings.TrimSpace(strings.TrimPrefix(line, "* Noteworthy changes in release "))
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return false
	}
	headingVersion := strings.Trim(fields[0], ":-")
	return versions[headingVersion]
}

func trimNewsSection(section []string) string {
	for len(section) > 0 && strings.TrimSpace(section[0]) == "" {
		section = section[1:]
	}
	for len(section) > 0 && strings.TrimSpace(section[len(section)-1]) == "" {
		section = section[:len(section)-1]
	}
	if len(section) == 0 {
		return ""
	}
	return strings.Join(section, "\n")
}

func (a *app) updateReleaseBody(tag, notesFile, token string) error {
	bodyMode := a.releaseBodyMode()
	if bodyMode == "none" {
		return nil
	}
	if bodyMode != "patch" {
		return fmt.Errorf("unsupported RELEASE_BODY_MODE: %s", bodyMode)
	}

	body, err := os.ReadFile(notesFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("release notes file not found: %s", notesFile)
		}
		return err
	}

	forge, err := a.releaseForge()
	if err != nil {
		return err
	}
	switch forge {
	case forgeGitea:
		return a.updateGiteaReleaseBody(tag, body, token)
	case forgeGitHub:
		return a.updateGitHubReleaseBody(tag, body, token)
	case forgeGitLab:
		return a.updateGitLabReleaseBody(tag, body, token)
	default:
		return fmt.Errorf("unsupported RELEASE_FORGE for release body patching: %s", forge)
	}
}

func (a *app) updateGiteaReleaseBody(tag string, body []byte, token string) error {
	releaseURL := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", a.releaseAPIURL(), a.releaseOwner(), a.releaseRepo(), tag)
	req, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to fetch release %s: %s", tag, resp.Status)
	}
	var release struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return err
	}
	if release.ID == 0 {
		return fmt.Errorf("release id not found for %s", tag)
	}

	payload, err := json.Marshal(map[string]string{"body": string(body)})
	if err != nil {
		return err
	}
	patchURL := fmt.Sprintf("%s/repos/%s/%s/releases/%d", a.releaseAPIURL(), a.releaseOwner(), a.releaseRepo(), release.ID)
	patchReq, err := http.NewRequest(http.MethodPatch, patchURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	patchReq.Header.Set("Authorization", "token "+token)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		return err
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode < 200 || patchResp.StatusCode >= 300 {
		return fmt.Errorf("failed to update release %s: %s", tag, patchResp.Status)
	}
	a.log("Updated release body for %s", tag)
	return nil
}

func (a *app) updateGitHubReleaseBody(tag string, body []byte, token string) error {
	releaseURL := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", a.releaseAPIURL(), a.releaseOwner(), a.releaseRepo(), url.PathEscape(tag))
	req, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to fetch release %s: %s", tag, resp.Status)
	}
	var release struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return err
	}
	if release.ID == 0 {
		return fmt.Errorf("release id not found for %s", tag)
	}

	payload, err := json.Marshal(map[string]string{"body": string(body)})
	if err != nil {
		return err
	}
	patchURL := fmt.Sprintf("%s/repos/%s/%s/releases/%d", a.releaseAPIURL(), a.releaseOwner(), a.releaseRepo(), release.ID)
	patchReq, err := http.NewRequest(http.MethodPatch, patchURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	patchReq.Header.Set("Authorization", "Bearer "+token)
	patchReq.Header.Set("Accept", "application/vnd.github+json")
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		return err
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode < 200 || patchResp.StatusCode >= 300 {
		return fmt.Errorf("failed to update release %s: %s", tag, patchResp.Status)
	}
	a.log("Updated release body for %s", tag)
	return nil
}

func (a *app) updateGitLabReleaseBody(tag string, body []byte, token string) error {
	projectPath := url.PathEscape(a.releaseOwner() + "/" + a.releaseRepo())
	patchURL := fmt.Sprintf("%s/projects/%s/releases/%s", a.releaseAPIURL(), projectPath, url.PathEscape(tag))
	payload, err := json.Marshal(map[string]string{"description": string(body)})
	if err != nil {
		return err
	}
	patchReq, err := http.NewRequest(http.MethodPut, patchURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	patchReq.Header.Set("PRIVATE-TOKEN", token)
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		return err
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode < 200 || patchResp.StatusCode >= 300 {
		return fmt.Errorf("failed to update release %s: %s", tag, patchResp.Status)
	}
	a.log("Updated release body for %s", tag)
	return nil
}

func (a *app) uploadReleaseManifestIfEnabled(tag, token string) error {
	if !a.manifestUpload() {
		return nil
	}
	manifestPath := filepath.Join(a.repoRoot, "dist", "release-manifest.json")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("release manifest not found for upload: %s", manifestPath)
		}
		return err
	}
	forge, err := a.releaseForge()
	if err != nil {
		return err
	}
	switch forge {
	case forgeGitea:
		return a.uploadGiteaReleaseManifest(tag, content, token)
	case forgeGitHub:
		return a.uploadGitHubReleaseManifest(tag, content, token)
	case forgeGitLab:
		return a.uploadGitLabReleaseManifest(tag, content, token)
	default:
		return fmt.Errorf("unsupported RELEASE_FORGE for manifest upload: %s", forge)
	}
}

func (a *app) uploadGiteaReleaseManifest(tag string, content []byte, token string) error {
	releaseURL := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", a.releaseAPIURL(), a.releaseOwner(), a.releaseRepo(), url.PathEscape(tag))
	req, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "token "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to fetch release %s: %s", tag, resp.Status)
	}
	var release struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return err
	}
	if release.ID == 0 {
		return fmt.Errorf("release id not found for %s", tag)
	}
	body, contentType, err := multipartFileBody("attachment", "release-manifest.json", content)
	if err != nil {
		return err
	}
	uploadURL := fmt.Sprintf("%s/repos/%s/%s/releases/%d/assets?name=%s", a.releaseAPIURL(), a.releaseOwner(), a.releaseRepo(), release.ID, url.QueryEscape("release-manifest.json"))
	return a.postReleaseManifestAsset(uploadURL, "token "+token, contentType, body)
}

func (a *app) uploadGitHubReleaseManifest(tag string, content []byte, token string) error {
	releaseURL := fmt.Sprintf("%s/repos/%s/%s/releases/tags/%s", a.releaseAPIURL(), a.releaseOwner(), a.releaseRepo(), url.PathEscape(tag))
	req, err := http.NewRequest(http.MethodGet, releaseURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to fetch release %s: %s", tag, resp.Status)
	}
	var release struct {
		UploadURL string `json:"upload_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return err
	}
	if release.UploadURL == "" {
		return fmt.Errorf("release upload URL not found for %s", tag)
	}
	uploadURL := strings.TrimSuffix(release.UploadURL, "{?name,label}")
	separator := "?"
	if strings.Contains(uploadURL, "?") {
		separator = "&"
	}
	uploadURL += separator + "name=" + url.QueryEscape("release-manifest.json")
	return a.postReleaseManifestAsset(uploadURL, "Bearer "+token, "application/json", bytes.NewReader(content))
}

func (a *app) uploadGitLabReleaseManifest(tag string, content []byte, token string) error {
	projectPath := url.PathEscape(a.releaseOwner() + "/" + a.releaseRepo())
	body, contentType, err := multipartFileBody("file", "release-manifest.json", content)
	if err != nil {
		return err
	}
	uploadURL := fmt.Sprintf("%s/projects/%s/uploads", a.releaseAPIURL(), projectPath)
	uploadReq, err := http.NewRequest(http.MethodPost, uploadURL, body)
	if err != nil {
		return err
	}
	uploadReq.Header.Set("PRIVATE-TOKEN", token)
	uploadReq.Header.Set("Content-Type", contentType)
	uploadResp, err := http.DefaultClient.Do(uploadReq)
	if err != nil {
		return err
	}
	defer uploadResp.Body.Close()
	if uploadResp.StatusCode < 200 || uploadResp.StatusCode >= 300 {
		return fmt.Errorf("failed to upload release manifest to GitLab project: %s", uploadResp.Status)
	}
	var upload struct {
		URL      string `json:"url"`
		FullPath string `json:"full_path"`
	}
	if err := json.NewDecoder(uploadResp.Body).Decode(&upload); err != nil {
		return err
	}
	assetURL := upload.FullPath
	if assetURL == "" {
		assetURL = upload.URL
	}
	if assetURL == "" {
		return errors.New("GitLab upload response did not include a manifest URL")
	}
	if strings.HasPrefix(assetURL, "/") {
		assetURL = strings.TrimRight(gitLabWebURL(a.releaseAPIURL()), "/") + assetURL
	}
	payload, err := json.Marshal(map[string]string{"name": "release-manifest.json", "url": assetURL, "link_type": "other"})
	if err != nil {
		return err
	}
	linkURL := fmt.Sprintf("%s/projects/%s/releases/%s/assets/links", a.releaseAPIURL(), projectPath, url.PathEscape(tag))
	linkReq, err := http.NewRequest(http.MethodPost, linkURL, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	linkReq.Header.Set("PRIVATE-TOKEN", token)
	linkReq.Header.Set("Content-Type", "application/json")
	linkResp, err := http.DefaultClient.Do(linkReq)
	if err != nil {
		return err
	}
	defer linkResp.Body.Close()
	if linkResp.StatusCode < 200 || linkResp.StatusCode >= 300 {
		return fmt.Errorf("failed to add GitLab release manifest asset link: %s", linkResp.Status)
	}
	a.log("Uploaded release manifest for %s", tag)
	return nil
}

func (a *app) postReleaseManifestAsset(uploadURL, authorization, contentType string, body io.Reader) error {
	req, err := http.NewRequest(http.MethodPost, uploadURL, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", authorization)
	req.Header.Set("Content-Type", contentType)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("failed to upload release manifest: %s", resp.Status)
	}
	a.log("Uploaded release manifest")
	return nil
}

func multipartFileBody(fieldName, fileName string, content []byte) (io.Reader, string, error) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile(fieldName, fileName)
	if err != nil {
		return nil, "", err
	}
	if _, err := file.Write(content); err != nil {
		return nil, "", err
	}
	if err := writer.Close(); err != nil {
		return nil, "", err
	}
	return bytes.NewReader(body.Bytes()), writer.FormDataContentType(), nil
}

func gitLabWebURL(apiURL string) string {
	return strings.TrimSuffix(strings.TrimRight(apiURL, "/"), "/api/v4")
}

func (a *app) resolveToken() (string, error) {
	if _, err := a.releaseForge(); err != nil {
		return "", err
	}
	tokenEnv := a.goreleaserTokenEnv()
	if token, ok := a.resolveEnvironmentToken(); ok {
		return token, nil
	}
	if tokenFile := a.env["RELEASE_TOKEN_FILE"]; tokenFile != "" {
		token, err := readTokenFile(expandTokenFilePath(tokenFile))
		if err != nil {
			return "", err
		}
		return token, nil
	}
	return "", fmt.Errorf("RELEASE_TOKEN, %s, or RELEASE_TOKEN_FILE is required for RELEASE_FORGE=%s", tokenEnv, a.releaseForgeName())
}

func readTokenFile(path string) (string, error) {
	return readSecretFile("RELEASE_TOKEN_FILE", path)
}

func readSecretFile(label, path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read %s %s: %w", label, path, err)
	}
	token := strings.TrimRight(string(content), "\r\n")
	if token == "" {
		return "", fmt.Errorf("%s is empty: %s", label, path)
	}
	return token, nil
}

func expandTokenFilePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	for _, prefix := range []string{"~/", "$HOME/", "${HOME}/"} {
		if rest, ok := strings.CutPrefix(path, prefix); ok {
			return filepath.Join(home, rest)
		}
	}
	if path == "~" || path == "$HOME" || path == "${HOME}" {
		return home
	}
	return path
}

func (a *app) resolveOptionalToken() (string, bool) {
	if _, err := a.releaseForge(); err != nil {
		return "", false
	}
	return a.resolveEnvironmentToken()
}

func (a *app) resolveEnvironmentToken() (string, bool) {
	if token := a.env["RELEASE_TOKEN"]; token != "" {
		return token, true
	}
	tokenEnv := a.goreleaserTokenEnv()
	if token := a.env[tokenEnv]; token != "" {
		return token, true
	}
	return "", false
}

func (a *app) releaseForge() (forgeKind, error) {
	switch strings.ToLower(envValue(a.env, "RELEASE_FORGE", string(forgeGitea))) {
	case "gitea", "forgejo", "codeberg":
		return forgeGitea, nil
	case "github":
		return forgeGitHub, nil
	case "gitlab":
		return forgeGitLab, nil
	default:
		return "", fmt.Errorf("unsupported RELEASE_FORGE: %s", a.env["RELEASE_FORGE"])
	}
}

func (a *app) releaseForgeName() string {
	name := strings.ToLower(envValue(a.env, "RELEASE_FORGE", string(forgeGitea)))
	_, err := a.releaseForge()
	if err != nil {
		return name
	}
	return name
}

func (a *app) goreleaserTokenEnv() string {
	forge, err := a.releaseForge()
	if err != nil {
		return "RELEASE_TOKEN"
	}
	switch forge {
	case forgeGitHub:
		return "GITHUB_TOKEN"
	case forgeGitLab:
		return "GITLAB_TOKEN"
	default:
		return "GITEA_TOKEN"
	}
}

func (a *app) releaseOwner() string {
	return a.env["RELEASE_OWNER"]
}

func (a *app) releaseRepo() string {
	if repo := a.env["RELEASE_REPO"]; repo != "" {
		return repo
	}
	return a.env["RELEASE_PROJECT"]
}

func (a *app) releaseAPIURL() string {
	if apiURL := a.env["RELEASE_API_URL"]; apiURL != "" {
		return apiURL
	}
	forge, err := a.releaseForge()
	if err != nil {
		return "https://codeberg.org/api/v1"
	}
	switch forge {
	case forgeGitHub:
		return "https://api.github.com"
	case forgeGitLab:
		return "https://gitlab.com/api/v4"
	default:
		return "https://codeberg.org/api/v1"
	}
}

func (a *app) releaseNotesSource() string {
	return envValue(a.env, "RELEASE_NOTES_SOURCE", "NEWS.md")
}

func (a *app) releaseNotesMode() string {
	return envValue(a.env, "RELEASE_NOTES_MODE", "news-md")
}

func (a *app) releaseBodyMode() string {
	return envValue(a.env, "RELEASE_BODY_MODE", "none")
}

func (a *app) manifestUpload() bool {
	return configBool(a.env["RELEASE_MANIFEST_UPLOAD"])
}

func (a *app) validateManifestUploadConfig() error {
	return validateConfigBool("RELEASE_MANIFEST_UPLOAD", a.env["RELEASE_MANIFEST_UPLOAD"])
}

func (a *app) goreleaserConfig() string {
	return envValue(a.env, "GORELEASER_CONFIG", ".goreleaser.yaml")
}

func (a *app) detectGoreleaserContainerConfig() (goreleaserContainerConfig, error) {
	config := goreleaserContainerConfig{tools: map[string]map[string]bool{}}
	content, err := os.ReadFile(filepath.Join(a.repoRoot, a.goreleaserConfig()))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return config, nil
		}
		return config, err
	}
	blocks := topLevelYAMLBlocks(string(content), "dockers", "dockers_v2", "docker_manifests", "docker_signs")
	for _, key := range []string{"dockers", "dockers_v2", "docker_manifests", "docker_signs"} {
		block, ok := blocks[key]
		if !ok || yamlBlockDisabled(block) {
			continue
		}
		config.keys = append(config.keys, key)
		items := yamlListItems(block)
		if len(items) == 0 {
			items = [][]string{block}
		}
		switch key {
		case "dockers", "docker_manifests":
			for _, item := range items {
				uses := yamlScalarValues(item, "use")
				if len(uses) == 0 {
					config.addTool("docker", key)
					continue
				}
				for _, use := range uses {
					switch use {
					case "podman":
						config.addTool("podman", key)
					case "docker", "buildx", "":
						config.addTool("docker", key)
					}
				}
			}
		case "dockers_v2":
			config.addTool("docker", key)
		case "docker_signs":
			for _, item := range items {
				cmds := yamlScalarValues(item, "cmd")
				if len(cmds) == 0 {
					config.addTool("cosign", key)
					continue
				}
				for _, cmd := range cmds {
					tool := firstCommandWord(cmd)
					config.addTool(tool, key)
				}
			}
		}
	}
	return config, nil
}

func topLevelYAMLBlocks(content string, keys ...string) map[string][]string {
	wanted := map[string]bool{}
	for _, key := range keys {
		wanted[key] = true
	}
	blocks := map[string][]string{}
	current := ""
	for _, raw := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			if current != "" {
				blocks[current] = append(blocks[current], raw)
			}
			continue
		}
		if raw == strings.TrimLeft(raw, " \t") {
			key, _, ok := strings.Cut(trimmed, ":")
			key = strings.TrimSpace(strings.Trim(key, "'\""))
			if ok && wanted[key] {
				current = key
				blocks[current] = append(blocks[current], raw)
				continue
			}
			current = ""
			continue
		}
		if current != "" {
			blocks[current] = append(blocks[current], raw)
		}
	}
	return blocks
}

func yamlScalarValues(block []string, key string) []string {
	values := []string{}
	prefix := key + ":"
	for _, raw := range block {
		line := strings.TrimSpace(raw)
		line = strings.TrimSpace(strings.TrimPrefix(line, "-"))
		if !strings.HasPrefix(line, prefix) {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, prefix))
		if beforeComment, _, ok := strings.Cut(value, "#"); ok {
			value = beforeComment
		}
		values = append(values, strings.Trim(strings.TrimSpace(value), "'\""))
	}
	return values
}

func yamlListItems(block []string) [][]string {
	items := [][]string{}
	current := -1
	itemIndent := -1
	for _, raw := range block {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.HasPrefix(trimmed, "-") {
			if current >= 0 {
				items[current] = append(items[current], raw)
			}
			continue
		}
		indent := leadingIndent(raw)
		if itemIndent == -1 {
			itemIndent = indent
		}
		if indent == itemIndent {
			items = append(items, []string{raw})
			current = len(items) - 1
			continue
		}
		if current >= 0 {
			items[current] = append(items[current], raw)
		}
	}
	return items
}

func leadingIndent(value string) int {
	indent := 0
	for _, r := range value {
		if r != ' ' && r != '\t' {
			break
		}
		indent++
	}
	return indent
}

func yamlBlockDisabled(block []string) bool {
	meaningful := []string{}
	for _, raw := range block {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		meaningful = append(meaningful, raw)
	}
	if len(meaningful) != 1 {
		return false
	}
	_, value, ok := strings.Cut(strings.TrimSpace(meaningful[0]), ":")
	if !ok {
		return false
	}
	if beforeComment, _, ok := strings.Cut(value, "#"); ok {
		value = beforeComment
	}
	switch strings.TrimSpace(value) {
	case "", "|", ">":
		return false
	case "[]", "{}", "null", "~":
		return true
	default:
		return false
	}
}

func firstCommandWord(command string) string {
	command = strings.TrimSpace(command)
	if command == "" || command == "|" || command == ">" || strings.Contains(command, "{{") {
		return ""
	}
	fields := strings.Fields(command)
	for i, field := range fields {
		field = strings.Trim(field, "'\"")
		if field == "env" || isShellAssignment(field) {
			continue
		}
		if (field == "sh" || field == "bash") && i+2 < len(fields) && strings.Trim(fields[i+1], "'\"") == "-c" {
			inner := strings.Trim(strings.Join(fields[i+2:], " "), "'\"")
			return firstCommandWord(inner)
		}
		return field
	}
	return ""
}

func isShellAssignment(value string) bool {
	name, _, ok := strings.Cut(value, "=")
	if !ok || name == "" {
		return false
	}
	for i, r := range name {
		if r == '_' || r >= 'A' && r <= 'Z' || r >= 'a' && r <= 'z' || i > 0 && r >= '0' && r <= '9' {
			continue
		}
		return false
	}
	return true
}

func (c goreleaserContainerConfig) enabled() bool {
	return len(c.keys) > 0
}

func (c goreleaserContainerConfig) toolNames() []string {
	tools := make([]string, 0, len(c.tools))
	for tool := range c.tools {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	return tools
}

func (c goreleaserContainerConfig) toolKeys(tool string) []string {
	keys := []string{}
	for key := range c.tools[tool] {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c goreleaserContainerConfig) addTool(tool, key string) {
	if tool == "" {
		return
	}
	if c.tools == nil {
		c.tools = map[string]map[string]bool{}
	}
	if c.tools[tool] == nil {
		c.tools[tool] = map[string]bool{}
	}
	c.tools[tool][key] = true
}

func (a *app) releaseArtifacts() ([]string, error) {
	raw, ok := a.env["RELEASE_ARTIFACTS"]
	if !ok {
		raw = artifactBinaries
	} else if strings.TrimSpace(raw) == "" {
		return nil, errors.New("RELEASE_ARTIFACTS is empty")
	}
	seen := map[string]bool{}
	artifacts := []string{}
	for _, entry := range strings.Split(raw, ",") {
		artifact := strings.TrimSpace(entry)
		if artifact == "" {
			return nil, errors.New("RELEASE_ARTIFACTS contains an empty entry")
		}
		switch artifact {
		case artifactBinaries, artifactCharts:
		default:
			return nil, fmt.Errorf("unsupported RELEASE_ARTIFACTS value: %s", artifact)
		}
		if !seen[artifact] {
			artifacts = append(artifacts, artifact)
			seen[artifact] = true
		}
	}
	return artifacts, nil
}

func (a *app) chartsEnabled() (bool, error) {
	artifacts, err := a.releaseArtifacts()
	if err != nil {
		return false, err
	}
	for _, artifact := range artifacts {
		if artifact == artifactCharts {
			return true, nil
		}
	}
	return false, nil
}

func (a *app) validateChartConfig() error {
	if _, err := a.helmVersionFromChecked(); err != nil {
		return err
	}
	if _, err := a.helmAppVersionFromChecked(); err != nil {
		return err
	}
	if _, err := a.helmOCIRepositoryChecked(); err != nil {
		return err
	}
	if err := a.validateHelmOCIAuthConfig(); err != nil {
		return err
	}
	if err := a.validateHelmOCIPlainHTTPConfig(); err != nil {
		return err
	}
	if err := a.validateHelmOCISignConfig(); err != nil {
		return err
	}
	if err := a.validateHelmClassicConfig(); err != nil {
		return err
	}
	if err := validateConfigBool("RELEASE_HELM_PROVENANCE", a.env["RELEASE_HELM_PROVENANCE"]); err != nil {
		return err
	}
	enabled, err := a.chartsEnabled()
	if err != nil {
		return err
	}
	if !enabled && a.helmOCIRepository() != "" {
		return errors.New("RELEASE_HELM_OCI_REPOSITORY requires RELEASE_ARTIFACTS to include charts")
	}
	if !enabled && a.env["RELEASE_HELM_OCI_PLAIN_HTTP"] != "" {
		return errors.New("RELEASE_HELM_OCI_PLAIN_HTTP requires RELEASE_ARTIFACTS to include charts")
	}
	if !enabled && a.helmOCISignConfigured() {
		return errors.New("RELEASE_HELM_OCI_SIGNER and RELEASE_HELM_OCI_SIGN_ARGS require RELEASE_ARTIFACTS to include charts")
	}
	if !enabled && a.helmClassicURL() != "" {
		return errors.New("RELEASE_HELM_CLASSIC_URL requires RELEASE_ARTIFACTS to include charts")
	}
	if !enabled && a.helmProvenanceConfigured() {
		return errors.New("RELEASE_HELM_PROVENANCE, RELEASE_HELM_GPG_KEY, and RELEASE_HELM_GPG_KEYRING require RELEASE_ARTIFACTS to include charts")
	}
	if !enabled {
		return nil
	}
	if err := a.validateHelmProvenanceConfig(); err != nil {
		return err
	}
	dirs, err := a.helmChartDirs()
	if err != nil {
		return err
	}
	repoRealPath, err := filepath.EvalSymlinks(a.repoRoot)
	if err != nil {
		return err
	}
	for _, dir := range dirs {
		chartDir := filepath.Join(a.repoRoot, dir)
		info, err := os.Stat(chartDir)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("Helm chart dir not found: %s", chartDir)
			}
			return err
		}
		if !info.IsDir() {
			return fmt.Errorf("Helm chart dir is not a directory: %s", chartDir)
		}
		chartRealPath, err := filepath.EvalSymlinks(chartDir)
		if err != nil {
			return err
		}
		inside, err := pathInside(repoRealPath, chartRealPath)
		if err != nil {
			return err
		}
		if !inside {
			return fmt.Errorf("Helm chart dir must stay inside repository: %s", chartDir)
		}
		chartFile := filepath.Join(chartDir, "Chart.yaml")
		if _, err := os.ReadFile(chartFile); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("Helm Chart.yaml not found: %s", chartFile)
			}
			return fmt.Errorf("Helm Chart.yaml is not readable: %s", chartFile)
		}
	}
	return nil
}

func pathInside(base, target string) (bool, error) {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return false, err
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)), nil
}

func (a *app) helmChartDirs() ([]string, error) {
	enabled, err := a.chartsEnabled()
	if err != nil {
		return nil, err
	}
	if !enabled {
		return nil, nil
	}
	raw, ok := a.env["RELEASE_HELM_CHART_DIRS"]
	if !ok || strings.TrimSpace(raw) == "" {
		return nil, errors.New("RELEASE_HELM_CHART_DIRS is required when RELEASE_ARTIFACTS includes charts")
	}
	seen := map[string]bool{}
	dirs := []string{}
	for _, entry := range strings.Split(raw, ",") {
		dir := strings.TrimSpace(entry)
		if dir == "" {
			return nil, errors.New("RELEASE_HELM_CHART_DIRS contains an empty entry")
		}
		if filepath.IsAbs(dir) {
			return nil, fmt.Errorf("RELEASE_HELM_CHART_DIRS must be relative paths inside the repository: %s", dir)
		}
		dir = filepath.Clean(dir)
		if dir == ".." || strings.HasPrefix(dir, ".."+string(filepath.Separator)) {
			return nil, fmt.Errorf("RELEASE_HELM_CHART_DIRS must be relative paths inside the repository: %s", dir)
		}
		if !seen[dir] {
			dirs = append(dirs, dir)
			seen[dir] = true
		}
	}
	return dirs, nil
}

func (a *app) helmVersionFrom() string {
	return envValue(a.env, "RELEASE_HELM_VERSION_FROM", helmVersionFromTag)
}

func (a *app) helmAppVersionFrom() string {
	return envValue(a.env, "RELEASE_HELM_APP_VERSION_FROM", helmVersionFromTag)
}

func (a *app) helmVersionFromChecked() (string, error) {
	value := a.helmVersionFrom()
	if value != helmVersionFromTag {
		return "", fmt.Errorf("unsupported RELEASE_HELM_VERSION_FROM: %s", value)
	}
	return value, nil
}

func (a *app) helmAppVersionFromChecked() (string, error) {
	value := a.helmAppVersionFrom()
	if value != helmVersionFromTag {
		return "", fmt.Errorf("unsupported RELEASE_HELM_APP_VERSION_FROM: %s", value)
	}
	return value, nil
}

func (a *app) helmOCIRepository() string {
	return strings.TrimSpace(a.env["RELEASE_HELM_OCI_REPOSITORY"])
}

func (a *app) helmOCIPlainHTTP() bool {
	return configBool(a.env["RELEASE_HELM_OCI_PLAIN_HTTP"])
}

func (a *app) helmOCISigner() string {
	value := strings.ToLower(strings.TrimSpace(a.env["RELEASE_HELM_OCI_SIGNER"]))
	if value == "" {
		return "none"
	}
	return value
}

func (a *app) helmOCISignArgs() string {
	return strings.TrimSpace(a.env["RELEASE_HELM_OCI_SIGN_ARGS"])
}

func configBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func validateConfigBool(key, value string) error {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "1", "false", "true", "no", "yes", "off", "on":
		return nil
	default:
		return fmt.Errorf("%s must be a boolean value", key)
	}
}

func (a *app) helmProvenance() bool {
	return configBool(a.env["RELEASE_HELM_PROVENANCE"])
}

func (a *app) helmGPGKey() string {
	return strings.TrimSpace(a.env["RELEASE_HELM_GPG_KEY"])
}

func (a *app) helmGPGKeyring() string {
	return strings.TrimSpace(a.env["RELEASE_HELM_GPG_KEYRING"])
}

func (a *app) helmGPGKeyringPath() string {
	keyring := expandTokenFilePath(a.helmGPGKeyring())
	if keyring == "" || filepath.IsAbs(keyring) {
		return keyring
	}
	return filepath.Join(a.repoRoot, keyring)
}

func (a *app) helmProvenanceConfigured() bool {
	return a.env["RELEASE_HELM_PROVENANCE"] != "" || a.helmGPGKey() != "" || a.helmGPGKeyring() != ""
}

func (a *app) validateHelmProvenanceConfig() error {
	if err := validateConfigBool("RELEASE_HELM_PROVENANCE", a.env["RELEASE_HELM_PROVENANCE"]); err != nil {
		return err
	}
	if !a.helmProvenance() {
		if a.helmGPGKey() != "" || a.helmGPGKeyring() != "" {
			return errors.New("RELEASE_HELM_GPG_KEY and RELEASE_HELM_GPG_KEYRING require RELEASE_HELM_PROVENANCE=true")
		}
		return nil
	}
	if a.helmGPGKey() == "" {
		return errors.New("RELEASE_HELM_GPG_KEY is required when RELEASE_HELM_PROVENANCE=true")
	}
	if a.helmGPGKeyring() == "" {
		return errors.New("RELEASE_HELM_GPG_KEYRING is required when RELEASE_HELM_PROVENANCE=true")
	}
	file, err := os.Open(a.helmGPGKeyringPath())
	if err != nil {
		return fmt.Errorf("RELEASE_HELM_GPG_KEYRING is not readable: %s", a.helmGPGKeyringPath())
	}
	return file.Close()
}

func (a *app) helmOCIRepositoryChecked() (string, error) {
	repository := a.helmOCIRepository()
	if repository == "" {
		return "", nil
	}
	if strings.ContainsAny(repository, " \t\r\n") || !strings.HasPrefix(repository, "oci://") || strings.TrimPrefix(repository, "oci://") == "" {
		return "", fmt.Errorf("RELEASE_HELM_OCI_REPOSITORY must be an oci:// repository: %s", repository)
	}
	return repository, nil
}

func helmOCIRegistryHost(repository string) (string, error) {
	if !strings.HasPrefix(repository, "oci://") {
		return "", fmt.Errorf("RELEASE_HELM_OCI_REPOSITORY must be an oci:// repository: %s", repository)
	}
	rest := strings.TrimPrefix(repository, "oci://")
	host, _, _ := strings.Cut(rest, "/")
	if host == "" {
		return "", fmt.Errorf("RELEASE_HELM_OCI_REPOSITORY must include a registry host: %s", repository)
	}
	return host, nil
}

func (a *app) helmOCIUsername() string {
	return strings.TrimSpace(a.env["RELEASE_HELM_OCI_USERNAME"])
}

func (a *app) helmOCIPasswordFile() string {
	return strings.TrimSpace(a.env["RELEASE_HELM_OCI_PASSWORD_FILE"])
}

func (a *app) helmOCIPasswordEnv() string {
	return a.env["RELEASE_HELM_OCI_PASSWORD"]
}

func (a *app) helmOCIAuthConfigured() bool {
	return a.helmOCIUsername() != "" || a.helmOCIPasswordEnv() != "" || a.helmOCIPasswordFile() != ""
}

func (a *app) validateHelmOCIAuthConfig() error {
	if !a.helmOCIAuthConfigured() {
		return nil
	}
	if a.helmOCIRepository() == "" {
		return errors.New("RELEASE_HELM_OCI_USERNAME, RELEASE_HELM_OCI_PASSWORD, and RELEASE_HELM_OCI_PASSWORD_FILE require RELEASE_HELM_OCI_REPOSITORY")
	}
	if a.helmOCIUsername() == "" {
		return errors.New("RELEASE_HELM_OCI_USERNAME is required when Helm OCI auth is configured")
	}
	if a.helmOCIPasswordEnv() == "" && a.helmOCIPasswordFile() == "" {
		return errors.New("RELEASE_HELM_OCI_PASSWORD or RELEASE_HELM_OCI_PASSWORD_FILE is required when RELEASE_HELM_OCI_USERNAME is set")
	}
	return nil
}

func (a *app) validateHelmOCIPlainHTTPConfig() error {
	if err := validateConfigBool("RELEASE_HELM_OCI_PLAIN_HTTP", a.env["RELEASE_HELM_OCI_PLAIN_HTTP"]); err != nil {
		return err
	}
	if a.env["RELEASE_HELM_OCI_PLAIN_HTTP"] != "" && a.helmOCIRepository() == "" {
		return errors.New("RELEASE_HELM_OCI_PLAIN_HTTP requires RELEASE_HELM_OCI_REPOSITORY")
	}
	return nil
}

func (a *app) helmOCISignConfigured() bool {
	return a.env["RELEASE_HELM_OCI_SIGNER"] != "" || a.helmOCISignArgs() != ""
}

func (a *app) validateHelmOCISignConfig() error {
	signer := a.helmOCISigner()
	switch signer {
	case "none", "cosign":
	default:
		return fmt.Errorf("unsupported RELEASE_HELM_OCI_SIGNER: %s", signer)
	}
	if signer == "none" && a.helmOCISignArgs() != "" {
		return errors.New("RELEASE_HELM_OCI_SIGN_ARGS requires RELEASE_HELM_OCI_SIGNER")
	}
	if signer != "none" && a.helmOCIRepository() == "" {
		return errors.New("RELEASE_HELM_OCI_SIGNER requires RELEASE_HELM_OCI_REPOSITORY")
	}
	return nil
}

func (a *app) resolveHelmOCIAuth() (*helmOCIAuth, error) {
	if !a.helmOCIAuthConfigured() {
		return nil, nil
	}
	if err := a.validateHelmOCIAuthConfig(); err != nil {
		return nil, err
	}
	password := a.helmOCIPasswordEnv()
	if password == "" {
		value, err := readSecretFile("RELEASE_HELM_OCI_PASSWORD_FILE", expandTokenFilePath(a.helmOCIPasswordFile()))
		if err != nil {
			return nil, err
		}
		password = value
	}
	return &helmOCIAuth{username: a.helmOCIUsername(), password: password}, nil
}

func (a *app) helmClassicURL() string {
	return strings.TrimRight(strings.TrimSpace(a.env["RELEASE_HELM_CLASSIC_URL"]), "/")
}

func (a *app) helmClassicUsername() string {
	return strings.TrimSpace(a.env["RELEASE_HELM_CLASSIC_USERNAME"])
}

func (a *app) helmClassicTokenFile() string {
	return strings.TrimSpace(a.env["RELEASE_HELM_CLASSIC_TOKEN_FILE"])
}

func (a *app) helmClassicTokenEnv() string {
	return a.env["RELEASE_HELM_CLASSIC_TOKEN"]
}

func (a *app) helmClassicAuthConfigured() bool {
	return a.helmClassicUsername() != "" || a.helmClassicTokenEnv() != "" || a.helmClassicTokenFile() != ""
}

func (a *app) validateHelmClassicConfig() error {
	classicURL := a.helmClassicURL()
	if classicURL == "" {
		if a.helmClassicAuthConfigured() {
			return errors.New("RELEASE_HELM_CLASSIC_TOKEN and RELEASE_HELM_CLASSIC_TOKEN_FILE require RELEASE_HELM_CLASSIC_URL")
		}
		return nil
	}
	parsed, err := url.Parse(classicURL)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.Scheme != "https" {
		return fmt.Errorf("RELEASE_HELM_CLASSIC_URL must be an https:// URL: %s", classicURL)
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("RELEASE_HELM_CLASSIC_URL must not include credentials, query, or fragment: %s", classicURL)
	}
	if strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/api/charts") {
		return fmt.Errorf("RELEASE_HELM_CLASSIC_URL must be the Helm package base URL, not the upload endpoint: %s", classicURL)
	}
	if a.helmClassicAuthConfigured() && a.helmClassicUsername() == "" {
		return errors.New("RELEASE_HELM_CLASSIC_USERNAME is required when Helm classic auth is configured")
	}
	return nil
}

func helmClassicUploadURL(classicURL string) (string, error) {
	parsed, err := url.Parse(classicURL)
	if err != nil {
		return "", err
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/") + "/api/charts"
	parsed.RawPath = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String(), nil
}

func (a *app) resolveHelmClassicAuth() (*helmClassicAuth, error) {
	if a.helmClassicURL() == "" {
		return nil, nil
	}
	if a.helmClassicUsername() == "" {
		return nil, errors.New("RELEASE_HELM_CLASSIC_USERNAME is required when RELEASE_HELM_CLASSIC_URL is set")
	}
	if token := a.helmClassicTokenEnv(); token != "" {
		return &helmClassicAuth{username: a.helmClassicUsername(), token: token}, nil
	}
	if tokenFile := a.helmClassicTokenFile(); tokenFile != "" {
		token, err := readSecretFile("RELEASE_HELM_CLASSIC_TOKEN_FILE", expandTokenFilePath(tokenFile))
		if err != nil {
			return nil, err
		}
		return &helmClassicAuth{username: a.helmClassicUsername(), token: token}, nil
	}
	return nil, errors.New("RELEASE_HELM_CLASSIC_TOKEN or RELEASE_HELM_CLASSIC_TOKEN_FILE is required when RELEASE_HELM_CLASSIC_URL is set")
}

func (a *app) helmVersion() (string, error) {
	if _, err := a.helmVersionFromChecked(); err != nil {
		return "", err
	}
	tag, err := a.resolveTag("")
	if err != nil {
		return "", err
	}
	version := chartVersionFromTag(tag)
	if version == "" {
		return "", fmt.Errorf("release tag does not contain a Helm version: %s", tag)
	}
	return version, nil
}

func chartVersionFromTag(tag string) string {
	return strings.TrimPrefix(tag, "v")
}

func (a *app) log(format string, args ...any) {
	stdout := a.stdout
	if stdout == nil {
		stdout = io.Discard
	}
	fmt.Fprintf(stdout, "[INFO] "+format+"\n", args...)
}

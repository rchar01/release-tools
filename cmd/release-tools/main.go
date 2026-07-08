package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	"RELEASE_FORGE":                 true,
	"RELEASE_PROJECT":               true,
	"RELEASE_OWNER":                 true,
	"RELEASE_REPO":                  true,
	"RELEASE_API_URL":               true,
	"RELEASE_ARTIFACTS":             true,
	"RELEASE_HELM_CHART_DIRS":       true,
	"RELEASE_HELM_VERSION_FROM":     true,
	"RELEASE_HELM_APP_VERSION_FROM": true,
	"RELEASE_HELM_OCI_REPOSITORY":   true,
	"RELEASE_NOTES_SOURCE":          true,
	"RELEASE_NOTES_MODE":            true,
	"RELEASE_BODY_MODE":             true,
	"GORELEASER_CONFIG":             true,
	"GORELEASER_BIN":                true,
	"RELEASE_REQUIRE_GO":            true,
	"RELEASE_TOKEN_FILE":            true,
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
	if err := a.runGoreleaser("check"); err != nil {
		return err
	}
	return a.runHelmChecks()
}

func (a *app) snapshot() error {
	if err := a.validateChartConfig(); err != nil {
		return err
	}
	chartVersion := ""
	if enabled, err := a.chartsEnabled(); err != nil {
		return err
	} else if enabled {
		version, err := a.helmVersion()
		if err != nil {
			return err
		}
		chartVersion = version
	}
	if err := a.runGoreleaser("release", "--snapshot", "--skip=publish", "--clean"); err != nil {
		return err
	}
	_, err := a.runHelmPackages(chartVersion)
	return err
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

	if err := a.ensureTools(); err != nil {
		return err
	}
	goreleaserBin, err := a.resolveGoreleaserBin()
	if err != nil {
		return err
	}
	goreleaserVersion := resolveGoreleaserVersion(a.commandRunner(), goreleaserBin)

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
	a.log("GoReleaser version: %s", goreleaserVersion)
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
		if repository := a.helmOCIRepository(); repository != "" {
			a.log("Helm OCI repository: %s", repository)
		}
	}
	a.log("Release notes mode: %s", notesMode)
	a.log("Release body mode: %s", bodyMode)
	a.log("release-tools configuration looks valid")
	return nil
}

func resolveGoreleaserVersion(r runner.Runner, goreleaserBin string) string {
	if r == nil {
		r = runner.OSRunner{}
	}
	output, err := r.CombinedOutput(runner.NewCommand("", goreleaserBin, "--version"))
	if err != nil {
		return "unknown"
	}
	if version := parseGoreleaserVersion(string(output)); version != "" {
		return version
	}
	return "unknown"
}

func parseGoreleaserVersion(output string) string {
	for _, raw := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if version, ok := strings.CutPrefix(line, "GitVersion:"); ok {
			return strings.TrimSpace(version)
		}
	}
	return ""
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
		_, err := a.resolveHelmBin()
		return err
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
	cmd := runner.Command{Dir: a.repoRoot, Name: goreleaserBin, Args: cmdArgs, Env: a.environ(), Stdout: a.stdout, Stderr: a.stderr}
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
	destination := filepath.Join("dist", "charts")
	if err := os.MkdirAll(filepath.Join(a.repoRoot, destination), 0o755); err != nil {
		return nil, err
	}
	packages := []string{}
	for _, dir := range dirs {
		before, err := a.helmPackageFiles(destination, version)
		if err != nil {
			return nil, err
		}
		if err := a.runHelm(helmBin, "package", dir, "--version", version, "--app-version", appVersion, "--destination", destination); err != nil {
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
		packages = append(packages, chartPackage)
	}
	return packages, nil
}

func (a *app) helmPackageFiles(destination, version string) (map[string]fileState, error) {
	entries, err := os.ReadDir(filepath.Join(a.repoRoot, destination))
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

func (a *app) runHelmOCIPushes(packages []string) error {
	if len(packages) == 0 {
		return nil
	}
	repository := a.helmOCIRepository()
	if repository == "" {
		return nil
	}
	if _, err := a.helmOCIRepositoryChecked(); err != nil {
		return err
	}
	helmBin, err := a.resolveHelmBin()
	if err != nil {
		return err
	}
	for _, chartPackage := range packages {
		if err := a.runHelm(helmBin, "push", chartPackage, repository); err != nil {
			return err
		}
	}
	return nil
}

func (a *app) runHelm(helmBin string, args ...string) error {
	cmd := runner.Command{Dir: a.repoRoot, Name: helmBin, Args: args, Stdout: a.stdout, Stderr: a.stderr}
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
	notesFile, err := a.generateNotes(tag)
	if err != nil {
		return err
	}
	packages, err := a.runHelmPackages(chartVersionFromTag(tag))
	if err != nil {
		return err
	}
	if err := a.runGoreleaserWithToken(token, "release", "--clean", "--release-notes", notesFile); err != nil {
		return err
	}
	if err := a.updateReleaseBody(tag, notesFile, token); err != nil {
		return err
	}
	return a.runHelmOCIPushes(packages)
}

func (a *app) publishTag(tag string) error {
	token, err := a.resolveToken()
	if err != nil {
		return err
	}
	if err := a.verifyTagExists(tag); err != nil {
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
	notesFile, err := cloneApp.generateNotes(tag)
	if err != nil {
		return err
	}
	packages, err := cloneApp.runHelmPackages(chartVersionFromTag(tag))
	if err != nil {
		return err
	}

	a.log("Publishing %s", tag)
	if err := cloneApp.runGoreleaserWithToken(token, "release", "--clean", "--release-notes", notesFile); err != nil {
		return err
	}
	if err := cloneApp.updateReleaseBody(tag, notesFile, token); err != nil {
		return err
	}
	if err := cloneApp.runHelmOCIPushes(packages); err != nil {
		return err
	}
	a.log("Published %s", tag)
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
		Env:    append(a.environ(), a.goreleaserTokenEnv()+"="+token),
		Stdout: a.stdout,
		Stderr: a.stderr,
	}
	return a.commandRunner().Run(cmd)
}

func (a *app) environ() []string {
	merged := config.EnvironMap(os.Environ())
	for key, value := range a.env {
		merged[key] = value
	}
	out := make([]string, 0, len(merged))
	for key, value := range merged {
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
	content, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("failed to read RELEASE_TOKEN_FILE %s: %w", path, err)
	}
	token := strings.TrimRight(string(content), "\r\n")
	if token == "" {
		return "", fmt.Errorf("RELEASE_TOKEN_FILE is empty: %s", path)
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

func (a *app) goreleaserConfig() string {
	return envValue(a.env, "GORELEASER_CONFIG", ".goreleaser.yaml")
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
	enabled, err := a.chartsEnabled()
	if err != nil {
		return err
	}
	if !enabled && a.helmOCIRepository() != "" {
		return errors.New("RELEASE_HELM_OCI_REPOSITORY requires RELEASE_ARTIFACTS to include charts")
	}
	if !enabled {
		return nil
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
	fmt.Fprintf(a.stdout, "[INFO] "+format+"\n", args...)
}

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
	"runtime"
	"strings"

	"github.com/charmbracelet/fang"
	"github.com/spf13/cobra"
)

var allowedConfigKeys = map[string]bool{
	"RELEASE_FORGE":        true,
	"RELEASE_PROJECT":      true,
	"RELEASE_OWNER":        true,
	"RELEASE_REPO":         true,
	"RELEASE_API_URL":      true,
	"RELEASE_NOTES_SOURCE": true,
	"RELEASE_NOTES_MODE":   true,
	"RELEASE_BODY_MODE":    true,
	"GORELEASER_CONFIG":    true,
	"GORELEASER_BIN":       true,
	"RELEASE_REQUIRE_GO":   true,
	"RELEASE_TOKEN_FILE":   true,
}

type forgeKind string

type appFactory func() (*app, error)

var releaseToolsVersion = "dev"

const (
	forgeGitea  forgeKind = "gitea"
	forgeGitHub forgeKind = "github"
	forgeGitLab forgeKind = "gitlab"
)

type app struct {
	toolkitRoot string
	repoRoot    string
	tmpDir      string
	configFile  string
	env         map[string]string
	stdout      io.Writer
	stderr      io.Writer
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
	env := environMap(environ)
	pwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	repoRoot := envValue(env, "RELEASE_REPO_ROOT", pwd)
	tmpDir := envValue(env, "RELEASE_TMP_DIR", filepath.Join(repoRoot, ".tmp"))
	configFile := envValue(env, "RELEASE_CONFIG_FILE", filepath.Join(repoRoot, ".release-tools.env"))
	toolkitRoot := resolveToolkitRoot()

	a := &app{
		toolkitRoot: toolkitRoot,
		repoRoot:    repoRoot,
		tmpDir:      tmpDir,
		configFile:  configFile,
		env:         env,
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
	env := make(map[string]string, len(environ))
	for _, entry := range environ {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			env[key] = value
		}
	}
	return env
}

func envValue(env map[string]string, key, fallback string) string {
	if value, ok := env[key]; ok && value != "" {
		return value
	}
	return fallback
}

func (a *app) loadConfigFile() error {
	content, err := os.ReadFile(a.configFile)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}

	for _, raw := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			return fmt.Errorf("invalid release config line in %s: %s", a.configFile, line)
		}
		if !allowedConfigKeys[key] {
			return fmt.Errorf("unsupported release config key in %s: %s", a.configFile, key)
		}
		if _, exists := a.env[key]; exists {
			continue
		}
		a.env[key] = stripSimpleQuotes(value)
	}
	return nil
}

func stripSimpleQuotes(value string) string {
	value = strings.TrimSuffix(value, `"`)
	value = strings.TrimPrefix(value, `"`)
	value = strings.TrimSuffix(value, `'`)
	value = strings.TrimPrefix(value, `'`)
	return value
}

func (a *app) run(args []string) error {
	cmd := newRootCommand(func() (*app, error) { return a, nil }, a.stdout, a.stderr)
	cmd.SetArgs(args)
	return cmd.Execute()
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
			return a.runGoreleaser("check")
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
			return a.runGoreleaser("release", "--snapshot", "--skip=publish", "--clean")
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

func (a *app) doctor() error {
	if err := a.requireReleaseVars(); err != nil {
		return err
	}
	config := a.goreleaserConfig()
	notesSource := a.releaseNotesSource()
	notesMode := a.releaseNotesMode()
	bodyMode := a.releaseBodyMode()
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
	case "news-md":
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
	goreleaserVersion := resolveGoreleaserVersion(goreleaserBin)

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
	a.log("Release notes mode: %s", notesMode)
	a.log("Release body mode: %s", bodyMode)
	a.log("release-tools configuration looks valid")
	return nil
}

func resolveGoreleaserVersion(goreleaserBin string) string {
	output, err := exec.Command(goreleaserBin, "--version").CombinedOutput()
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
		if _, err := exec.LookPath("go"); err != nil {
			return errors.New("go is required")
		}
	}
	_, err := a.resolveGoreleaserBin()
	return err
}

func (a *app) resolveGoreleaserBin() (string, error) {
	if bin := a.env["GORELEASER_BIN"]; bin != "" {
		if isExecutable(bin) {
			return bin, nil
		}
		return "", fmt.Errorf("GORELEASER_BIN is not executable: %s", bin)
	}
	if bin, err := exec.LookPath("goreleaser"); err == nil {
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
		if isExecutable(candidate) {
			return candidate, nil
		}
	}
	return "", errors.New("goreleaser not found. Install it and ensure it is available in PATH or GORELEASER_BIN")
}

func isExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func (a *app) runGoreleaser(args ...string) error {
	goreleaserBin, err := a.resolveGoreleaserBin()
	if err != nil {
		return err
	}
	cmdArgs := append([]string{"--config", a.goreleaserConfig()}, args...)
	cmd := exec.Command(goreleaserBin, cmdArgs...)
	cmd.Dir = a.repoRoot
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr
	cmd.Env = a.environ()
	if token, ok := a.resolveOptionalToken(); ok {
		cmd.Env = append(cmd.Env, a.goreleaserTokenEnv()+"="+token)
	}
	return cmd.Run()
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
	notesFile, err := a.generateNotes(tag)
	if err != nil {
		return err
	}
	if err := a.runGoreleaserWithToken(token, "release", "--clean", "--release-notes", notesFile); err != nil {
		return err
	}
	return a.updateReleaseBody(tag, notesFile, token)
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
	notesFile, err := cloneApp.generateNotes(tag)
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
	a.log("Published %s", tag)
	return nil
}

func (a *app) cloneTag(tag, cloneDir string) error {
	if err := runAttached(a.stdout, a.stderr, "", "git", "clone", "--quiet", "file://"+filepath.Join(a.repoRoot, ".git"), cloneDir); err != nil {
		return err
	}
	return runAttached(a.stdout, a.stderr, cloneDir, "git", "checkout", "--quiet", "--detach", "refs/tags/"+tag)
}

func (a *app) runGoreleaserWithToken(token string, args ...string) error {
	goreleaserBin, err := a.resolveGoreleaserBin()
	if err != nil {
		return err
	}
	cmdArgs := append([]string{"--config", a.goreleaserConfig()}, args...)
	cmd := exec.Command(goreleaserBin, cmdArgs...)
	cmd.Dir = a.repoRoot
	cmd.Stdout = a.stdout
	cmd.Stderr = a.stderr
	cmd.Env = append(a.environ(), a.goreleaserTokenEnv()+"="+token)
	return cmd.Run()
}

func (a *app) environ() []string {
	merged := environMap(os.Environ())
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
	cmd := exec.Command("git", "-C", a.repoRoot, "rev-parse", "-q", "--verify", "refs/tags/"+tag)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
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
	case "news-md":
		newsFile := filepath.Join(a.repoRoot, notesSource)
		content, err := os.ReadFile(newsFile)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return "", fmt.Errorf("release notes source not found: %s", newsFile)
			}
			return "", err
		}
		section := extractNewsSection(string(content), tag)
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
	cmd := exec.Command("git", "-C", a.repoRoot, "describe", "--tags", "--exact-match")
	output, err := cmd.Output()
	if err == nil {
		if tag := strings.TrimSpace(string(output)); tag != "" {
			return tag, nil
		}
	}
	return "", errors.New("VERSION is required when the current commit is not an exact tag")
}

func extractNewsSection(content, tag string) string {
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

func (a *app) log(format string, args ...any) {
	fmt.Fprintf(a.stdout, "[INFO] "+format+"\n", args...)
}

func runAttached(stdout, stderr io.Writer, dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	return cmd.Run()
}

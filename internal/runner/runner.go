package runner

import (
	"io"
	"os"
	"os/exec"
	"runtime"
)

type Command struct {
	Dir    string
	Name   string
	Args   []string
	Env    []string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

type Runner interface {
	Run(Command) error
	Output(Command) ([]byte, error)
	CombinedOutput(Command) ([]byte, error)
	LookPath(string) (string, error)
	IsExecutable(string) bool
}

type OSRunner struct{}

type AttachedRunner struct {
	Stdout io.Writer
	Stderr io.Writer
}

func (r OSRunner) Run(command Command) error {
	return command.exec().Run()
}

func (r OSRunner) Output(command Command) ([]byte, error) {
	return command.exec().Output()
}

func (r OSRunner) CombinedOutput(command Command) ([]byte, error) {
	return command.exec().CombinedOutput()
}

func (r OSRunner) LookPath(file string) (string, error) {
	return exec.LookPath(file)
}

func (r OSRunner) IsExecutable(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	if runtime.GOOS == "windows" {
		return true
	}
	return info.Mode()&0111 != 0
}

func (r AttachedRunner) Command(dir, name string, args ...string) Command {
	return Command{Dir: dir, Name: name, Args: args, Stdout: r.Stdout, Stderr: r.Stderr}
}

func (r AttachedRunner) Run(dir, name string, args ...string) error {
	return OSRunner{}.Run(r.Command(dir, name, args...))
}

func NewCommand(dir, name string, args ...string) Command {
	return Command{Dir: dir, Name: name, Args: args}
}

func (c Command) exec() *exec.Cmd {
	cmd := exec.Command(c.Name, c.Args...)
	cmd.Dir = c.Dir
	cmd.Env = c.Env
	cmd.Stdin = c.Stdin
	cmd.Stdout = c.Stdout
	cmd.Stderr = c.Stderr
	return cmd
}

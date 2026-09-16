package worker

import (
	"bytes"
	"context"
	"os/exec"
	"runtime"
	"syscall"
)

type CommandResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}
type Commander interface {
	Run(context.Context, string, ...string) CommandResult
}
type ExecCommander struct{}

func (ExecCommander) Run(ctx context.Context, dir string, args ...string) CommandResult {
	if len(args) == 0 {
		return CommandResult{}
	}
	c := exec.CommandContext(ctx, args[0], args[1:]...)
	c.Dir = dir
	if runtime.GOOS == "linux" {
		c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
		c.Cancel = func() error { return syscall.Kill(-c.Process.Pid, syscall.SIGKILL) }
	}
	var out, er bytes.Buffer
	c.Stdout = &out
	c.Stderr = &er
	err := c.Run()
	code := 0
	if err != nil {
		code = -1
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		}
	}
	return CommandResult{Stdout: out.String(), Stderr: er.String(), ExitCode: code, Err: err}
}

package shell

import (
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
)

// Command like exec.Cmd, but with support:
// - Piped execution without shell interpreter
// - io.Closer interface
// - Wait from multiple places
// - Done channel
type Command struct {
	cmds   []*exec.Cmd
	ctx    context.Context
	cancel context.CancelFunc
	err    error

	Args      []string
	Stderr    io.Writer
	Stdout    io.Writer
	Cancel    func() error
	WaitDelay time.Duration
}

func NewCommand(s string) *Command {
	ctx, cancel := context.WithCancel(context.Background())

	parts := strings.Split(s, "|")
	cmds := make([]*exec.Cmd, len(parts))

	for i, part := range parts {
		args := QuoteSplit(strings.TrimSpace(part))
		cmd := exec.CommandContext(ctx, args[0], args[1:]...)
		cmd.SysProcAttr = procAttr
		cmds[i] = cmd
	}

	for i := 0; i < len(cmds)-1; i++ {
		pipeReader, pipeWriter := io.Pipe()
		cmds[i].Stdout = pipeWriter
		cmds[i+1].Stdin = pipeReader
	}

	return &Command{
		cmds:   cmds,
		ctx:    ctx,
		cancel: cancel,
		Args:   cmds[0].Args,
	}
}

func (c *Command) Start() error {
	if c.Stderr != nil {
		for _, cmd := range c.cmds {
			cmd.Stderr = c.Stderr
		}
	}
	if c.Stdout != nil {
		c.cmds[len(c.cmds)-1].Stdout = c.Stdout
	}
	if c.Cancel != nil {
		for _, cmd := range c.cmds {
			cmd.Cancel = c.Cancel
		}
	}
	if c.WaitDelay != 0 {
		for _, cmd := range c.cmds {
			cmd.WaitDelay = c.WaitDelay
		}
	}

	for _, cmd := range c.cmds {
		if err := cmd.Start(); err != nil {
			c.Close()
			return err
		}
	}

	go func() {
		var firstErr error
		for _, cmd := range c.cmds {
			if err := cmd.Wait(); err != nil && firstErr == nil {
				firstErr = err
			}
			if w, ok := cmd.Stdout.(*io.PipeWriter); ok {
				_ = w.Close()
			}
		}
		c.err = firstErr
		c.cancel()
	}()

	return nil
}

func (c *Command) StdinPipe() (io.WriteCloser, error) {
	firstCmd := c.cmds[0]
	return firstCmd.StdinPipe()
}

func (c *Command) StdoutPipe() (io.ReadCloser, error) {
	lastCmd := c.cmds[len(c.cmds)-1]
	return lastCmd.StdoutPipe()
}

func (c *Command) Wait() error {
	<-c.ctx.Done()
	return c.err
}

func (c *Command) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	return c.Wait()
}

func (c *Command) Done() <-chan struct{} {
	return c.ctx.Done()
}

func (c *Command) Signal(sig os.Signal) error {
	var firstErr error
	for _, cmd := range c.cmds {
		if cmd.Process != nil {
			if err := cmd.Process.Signal(sig); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (c *Command) Close() error {
	c.cancel()
	return nil
}

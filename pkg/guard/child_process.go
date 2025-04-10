package guard

import (
	"fmt"
	"io"
	"os"
	"os/exec"
)

// ChildProcess represents a child process that handles MCP requests.
type ChildProcess struct {
	cmd    *exec.Cmd
	Stdin  io.WriteCloser
	Stdout io.ReadCloser
	Stderr io.ReadCloser
}

// NewChildProcess creates a new child process.
func NewChildProcess(cmdArgs []string) *ChildProcess {
	return &ChildProcess{
		cmd: exec.Command(cmdArgs[0], cmdArgs[1:]...), // nolint:gosec
	}
}

// Start starts the child process.
func (c *ChildProcess) Start() error {
	var err error

	// Set up pipes for stdin, stdout, and stderr
	c.Stdin, err = c.cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("error creating stdin pipe: %w", err)
	}

	c.Stdout, err = c.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("error creating stdout pipe: %w", err)
	}

	c.Stderr, err = c.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("error creating stderr pipe: %w", err)
	}

	// Start a goroutine to pipe stderr to os.Stderr
	go func() {
		io.Copy(os.Stderr, c.Stderr) // nolint
	}()

	// Start the command
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("error starting command: %w", err)
	}

	return nil
}

// Close closes the child process.
func (c *ChildProcess) Close() error {
	if c.cmd.Process != nil {
		return c.cmd.Process.Kill()
	}
	return nil
}

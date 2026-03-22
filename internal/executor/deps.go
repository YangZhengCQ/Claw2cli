package executor

import (
	"context"
	"net"
	"os"
	"os/exec"
)

// Package-level function pointers for dependency injection in tests.
var (
	execCommand    = exec.Command
	execCommandCtx = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		return exec.CommandContext(ctx, name, args...)
	}
	osExecutable  = os.Executable
	osFindProcess = os.FindProcess
	netDial       = func(network, address string) (net.Conn, error) {
		return net.Dial(network, address)
	}
)

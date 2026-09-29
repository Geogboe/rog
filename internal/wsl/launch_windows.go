//go:build windows

package wsl

import (
	"context"
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

func hiddenCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, executable(), args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

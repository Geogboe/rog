//go:build !windows

package wsl

import (
	"context"
	"os/exec"
)

func hiddenCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	return exec.CommandContext(ctx, executable(), args...)
}

package git

import (
	"context"
	"os/exec"
)

func gitCommand(args ...string) *exec.Cmd {
	cmd := exec.Command(gitExecutable(), args...)
	hideCommandWindow(cmd)
	return cmd
}

func gitCommandContext(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, gitExecutable(), args...)
	hideCommandWindow(cmd)
	return cmd
}

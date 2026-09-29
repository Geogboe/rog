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

// CommandContext creates a Git command with the platform's window handling.
func CommandContext(ctx context.Context, args ...string) *exec.Cmd {
	return gitCommandContext(ctx, args...)
}

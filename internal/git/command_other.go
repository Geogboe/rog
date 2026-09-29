//go:build !windows

package git

import "os/exec"

func hideCommandWindow(_ *exec.Cmd) {}

//go:build !windows

package setup

func LocalSearchRoots() []SearchRoot { return []SearchRoot{{Name: "linux", Path: "/"}} }

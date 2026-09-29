package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/Geogboe/rog/cmd"
	"github.com/Geogboe/rog/internal/workerproto"
	"github.com/Geogboe/rog/internal/workerrun"
)

func main() {
	if len(os.Args) == 2 && os.Args[1] == "__windows-worker" {
		if err := runWindowsWorker(); err != nil {
			fmt.Fprintln(os.Stderr, "rog Windows worker:", err)
			os.Exit(1)
		}
		return
	}
	if err := cmd.Execute(); err != nil {
		os.Exit(cmd.ExitCode(err))
	}
}

func runWindowsWorker() error {
	if runtime.GOOS != "windows" {
		return fmt.Errorf("Windows worker requires Windows")
	}
	if _, err := fmt.Fprintf(os.Stdout, "ROG_WINDOWS_WORKER_READY %d\n", workerproto.Version); err != nil {
		return err
	}
	var req workerproto.Request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		return fmt.Errorf("decode request: %w", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return workerrun.Run(ctx, req, os.Stdout)
}

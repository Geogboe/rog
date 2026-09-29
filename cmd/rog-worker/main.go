// rog-worker serves Linux filesystem requests from the Windows CLI.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/Geogboe/rog/internal/workerproto"
	"github.com/Geogboe/rog/internal/workerrun"
)

func main() {
	var req workerproto.Request
	if err := json.NewDecoder(os.Stdin).Decode(&req); err != nil {
		fail(err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := workerrun.Run(ctx, req, os.Stdout); err != nil {
		fail(err)
	}
}

func fail(err error) { fmt.Fprintln(os.Stderr, "rog WSL worker:", err); os.Exit(1) }

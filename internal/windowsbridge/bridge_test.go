package windowsbridge

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/workerproto"
)

func TestMapPathNestedAndDuplicateNames(t *testing.T) {
	if got := windowsRelative("team/nested/repo"); got != `team\nested\repo` {
		t.Fatalf("native relative path = %q", got)
	}
	roots := []config.Root{{Name: "projects", Path: `C:\Users\me\dev`}, {Name: "github", Path: `C:\Users\me\dev\github`}}
	paths := map[string]string{"projects": "/mnt/c/Users/me/dev", "github": "/mnt/c/Users/me/dev/github"}
	got, name, rel, ok := mapPath(`c:\users\ME\dev\github\same name`, roots, paths)
	if !ok || name != "github" || rel != "same name" || got != "/mnt/c/Users/me/dev/github/same name" {
		t.Fatalf("mapPath = %q, %q, %q, %v", got, name, rel, ok)
	}
	if _, _, _, ok := mapPath(`C:\Users\me\developer\repo`, roots, paths); ok {
		t.Fatal("sibling path matched")
	}
}

func TestWorkerVersionAndCancellation(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture runs on Linux")
	}
	fixture := filepath.Join(t.TempDir(), "worker")
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\nprintf 'ROG_WINDOWS_WORKER_READY 999\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	bridge := Bridge{Executable: fixture}
	if err := bridge.invoke(context.Background(), workerproto.Request{Version: workerproto.Version}, func(workerproto.Event) error { return nil }); err == nil {
		t.Fatal("accepted incompatible worker")
	}
	if err := os.WriteFile(fixture, []byte("#!/bin/sh\nprintf 'ROG_WINDOWS_WORKER_READY 2\\n'\ncat >/dev/null\nsleep 30\n"), 0700); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	start := time.Now()
	if err := bridge.invoke(ctx, workerproto.Request{Version: workerproto.Version}, func(workerproto.Event) error { return nil }); err == nil {
		t.Fatal("cancelled worker succeeded")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatal("worker did not stop promptly")
	}
}

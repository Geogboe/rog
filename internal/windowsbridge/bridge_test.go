//go:build !windows

package windowsbridge

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/Geogboe/rog/internal/config"
)

func TestDiscoverAcceptsTypedWindowsWorkerResult(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "rog.exe")
	script := `#!/bin/sh
printf 'ROG_WINDOWS_WORKER_READY 3\n'
request=$(cat)
case "$request" in *'"windows":true'*) ;; *) echo 'missing Windows root flag' >&2; exit 2 ;; esac
printf '%s\n' '{"type":"discovery_progress","discovery_progress":{"root":"dev","name":"repo","completed":1,"total":1}}'
printf '%s\n' '{"type":"discovery_result","result":{"version":3,"discovery":{"candidates":[{"path":"C:\\Users\\me\\projects\\repo","root":"dev","windows":true,"valid":true}]}}}'
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(executable, 0700); err != nil {
		t.Fatal(err)
	}
	var progress int
	result, err := (Bridge{Executable: executable}).Discover(context.Background(), []config.Root{{Name: "dev", Path: `C:\Users\me\projects`, Windows: true}}, nil, func(root, name string, done, total int) {
		if root != "dev" || name != "repo" || done != 1 || total != 1 {
			t.Errorf("unexpected progress: %s %s %d/%d", root, name, done, total)
		}
		progress++
	})
	if err != nil {
		t.Fatal(err)
	}
	if progress != 1 || len(result.Candidates) != 1 || !result.Candidates[0].Windows {
		t.Fatalf("unexpected discovery result: progress=%d result=%+v", progress, result)
	}
	if result.Candidates[0].Path != `C:\Users\me\projects\repo` {
		t.Fatalf("Windows path changed: %q", result.Candidates[0].Path)
	}
}

package wslbridge

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
)

func TestInterruptedInstallPreservesExistingWorker(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("requires POSIX shell")
	}
	if _, err := exec.LookPath("sha256sum"); err != nil {
		t.Skip("requires sha256sum")
	}
	root := t.TempDir()
	cache := filepath.Join(root, "rog", "workers", "build")
	if err := os.MkdirAll(cache, 0700); err != nil {
		t.Fatal(err)
	}
	worker := filepath.Join(cache, "rog-worker")
	if err := os.WriteFile(worker, []byte("previous worker"), 0700); err != nil {
		t.Fatal(err)
	}
	expected := sha256.Sum256([]byte("complete worker"))
	cmd := exec.Command("sh", "-c", installScript, "rog-worker", "build", hex.EncodeToString(expected[:]))
	cmd.Env = append(os.Environ(), "XDG_CACHE_HOME="+root)
	cmd.Stdin = bytes.NewBufferString("interrupted")
	if err := cmd.Run(); err == nil {
		t.Fatal("truncated install unexpectedly succeeded")
	}
	got, err := os.ReadFile(worker)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "previous worker" {
		t.Fatalf("worker replaced with %q", got)
	}
	temps, err := filepath.Glob(filepath.Join(cache, ".worker.*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(temps) != 0 {
		t.Fatalf("temporary files remain: %v", temps)
	}
}

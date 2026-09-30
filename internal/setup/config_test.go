package setup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Geogboe/rog/internal/config"
)

func TestPreviewConfigPreservesUnmanagedKeys(t *testing.T) {
	original := []byte("roots:\n  - name: old\n    path: /old\n    max_depth: 2\neditor: vi\ncustom_future_setting:\n  enabled: true\n")
	next := &config.Config{Roots: []config.Root{{Name: "projects", Path: "/home/me/projects", MaxDepth: 4}}, GlobalExcludes: []string{".git"}}
	preview, err := PreviewConfig(original, next)
	if err != nil {
		t.Fatal(err)
	}
	for _, needle := range []string{"editor: vi", "custom_future_setting:", "enabled: true", "name: projects", "max_depth: 4"} {
		if !strings.Contains(string(preview), needle) {
			t.Fatalf("preview lost %q: %s", needle, preview)
		}
	}
}

func TestApplyConfigHistoryAndRollback(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	initial := []byte("roots: []\neditor: vi\n")
	if err := os.WriteFile(path, initial, 0644); err != nil {
		t.Fatal(err)
	}
	first := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	backup, err := ApplyConfig(path, []byte("roots: []\neditor: code\n"), first)
	if err != nil {
		t.Fatal(err)
	}
	if backup == "" {
		t.Fatal("expected backup")
	}
	info, err := os.Stat(backup)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("backup mode %o, want 0600", info.Mode().Perm())
	}
	if _, err := RestoreConfig(path, filepath.Base(backup), first.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != string(initial) {
		t.Fatalf("rollback did not restore config: %s", data)
	}
	history, err := History(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) < 1 {
		t.Fatal("restore should retain a revision")
	}
}

func TestApplyConfigRetainsFiveRevisions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	for i := 0; i < 7; i++ {
		at := time.Date(2026, 9, 30, 10, 0, i, 0, time.UTC)
		if _, err := ApplyConfig(path, []byte("roots: []\n"), at); err != nil {
			t.Fatal(err)
		}
	}
	history, err := History(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != 5 {
		t.Fatalf("kept %d revisions, want 5", len(history))
	}
}

func TestValidateSetupConfigRejectsDuplicateAndRelativeRoots(t *testing.T) {
	valid := &config.Config{Roots: []config.Root{{Name: "projects", Path: "/home/me/projects", MaxDepth: 3}}}
	if err := ValidateSetupConfig(valid); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}
	duplicate := &config.Config{Roots: []config.Root{
		{Name: "Projects", Path: "/home/me/projects", MaxDepth: 3},
		{Name: "projects", Path: "/home/me/other", MaxDepth: 2},
	}}
	if err := ValidateSetupConfig(duplicate); err == nil || !strings.Contains(err.Error(), "repeated") {
		t.Fatalf("duplicate root error=%v", err)
	}
	relative := &config.Config{Roots: []config.Root{{Name: "projects", Path: "projects", MaxDepth: 3}}}
	if err := ValidateSetupConfig(relative); err == nil || !strings.Contains(err.Error(), "absolute") {
		t.Fatalf("relative root error=%v", err)
	}
	windows := &config.Config{Roots: []config.Root{{Name: "windows", Path: `C:\Users\me\dev`, Windows: true, MaxDepth: 3}}}
	if err := ValidateSetupConfig(windows); err != nil {
		t.Fatalf("valid Windows root rejected on non-Windows host: %v", err)
	}
}

func TestApplyConfigRejectsInvalidTypedYAMLWithoutChangingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yml")
	original := []byte("roots: []\neditor: vi\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyConfig(path, []byte("roots: not-a-list\n"), time.Now()); err == nil {
		t.Fatal("invalid typed YAML was accepted")
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(original) {
		t.Fatalf("invalid config changed original: %s", after)
	}
}

func TestApplyConfigDoesNotOverwriteBackupRevision(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yml")
	original := []byte("roots: []\neditor: vi\n")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	history := filepath.Join(dir, "setup-history")
	if err := os.MkdirAll(history, 0700); err != nil {
		t.Fatal(err)
	}
	revision := filepath.Join(history, now.Local().Format("20060102-150405.000000000")+".yml")
	priorRevision := []byte("previous protected revision\n")
	if err := os.WriteFile(revision, priorRevision, 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := ApplyConfig(path, []byte("roots: []\neditor: code\n"), now); err == nil {
		t.Fatal("colliding revision timestamp was accepted")
	}
	configAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	revisionAfter, revErr := os.ReadFile(revision)
	if revErr != nil || string(configAfter) != string(original) || string(revisionAfter) != string(priorRevision) {
		t.Fatalf("collision changed protected state: config=%q revision=%q err=%v", configAfter, revisionAfter, revErr)
	}
}

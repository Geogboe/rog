package cmd

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestReportDateOnlyUntilIncludesDSTDay(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skip(err)
	}
	old := time.Local
	time.Local = loc
	defer func() { time.Local = old }()
	start, err := parseReportDate("2026-03-08", false)
	if err != nil {
		t.Fatal(err)
	}
	end, err := parseReportDate("2026-03-08", true)
	if err != nil {
		t.Fatal(err)
	}
	if end.Sub(start) != 23*time.Hour {
		t.Fatalf("DST day should cover 23 hours, got %s", end.Sub(start))
	}
}

func TestReportOpenDestination(t *testing.T) {
	oldOpen, oldOutput, oldFile := reportOpen, reportOutput, reportFile
	t.Cleanup(func() { reportOpen, reportOutput, reportFile = oldOpen, oldOutput, oldFile })
	reportOpen, reportOutput, reportFile = true, "", ""
	dir := t.TempDir()
	mode, target, err := resolveReportDestination([]string{dir}, time.Date(2026, 9, 28, 12, 0, 0, 0, time.UTC))
	if err != nil || mode != "html" || filepath.Dir(target) != dir || !strings.HasPrefix(filepath.Base(target), "rog-report-2026-09-28-") || filepath.Ext(target) != ".html" {
		t.Fatalf("directory target: mode=%q target=%q err=%v", mode, target, err)
	}
	mode, target, err = resolveReportDestination(nil, time.Now())
	if err != nil || mode != "html" || filepath.Dir(target) != os.TempDir() {
		t.Fatalf("temporary target: mode=%q target=%q err=%v", mode, target, err)
	}
	reportFile = filepath.Join(dir, "named.html")
	mode, target, err = resolveReportDestination(nil, time.Now())
	if err != nil || mode != "html" || target != reportFile {
		t.Fatalf("explicit file: mode=%q target=%q err=%v", mode, target, err)
	}
	if _, _, err := resolveReportDestination([]string{dir}, time.Now()); err == nil {
		t.Fatal("path and --file should conflict")
	}
	reportFile = ""
	reportOutput = "json"
	if _, _, err := resolveReportDestination([]string{dir}, time.Now()); err == nil {
		t.Fatal("--open with JSON should fail")
	}
	reportOpen, reportOutput = false, ""
	if _, _, err := resolveReportDestination([]string{dir}, time.Now()); err == nil {
		t.Fatal("path without --open should fail")
	}
}

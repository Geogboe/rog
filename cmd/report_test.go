package cmd

import (
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

// Package workerproto defines the versioned local pipe protocol used by a
// Windows rog process and its short-lived Linux WSL scan worker.
package workerproto

import (
	"github.com/Geogboe/rog/internal/config"
	"github.com/Geogboe/rog/internal/index"
	"github.com/Geogboe/rog/internal/metadata"
)

const Version = 1

type Request struct {
	Version    int                  `json:"version"`
	Config     config.Config        `json:"config"`
	Existing   []*index.Repo        `json:"existing"`
	GlobalMeta *metadata.GlobalMeta `json:"global_meta"`
	Full       bool                 `json:"full"`
	Remote     bool                 `json:"remote"`
	DryRun     bool                 `json:"dry_run"`
}

type Response struct {
	Version           int           `json:"version"`
	Found             []string      `json:"found"`
	Candidates        int           `json:"candidates"`
	CompleteRoots     []string      `json:"complete_roots"`
	Warnings          []string      `json:"warnings,omitempty"`
	Repos             []*index.Repo `json:"repos"`
	Reused            int           `json:"reused"`
	Refreshed         int           `json:"refreshed"`
	StatusUnavailable int           `json:"status_unavailable"`
	StatusTimeouts    int           `json:"status_timeouts"`
	DiscoveryNanos    int64         `json:"discovery_nanos"`
	GitNanos          int64         `json:"git_nanos"`
	MetadataNanos     int64         `json:"metadata_nanos"`
	Error             string        `json:"error,omitempty"`
}

// Progress is a periodic snapshot; result is sent exactly once after success.
type Progress struct {
	Root      string `json:"root"`
	Repo      string `json:"repo"`
	Found     int    `json:"found"`
	Refreshed int    `json:"refreshed"`
	Reused    int    `json:"reused"`
}

type Event struct {
	Type     string    `json:"type"`
	Progress *Progress `json:"progress,omitempty"`
	Result   *Response `json:"result,omitempty"`
}

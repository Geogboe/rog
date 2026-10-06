package workerrun

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/Geogboe/rog/internal/setup"
	"github.com/Geogboe/rog/internal/workerproto"
)

func TestWorkerDiscoveryRunsWithoutGitAndLeavesIndexAlone(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "dev", "repo")
	if err := os.MkdirAll(repo, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("not validated"), 0600); err != nil {
		t.Fatal(err)
	}
	data := t.TempDir()
	t.Setenv("ROG_DATA", data)
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	req := workerproto.Request{Version: workerproto.Version, Operation: "discover_locations", DiscoveryRoots: []setup.SearchRoot{{Name: "dev", Path: root}}}
	if err := Run(context.Background(), req, &out); err != nil {
		t.Fatal(err)
	}
	decoder := json.NewDecoder(&out)
	found := false
	for {
		var event workerproto.Event
		if err := decoder.Decode(&event); err == io.EOF {
			break
		} else if err != nil {
			t.Fatal(err)
		}
		if event.Type == "discovery_result" {
			if event.Result == nil || event.Result.Discovery == nil || len(event.Result.Discovery.Candidates) != 1 {
				t.Fatalf("bad result: %+v", event)
			}
			c := event.Result.Discovery.Candidates[0]
			if c.Valid || c.Email != "" || len(c.AuthorEmails) > 0 {
				t.Fatal("worker returned Git metadata")
			}
			found = true
		}
	}
	if !found {
		t.Fatal("missing discovery result")
	}
	files, err := os.ReadDir(data)
	if err != nil || len(files) != 0 {
		t.Fatal("discovery wrote index data")
	}
}

func TestLegacyGitDiscoveryOperationRejected(t *testing.T) {
	var out bytes.Buffer
	if err := Run(context.Background(), workerproto.Request{Version: workerproto.Version, Operation: "discover"}, &out); err == nil {
		t.Fatal("legacy Git discovery should not execute")
	}
}

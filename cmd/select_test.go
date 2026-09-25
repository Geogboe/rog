package cmd

import (
	"github.com/Geogboe/rog/internal/index"
	"testing"
)

func TestSelectUsesDistinctPathsForDuplicateNames(t *testing.T) {
	first := &index.Repo{Name: "service", AbsPath: "/one/service", Root: "one"}
	second := &index.Repo{Name: "service", AbsPath: "/two/service", Root: "two"}
	items, byID := selectItems([]*index.Repo{first, second})
	if len(items) != 2 || items[0].ID == items[1].ID {
		t.Fatalf("duplicate names collapsed: %+v", items)
	}
	if items[0].ID != first.AbsPath || items[1].ID != second.AbsPath {
		t.Fatalf("picker IDs do not use stable paths: %+v", items)
	}
	if byID[items[0].ID] != first || byID[items[1].ID] != second {
		t.Fatal("selected path resolves to wrong repository")
	}
}

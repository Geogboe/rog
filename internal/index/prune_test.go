package index

import "testing"

func TestRemoveStaleInRootsPreservesSkippedRoot(t *testing.T) {
	idx := New()
	idx.Upsert(&Repo{Root: "good", AbsPath: "/good/old"})
	idx.Upsert(&Repo{Root: "skipped", AbsPath: "/skipped/old"})
	removed := idx.RemoveStaleInRoots(map[string]struct{}{}, map[string]struct{}{"good": {}})
	if removed != 1 || idx.Count() != 1 {
		t.Fatalf("removed=%d count=%d", removed, idx.Count())
	}
	if _, ok := idx.Get("/skipped/old"); !ok {
		t.Fatal("skipped root was pruned")
	}
}

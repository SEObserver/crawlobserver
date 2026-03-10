package interlinking

import (
	"testing"

	"github.com/SEObserver/crawlobserver/internal/storage"
)

func TestWithVirtualLinks(t *testing.T) {
	g := &storage.PageRankGraph{
		N:             3,
		OutLinks:      [][]uint32{{1}, {2}, {}},
		TotalOutLinks: []uint32{1, 1, 0},
		URLToID:       map[string]uint32{"a": 0, "b": 1, "c": 2},
		IDToURL:       []string{"a", "b", "c"},
	}

	gv := WithVirtualLinks(g, []storage.VirtualLink{{SourceURL: "c", TargetURL: "a"}})

	// Original should be unchanged
	if len(g.OutLinks[2]) != 0 {
		t.Error("original graph should not be mutated")
	}
	if g.TotalOutLinks[2] != 0 {
		t.Error("original totalOutLinks should not be mutated")
	}

	// New graph should have the extra edge
	if len(gv.OutLinks[2]) != 1 || gv.OutLinks[2][0] != 0 {
		t.Error("virtual link c→a not added")
	}
	if gv.TotalOutLinks[2] != 1 {
		t.Error("totalOutLinks[2] should be 1")
	}
}

func TestWithVirtualLinksIgnoresInvalid(t *testing.T) {
	g := &storage.PageRankGraph{
		N:             2,
		OutLinks:      [][]uint32{{}, {}},
		TotalOutLinks: []uint32{0, 0},
		URLToID:       map[string]uint32{"a": 0, "b": 1},
		IDToURL:       []string{"a", "b"},
	}

	gv := WithVirtualLinks(g, []storage.VirtualLink{
		{SourceURL: "a", TargetURL: "a"},       // self-link
		{SourceURL: "a", TargetURL: "unknown"}, // unknown target
	})

	if len(gv.OutLinks[0]) != 0 {
		t.Error("invalid virtual links should be ignored")
	}
}

func TestSimulationDiff(t *testing.T) {
	g := &storage.PageRankGraph{
		N:             3,
		OutLinks:      [][]uint32{{1}, {2}, {}},
		TotalOutLinks: []uint32{1, 1, 0},
		URLToID:       map[string]uint32{"a": 0, "b": 1, "c": 2},
		IDToURL:       []string{"a", "b", "c"},
	}

	before := storage.ComputePageRankIterations(g.N, g.OutLinks, g.TotalOutLinks)
	gv := WithVirtualLinks(g, []storage.VirtualLink{{SourceURL: "c", TargetURL: "a"}})
	after := storage.ComputePageRankIterations(gv.N, gv.OutLinks, gv.TotalOutLinks)

	totalBefore := before[0] + before[1] + before[2]
	totalAfter := after[0] + after[1] + after[2]

	if totalBefore < 100 || totalAfter < 100 {
		t.Errorf("unexpected totals: before=%.2f, after=%.2f", totalBefore, totalAfter)
	}

	t.Logf("Before: A=%.2f B=%.2f C=%.2f", before[0], before[1], before[2])
	t.Logf("After:  A=%.2f B=%.2f C=%.2f", after[0], after[1], after[2])
}

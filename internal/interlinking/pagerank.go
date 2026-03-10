package interlinking

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/SEObserver/crawlobserver/internal/applog"
	"github.com/SEObserver/crawlobserver/internal/storage"
)

// WithVirtualLinks returns a copy of the graph with additional edges injected.
func WithVirtualLinks(g *storage.PageRankGraph, links []storage.VirtualLink) *storage.PageRankGraph {
	newOutLinks := make([][]uint32, g.N)
	newTotalOutLinks := make([]uint32, g.N)
	for i := uint32(0); i < g.N; i++ {
		if len(g.OutLinks[i]) > 0 {
			newOutLinks[i] = make([]uint32, len(g.OutLinks[i]))
			copy(newOutLinks[i], g.OutLinks[i])
		}
		newTotalOutLinks[i] = g.TotalOutLinks[i]
	}

	for _, vl := range links {
		srcID, srcOK := g.URLToID[vl.SourceURL]
		tgtID, tgtOK := g.URLToID[vl.TargetURL]
		if !srcOK || !tgtOK || srcID == tgtID {
			continue
		}
		newOutLinks[srcID] = append(newOutLinks[srcID], tgtID)
		newTotalOutLinks[srcID]++
	}

	return &storage.PageRankGraph{
		N:             g.N,
		OutLinks:      newOutLinks,
		TotalOutLinks: newTotalOutLinks,
		URLToID:       g.URLToID,
		IDToURL:       g.IDToURL,
	}
}

// SimulationStore is the subset of storage needed for PageRank simulation.
type SimulationStore interface {
	LoadPageRankGraph(ctx context.Context, sessionID string) (*storage.PageRankGraph, error)
	InsertSimulation(ctx context.Context, sessionID string, simID string, virtualLinks []storage.VirtualLink, results []storage.SimulationResultRow, meta storage.SimulationMeta) error
}

// SimulateResult holds the outcome of a PageRank simulation.
type SimulateResult struct {
	SimulationID  string
	PagesImproved uint32
	PagesDeclined uint32
	AvgDiff       float64
	MaxDiff       float64
	Results       []storage.SimulationResultRow
}

// SimulatePageRank computes PageRank before/after adding virtual links.
func SimulatePageRank(ctx context.Context, store SimulationStore, sessionID, simID string, links []storage.VirtualLink) (*SimulateResult, error) {
	start := time.Now()

	applog.Info("interlinking", "Loading PageRank graph for simulation...")
	graph, err := store.LoadPageRankGraph(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("loading graph: %w", err)
	}
	applog.Infof("interlinking", "Graph loaded: %d nodes", graph.N)

	if graph.N == 0 {
		return nil, fmt.Errorf("empty graph")
	}

	// Compute before
	before := storage.ComputePageRankIterations(graph.N, graph.OutLinks, graph.TotalOutLinks)

	// Inject virtual links and compute after
	graphWith := WithVirtualLinks(graph, links)
	after := storage.ComputePageRankIterations(graphWith.N, graphWith.OutLinks, graphWith.TotalOutLinks)

	// Compute diffs
	var (
		improved, declined uint32
		totalDiff          float64
		maxDiff            float64
	)
	results := make([]storage.SimulationResultRow, graph.N)
	for i := uint32(0); i < graph.N; i++ {
		diff := after[i] - before[i]
		results[i] = storage.SimulationResultRow{
			URL:            graph.IDToURL[i],
			PageRankBefore: before[i],
			PageRankAfter:  after[i],
			PageRankDiff:   diff,
		}
		if diff > 0.001 {
			improved++
		} else if diff < -0.001 {
			declined++
		}
		totalDiff += diff
		absDiff := math.Abs(diff)
		if absDiff > maxDiff {
			maxDiff = absDiff
		}
	}

	avgDiff := 0.0
	if graph.N > 0 {
		avgDiff = totalDiff / float64(graph.N)
	}

	meta := storage.SimulationMeta{
		ID:                simID,
		CrawlSessionID:    sessionID,
		VirtualLinksCount: uint32(len(links)),
		PagesImproved:     improved,
		PagesDeclined:     declined,
		AvgDiff:           avgDiff,
		MaxDiff:           maxDiff,
		ComputedAt:        time.Now(),
	}

	if err := store.InsertSimulation(ctx, sessionID, simID, links, results, meta); err != nil {
		return nil, fmt.Errorf("storing simulation: %w", err)
	}

	applog.Infof("interlinking", "Simulation complete: %d improved, %d declined, avg diff %.4f in %s",
		improved, declined, avgDiff, time.Since(start))

	return &SimulateResult{
		SimulationID:  simID,
		PagesImproved: improved,
		PagesDeclined: declined,
		AvgDiff:       avgDiff,
		MaxDiff:       maxDiff,
		Results:       results,
	}, nil
}

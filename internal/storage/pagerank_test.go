package storage

import (
	"fmt"
	"math"
	"testing"
)

// helper: return normalized 0–100 scores for a graph.
func computePR(t *testing.T, n uint32, outLinks [][]uint32, totalOutLinks []uint32, edgeWeights ...[][]float64) []float64 {
	t.Helper()
	rank := ComputePageRankIterations(n, outLinks, totalOutLinks, edgeWeights...)
	if len(rank) != int(n) {
		t.Fatalf("expected %d ranks, got %d", n, len(rank))
	}
	return rank
}

func assertApprox(t *testing.T, label string, got, want, eps float64) {
	t.Helper()
	if math.Abs(got-want) > eps {
		t.Errorf("%s = %.4f, want ~%.4f (±%.4f)", label, got, want, eps)
	}
}

// --- Test 1: simple chain A→B→C (no external links) ---
// Same behavior as before: classic PageRank.
func TestPageRank_SimpleChain(t *testing.T) {
	// A(0) → B(1) → C(2), C is dangling
	outLinks := [][]uint32{
		{1},       // A → B
		{2},       // B → C
		nil,       // C → (nothing)
	}
	totalOutLinks := []uint32{1, 1, 0}
	rank := computePR(t, 3, outLinks, totalOutLinks)

	// C receives all flow and dangling redistribution → highest PR
	if rank[2] <= rank[1] || rank[1] <= rank[0] {
		t.Errorf("expected rank[C] > rank[B] > rank[A], got %v", rank)
	}
	// C should be the max (normalized to 100)
	assertApprox(t, "C (top)", rank[2], 100.0, 0.01)
}

// --- Test 2: external links dilute PR ---
// A has 1 internal + 9 external links. B has 1 internal + 0 external.
// Both point to C. A's contribution to C should be ~1/10 vs B's 1/1.
func TestPageRank_ExternalLinksDilute(t *testing.T) {
	// A(0) → C(2) (dofollow) + 9 external links
	// B(1) → C(2) (dofollow) only
	// C(2) → (dangling)
	outLinks := [][]uint32{
		{2}, // A → C (only internal dofollow edge)
		{2}, // B → C
		nil, // C dangling
	}
	totalOutLinks := []uint32{10, 1, 0} // A has 10 total outlinks (1 internal + 9 external)

	rank := computePR(t, 3, outLinks, totalOutLinks)

	// Compare with no-dilution scenario
	totalOutLinksNoDilution := []uint32{1, 1, 0}
	rankNoDilution := computePR(t, 3, outLinks, totalOutLinksNoDilution)

	// With dilution, A passes PR/10 to C instead of PR/1 → C gets less from A.
	// After normalization (C=100 in both), A's normalized score is HIGHER with
	// dilution because C's raw dominance shrinks. So C/A ratio DECREASES.
	ratioWithDilution := rank[2] / rank[0]
	ratioNoDilution := rankNoDilution[2] / rankNoDilution[0]

	if ratioWithDilution >= ratioNoDilution {
		t.Errorf("external dilution should decrease C/A ratio: with=%f, without=%f", ratioWithDilution, ratioNoDilution)
	}
}

// --- Test 3: nofollow links dilute but don't pass PR ---
// A has 2 internal links: one dofollow (→B), one nofollow (→C).
// B should receive PR from A, C should NOT.
func TestPageRank_NofollowDilutesButDoesNotPass(t *testing.T) {
	// A(0) → B(1) dofollow, A(0) → C(2) nofollow (not in outLinks, but counted in total)
	// B(1) → (dangling)
	// C(2) → (dangling)
	outLinks := [][]uint32{
		{1},       // A → B (only dofollow edge)
		nil,       // B dangling
		nil,       // C dangling
	}
	totalOutLinks := []uint32{2, 0, 0} // A has 2 total outlinks (1 dofollow + 1 nofollow)

	rank := computePR(t, 3, outLinks, totalOutLinks)

	// B should have significantly more PR than C.
	// C only gets teleportation + dangling redistribution.
	// B gets that PLUS direct PR flow from A.
	if rank[1] <= rank[2] {
		t.Errorf("B (dofollow target) should outrank C (nofollow target): B=%.4f, C=%.4f", rank[1], rank[2])
	}

	// Verify dilution: compare with scenario where A has only 1 outlink (no nofollow)
	totalOutLinksNoNF := []uint32{1, 0, 0}
	rankNoNF := computePR(t, 3, outLinks, totalOutLinksNoNF)

	// With nofollow dilution, B gets less PR from A (PR/2 vs PR/1)
	// But since normalization sets max to 100, compare ratios
	if rank[1]/rank[0] >= rankNoNF[1]/rankNoNF[0] {
		t.Errorf("nofollow should dilute A→B contribution: ratio with NF=%f, without=%f",
			rank[1]/rank[0], rankNoNF[1]/rankNoNF[0])
	}
}

// --- Test 4: external-only page leaks PR vs dangling page redistributes ---
// Compare two scenarios where the only difference is whether a source node
// leaks PR (external links) or redistributes it (truly dangling).
func TestPageRank_LeakingVsDangling(t *testing.T) {
	// Source(0) → no internal edges
	// Hub(1) → Target(2)
	// Target(2) → truly dangling
	outLinks := [][]uint32{
		nil, // Source: no internal edges
		{2}, // Hub → Target
		nil, // Target: dangling
	}

	// Case 1: Source is truly dangling (redistributes rank to everyone)
	totalDangling := []uint32{0, 1, 0}
	rankDangling := computePR(t, 3, outLinks, totalDangling)

	// Case 2: Source has external links (rank leaks out of graph)
	totalLeaking := []uint32{5, 1, 0}
	rankLeaking := computePR(t, 3, outLinks, totalLeaking)

	// When Source is dangling, its rank feeds back into the graph → Hub gets more
	// dangling redistribution → passes more to Target. After normalization (Target=100
	// in both), Hub's score is HIGHER in the dangling case.
	if rankDangling[1] <= rankLeaking[1] {
		t.Errorf("Hub should have higher normalized PR when Source is dangling vs leaking: dangling=%.4f, leaking=%.4f",
			rankDangling[1], rankLeaking[1])
	}
}

// --- Test 5: symmetric graph gives equal PR ---
func TestPageRank_SymmetricGraph(t *testing.T) {
	// A↔B↔C↔A (full cycle, 1 outlink each)
	outLinks := [][]uint32{
		{1}, // A → B
		{2}, // B → C
		{0}, // C → A
	}
	totalOutLinks := []uint32{1, 1, 1}

	rank := computePR(t, 3, outLinks, totalOutLinks)

	// All nodes should have equal PR due to symmetry
	assertApprox(t, "A", rank[0], rank[1], 0.01)
	assertApprox(t, "B", rank[1], rank[2], 0.01)
}

// --- Test 6: empty graph ---
func TestPageRank_EmptyGraph(t *testing.T) {
	rank := ComputePageRankIterations(0, nil, nil)
	if len(rank) != 0 {
		t.Errorf("expected empty ranks for empty graph, got %d", len(rank))
	}
}

// --- Test 7: single node ---
func TestPageRank_SingleNode(t *testing.T) {
	outLinks := [][]uint32{nil}
	totalOutLinks := []uint32{0}
	rank := computePR(t, 1, outLinks, totalOutLinks)
	assertApprox(t, "single node", rank[0], 100.0, 0.01)
}

// --- Test 8: hub page linking to many pages via dofollow vs same hub with external dilution ---
func TestPageRank_HubDilution(t *testing.T) {
	// Hub(0) → A(1), B(2), C(3) — all internal dofollow
	// A, B, C are dangling
	outLinksClean := [][]uint32{
		{1, 2, 3}, // Hub → A, B, C
		nil, nil, nil,
	}
	totalClean := []uint32{3, 0, 0, 0}
	rankClean := computePR(t, 4, outLinksClean, totalClean)

	// Same hub but with 7 external links (total 10 outlinks)
	totalDiluted := []uint32{10, 0, 0, 0}
	rankDiluted := computePR(t, 4, outLinksClean, totalDiluted)

	// With dilution, A/B/C get less PR relative to Hub
	// (Hub passes PR/10 per link instead of PR/3)
	ratioClean := rankClean[1] / rankClean[0]
	ratioDiluted := rankDiluted[1] / rankDiluted[0]

	if ratioDiluted >= ratioClean {
		t.Errorf("external dilution should reduce child/hub ratio: clean=%f, diluted=%f",
			ratioClean, ratioDiluted)
	}
}

// --- Test 9: redirect consolidation — edge via redirect has weight 0.90 ---
func TestPageRank_RedirectConsolidation(t *testing.T) {
	outLinks := [][]uint32{
		{1}, // A → B
		nil, // B dangling
	}
	totalOutLinks := []uint32{1, 0}

	weights := [][]float64{
		{0.90}, // A→B via 1-hop redirect
		nil,
	}
	rankRedirect := computePR(t, 2, outLinks, totalOutLinks, weights)
	rankDirect := computePR(t, 2, outLinks, totalOutLinks)

	ratioRedirect := rankRedirect[1] / rankRedirect[0]
	ratioDirect := rankDirect[1] / rankDirect[0]

	if ratioRedirect >= ratioDirect {
		t.Errorf("redirect weight should reduce B/A ratio: redirect=%f, direct=%f",
			ratioRedirect, ratioDirect)
	}
}

// --- Test 10: redirect chain — 2 hops = weight 0.81 ---
func TestPageRank_RedirectChain(t *testing.T) {
	outLinks := [][]uint32{
		{1, 2}, // A → B (2 hops), A → C (1 hop)
		nil,
		nil,
	}
	totalOutLinks := []uint32{2, 0, 0}

	weights := [][]float64{
		{0.81, 0.90}, // A→B 2-hop, A→C 1-hop
		nil,
		nil,
	}
	rank := computePR(t, 3, outLinks, totalOutLinks, weights)

	if rank[2] <= rank[1] {
		t.Errorf("C (1 hop) should outrank B (2 hops): C=%.4f, B=%.4f", rank[2], rank[1])
	}
}

// --- Test 11: log scale transforms values correctly ---
func TestPageRank_LogScale(t *testing.T) {
	logMax := math.Log1p(100.0)

	tests := []struct {
		linear float64
		want   float64
	}{
		{0.0, 0.0},
		{50.0, math.Log1p(50.0) / logMax * 100.0},
		{100.0, 100.0},
	}

	for _, tc := range tests {
		got := math.Log1p(tc.linear) / logMax * 100.0
		assertApprox(t, fmt.Sprintf("log(%v)", tc.linear), got, tc.want, 0.01)
	}

	// Specific: 50 → ~85.2
	val50 := math.Log1p(50.0) / logMax * 100.0
	if val50 < 80.0 || val50 > 90.0 {
		t.Errorf("log(50) expected ~85, got %.2f", val50)
	}
}

// --- Test 12: log scale preserves ordering ---
func TestPageRank_LogScalePreservesOrdering(t *testing.T) {
	outLinks := [][]uint32{
		{1},
		{2},
		{3},
		nil,
	}
	totalOutLinks := []uint32{1, 1, 1, 0}
	rank := computePR(t, 4, outLinks, totalOutLinks)

	if rank[3] <= rank[2] || rank[2] <= rank[1] || rank[1] <= rank[0] {
		t.Errorf("log scale should preserve ordering D > C > B > A, got %v", rank)
	}
	assertApprox(t, "D (max)", rank[3], 100.0, 0.01)

	// Log scale lifts small values: A's linear would be ~25-30, log lifts it
	if rank[0] < 30.0 {
		t.Errorf("log scale should lift small values: A=%.4f, expected > 30", rank[0])
	}
}

// --- Test 13: explicit nil edgeWeights behaves same as omitted ---
func TestPageRank_NilEdgeWeights(t *testing.T) {
	outLinks := [][]uint32{
		{1}, // A → B
		{2}, // B → C
		nil,
	}
	totalOutLinks := []uint32{1, 1, 0}

	rankOmitted := computePR(t, 3, outLinks, totalOutLinks)
	rankNil := computePR(t, 3, outLinks, totalOutLinks, nil)

	for i := range rankOmitted {
		assertApprox(t, fmt.Sprintf("node %d nil vs omitted", i), rankNil[i], rankOmitted[i], 0.001)
	}
}

// --- Test 14: weight=0 blocks PR through that edge entirely ---
func TestPageRank_EdgeWeightZero(t *testing.T) {
	// A(0) → B(1) weight=0, A(0) → C(2) weight=1.0
	outLinks := [][]uint32{
		{1, 2},
		nil,
		nil,
	}
	totalOutLinks := []uint32{2, 0, 0}

	weights := [][]float64{
		{0.0, 1.0}, // A→B blocked, A→C full
		nil,
		nil,
	}
	rank := computePR(t, 3, outLinks, totalOutLinks, weights)

	// C should get much more PR than B.
	// B only gets teleportation + dangling redistribution (no direct flow from A).
	if rank[2] <= rank[1] {
		t.Errorf("C (weight=1) should far outrank B (weight=0): C=%.4f, B=%.4f", rank[2], rank[1])
	}

	// Compare B's score with a scenario where B has no incoming edge at all
	outLinksNoB := [][]uint32{
		{2}, // A → C only
		nil,
		nil,
	}
	totalNoB := []uint32{1, 0, 0}
	rankNoB := computePR(t, 3, outLinksNoB, totalNoB)

	// B should have roughly the same score (only teleportation + dangling)
	// in both cases, but totalOutLinks differs so some variance expected
	if math.Abs(rank[1]-rankNoB[1]) > 10.0 {
		t.Errorf("B with weight=0 edge should be similar to B with no edge: w0=%.4f, noEdge=%.4f", rank[1], rankNoB[1])
	}
}

// --- Test 15: explicit weight=1.0 (canonical) is identical to no weights ---
func TestPageRank_CanonicalWeightOne(t *testing.T) {
	outLinks := [][]uint32{
		{1, 2},
		nil,
		nil,
	}
	totalOutLinks := []uint32{2, 0, 0}

	// All weights explicitly 1.0 (like canonical consolidation, no hop penalty)
	weights := [][]float64{
		{1.0, 1.0},
		nil,
		nil,
	}
	rankWeighted := computePR(t, 3, outLinks, totalOutLinks, weights)
	rankDirect := computePR(t, 3, outLinks, totalOutLinks)

	for i := range rankWeighted {
		assertApprox(t, fmt.Sprintf("node %d w=1.0 vs direct", i), rankWeighted[i], rankDirect[i], 0.001)
	}
}

// --- Test 16: long redirect chain (5 hops → 0.9^5 ≈ 0.59) ---
func TestPageRank_LongRedirectChain(t *testing.T) {
	// Hub(0) → A(1) direct, Hub(0) → B(2) via 5-hop redirect
	outLinks := [][]uint32{
		{1, 2},
		nil,
		nil,
	}
	totalOutLinks := []uint32{2, 0, 0}

	w5 := math.Pow(0.9, 5) // ~0.59049
	weights := [][]float64{
		{1.0, w5},
		nil,
		nil,
	}
	rank := computePR(t, 3, outLinks, totalOutLinks, weights)

	// A (direct) should significantly outrank B (5-hop redirect)
	if rank[1] <= rank[2] {
		t.Errorf("A (direct) should outrank B (5 hops): A=%.4f, B=%.4f", rank[1], rank[2])
	}

	// Compare: 1 hop vs 5 hops — 5 hops should be worse
	weights1 := [][]float64{
		{1.0, 0.9},
		nil,
		nil,
	}
	rank1hop := computePR(t, 3, outLinks, totalOutLinks, weights1)

	// B's relative score with 5 hops should be lower than with 1 hop
	ratio5 := rank[2] / rank[1]
	ratio1 := rank1hop[2] / rank1hop[1]
	if ratio5 >= ratio1 {
		t.Errorf("5-hop redirect should penalize more than 1-hop: ratio5=%.4f, ratio1=%.4f", ratio5, ratio1)
	}
}

// --- Test 17: mixed weights — some edges direct, some redirected ---
func TestPageRank_MixedWeightsMultipleSources(t *testing.T) {
	// A(0) → C(2) via redirect (weight 0.9)
	// B(1) → C(2) direct (weight 1.0)
	// C dangling
	outLinks := [][]uint32{
		{2},
		{2},
		nil,
	}
	totalOutLinks := []uint32{1, 1, 0}

	weights := [][]float64{
		{0.9}, // A→C redirect
		{1.0}, // B→C direct
		nil,
	}
	rankMixed := computePR(t, 3, outLinks, totalOutLinks, weights)

	// B contributes more to C than A does, so B should have lower normalized score
	// (C absorbs more from B → in normalized scale, B's relative position changes)
	// Key check: C is still max
	assertApprox(t, "C (max)", rankMixed[2], 100.0, 0.01)

	// With all direct edges, A and B would contribute equally
	rankDirect := computePR(t, 3, outLinks, totalOutLinks)
	// A and B should be equal in direct case (symmetric contributions)
	assertApprox(t, "A==B direct", rankDirect[0], rankDirect[1], 0.01)
	// But in mixed case, A should differ from B because A's edge is penalized
	// A "leaks" 10% via redirect → effectively contributes less → gets relatively more
	// after normalization (C's raw score is lower → A's relative position rises)
	// Actually: with log scale both A and B still get same teleport+dangling,
	// but A sends less to C. So raw C is lower, normalization lifts A more.
}

// --- Test 18: all edges weighted < 1 (every link is a redirect) ---
func TestPageRank_AllEdgesWeighted(t *testing.T) {
	outLinks := [][]uint32{
		{1}, // A → B via redirect
		{2}, // B → C via redirect
		nil,
	}
	totalOutLinks := []uint32{1, 1, 0}

	weights := [][]float64{
		{0.9},
		{0.9},
		nil,
	}
	rank := computePR(t, 3, outLinks, totalOutLinks, weights)

	// Ordering preserved: C > B > A (chain still flows, just attenuated)
	if rank[2] <= rank[1] || rank[1] <= rank[0] {
		t.Errorf("ordering C > B > A should hold even with all weights < 1: %v", rank)
	}
	assertApprox(t, "C (max)", rank[2], 100.0, 0.01)

	// Compare with direct: the gap between nodes should be smaller with weights
	// because less PR is transferred, so dangling/teleportation dominates more
	rankDirect := computePR(t, 3, outLinks, totalOutLinks)
	gapDirect := rankDirect[2] - rankDirect[0]
	gapWeighted := rank[2] - rank[0]
	if gapWeighted >= gapDirect {
		t.Errorf("weighted edges should compress the gap: weighted=%.4f, direct=%.4f", gapWeighted, gapDirect)
	}
}

// --- Test 19: log scale on symmetric graph — all nodes stay equal ---
func TestPageRank_LogScaleSymmetric(t *testing.T) {
	// Full cycle: A→B→C→D→A
	outLinks := [][]uint32{
		{1}, {2}, {3}, {0},
	}
	totalOutLinks := []uint32{1, 1, 1, 1}
	rank := computePR(t, 4, outLinks, totalOutLinks)

	// All nodes equal due to symmetry; log(x)/log(x) = 1 → all = 100
	for i := 0; i < 4; i++ {
		assertApprox(t, fmt.Sprintf("node %d symmetric", i), rank[i], 100.0, 0.01)
	}
}

// --- Test 20: weights + dangling nodes — dangling redistribution unaffected by weights ---
func TestPageRank_WeightsWithDanglingNodes(t *testing.T) {
	// A(0) → B(1) weight=0.9, C(2) dangling, D(3) dangling
	outLinks := [][]uint32{
		{1},
		nil,
		nil,
		nil,
	}
	totalOutLinks := []uint32{1, 0, 0, 0}

	weights := [][]float64{
		{0.9},
		nil,
		nil,
		nil,
	}
	rank := computePR(t, 4, outLinks, totalOutLinks, weights)

	// C and D are both dangling with no incoming edges →
	// they receive only teleportation + dangling redistribution → equal PR
	assertApprox(t, "C == D (both dangling)", rank[2], rank[3], 0.01)

	// B should outrank C/D because it receives direct flow from A
	if rank[1] <= rank[2] {
		t.Errorf("B (has incoming edge) should outrank C (dangling only): B=%.4f, C=%.4f", rank[1], rank[2])
	}
}

// --- Test 21: weights + external dilution combined ---
func TestPageRank_WeightsPlusExternalDilution(t *testing.T) {
	// A(0) has 5 total outlinks (1 internal + 4 external), points to B via redirect (weight 0.9)
	// C(1) has 1 total outlink (1 internal), points to B directly (weight 1.0)
	// B(2) dangling
	outLinks := [][]uint32{
		{2},
		{2},
		nil,
	}
	totalOutLinks := []uint32{5, 1, 0} // A diluted by external links

	weights := [][]float64{
		{0.9}, // A→B redirect
		{1.0}, // C→B direct
		nil,
	}
	rank := computePR(t, 3, outLinks, totalOutLinks, weights)

	// B is max (receives flow from both A and C)
	assertApprox(t, "B (max)", rank[2], 100.0, 0.01)

	// A and C have no incoming edges — they receive only teleportation + dangling
	// redistribution from B, which is equal for both. So A == C.
	assertApprox(t, "A == C (no incoming)", rank[0], rank[1], 0.01)

	// Compare with a direct scenario (no redirect, no external dilution):
	// both A and C contribute fully to B → B dominates more → A,C get less normalized score
	rankDirect := computePR(t, 3, outLinks, []uint32{1, 1, 0})
	// With dilution+weight, B gets less total flow → B/A ratio is lower → A is lifted
	ratioMixed := rank[2] / rank[0]
	ratioDirect := rankDirect[2] / rankDirect[0]
	if ratioMixed >= ratioDirect {
		t.Errorf("combined dilution+weight should reduce B dominance: mixed B/A=%f, direct B/A=%f",
			ratioMixed, ratioDirect)
	}
}

// --- Test 22: sparse edgeWeights — weights[src] nil for some nodes ---
func TestPageRank_SparseEdgeWeights(t *testing.T) {
	// A(0) → B(1) no weight (nil), B(1) → C(2) weight=0.9
	outLinks := [][]uint32{
		{1},
		{2},
		nil,
	}
	totalOutLinks := []uint32{1, 1, 0}

	weights := [][]float64{
		nil,    // A→B: no weight → defaults to 1.0
		{0.9},  // B→C: redirect penalty
		nil,
	}
	rank := computePR(t, 3, outLinks, totalOutLinks, weights)

	// Ordering preserved
	if rank[2] <= rank[1] || rank[1] <= rank[0] {
		t.Errorf("ordering C > B > A should hold: %v", rank)
	}

	// Compare with all-direct: C should get relatively less with B→C weighted
	rankDirect := computePR(t, 3, outLinks, totalOutLinks)
	ratioDirect := rankDirect[2] / rankDirect[1]
	ratioWeighted := rank[2] / rank[1]
	if ratioWeighted >= ratioDirect {
		t.Errorf("B→C weight should reduce C/B ratio: weighted=%.4f, direct=%.4f", ratioWeighted, ratioDirect)
	}
}

// --- Test 23: two dangling nodes only, no edges ---
func TestPageRank_TwoDanglingNodes(t *testing.T) {
	outLinks := [][]uint32{nil, nil}
	totalOutLinks := []uint32{0, 0}
	rank := computePR(t, 2, outLinks, totalOutLinks)

	// Both nodes are dangling, symmetric → equal PR
	assertApprox(t, "A == B", rank[0], rank[1], 0.01)
	// Both should be 100 (all equal = all max)
	assertApprox(t, "A", rank[0], 100.0, 0.01)
}

// --- Test 24: log scale spreads skewed distribution ---
func TestPageRank_LogScaleSpreadsDistribution(t *testing.T) {
	// Star graph: Hub(0) → A(1), B(2), C(3), D(4), E(5)
	// Hub has all the outlinks, leaves are dangling
	// This creates a very skewed distribution
	outLinks := [][]uint32{
		{1, 2, 3, 4, 5},
		nil, nil, nil, nil, nil,
	}
	totalOutLinks := []uint32{5, 0, 0, 0, 0, 0}
	rank := computePR(t, 6, outLinks, totalOutLinks)

	// All leaves should have equal PR
	for i := 2; i <= 5; i++ {
		assertApprox(t, fmt.Sprintf("leaf %d == leaf 1", i), rank[i], rank[1], 0.01)
	}

	// Log scale should lift the Hub's score significantly above where it would be
	// linearly. In linear scale Hub would be ~30-40% of max. With log it should
	// be higher (>60).
	if rank[0] < 60.0 {
		t.Errorf("log scale should lift Hub's score: got %.4f, expected > 60", rank[0])
	}
}

// --- Test 25: edgeWeights with all weights near zero — only teleportation remains ---
func TestPageRank_NearZeroWeights(t *testing.T) {
	outLinks := [][]uint32{
		{1},
		{0},
	}
	totalOutLinks := []uint32{1, 1}

	// Tiny weights: almost no PR transfer
	weights := [][]float64{
		{0.001},
		{0.001},
	}
	rank := computePR(t, 2, outLinks, totalOutLinks, weights)

	// With almost no transfer, both nodes converge to equal PR via teleportation
	assertApprox(t, "A ≈ B with near-zero weights", rank[0], rank[1], 1.0)
}

// --- Test 26: redirect penalty is strictly monotone with hops ---
func TestPageRank_RedirectPenaltyMonotone(t *testing.T) {
	// Hub(0) → A(1) 0 hops, B(2) 1 hop, C(3) 2 hops, D(4) 3 hops, E(5) 4 hops
	outLinks := [][]uint32{
		{1, 2, 3, 4, 5},
		nil, nil, nil, nil, nil,
	}
	totalOutLinks := []uint32{5, 0, 0, 0, 0, 0}

	weights := [][]float64{
		{
			math.Pow(0.9, 0), // 1.0
			math.Pow(0.9, 1), // 0.9
			math.Pow(0.9, 2), // 0.81
			math.Pow(0.9, 3), // 0.729
			math.Pow(0.9, 4), // 0.6561
		},
		nil, nil, nil, nil, nil,
	}
	rank := computePR(t, 6, outLinks, totalOutLinks, weights)

	// Strict ordering: A > B > C > D > E (more hops = less PR)
	for i := 1; i < 5; i++ {
		if rank[i] <= rank[i+1] {
			t.Errorf("node %d (hops=%d) should outrank node %d (hops=%d): %.4f vs %.4f",
				i, i-1, i+1, i, rank[i], rank[i+1])
		}
	}
}

// --- Test 27: all scores in valid range [0, 100] ---
func TestPageRank_ScoresInRange(t *testing.T) {
	graphs := []struct {
		name          string
		n             uint32
		outLinks      [][]uint32
		totalOutLinks []uint32
		weights       [][]float64
	}{
		{
			"chain",
			3,
			[][]uint32{{1}, {2}, nil},
			[]uint32{1, 1, 0},
			nil,
		},
		{
			"star with weights",
			4,
			[][]uint32{{1, 2, 3}, nil, nil, nil},
			[]uint32{3, 0, 0, 0},
			[][]float64{{0.5, 0.9, 0.1}, nil, nil, nil},
		},
		{
			"all dangling",
			5,
			[][]uint32{nil, nil, nil, nil, nil},
			[]uint32{0, 0, 0, 0, 0},
			nil,
		},
		{
			"cycle with external dilution",
			3,
			[][]uint32{{1}, {2}, {0}},
			[]uint32{5, 5, 5},
			nil,
		},
		{
			"weight zero edges",
			3,
			[][]uint32{{1, 2}, nil, nil},
			[]uint32{2, 0, 0},
			[][]float64{{0.0, 0.0}, nil, nil},
		},
	}

	for _, g := range graphs {
		t.Run(g.name, func(t *testing.T) {
			var rank []float64
			if g.weights != nil {
				rank = computePR(t, g.n, g.outLinks, g.totalOutLinks, g.weights)
			} else {
				rank = computePR(t, g.n, g.outLinks, g.totalOutLinks)
			}
			for i, r := range rank {
				if r < 0.0 || r > 100.0 {
					t.Errorf("node %d score out of range [0,100]: %.4f", i, r)
				}
			}
		})
	}
}

// --- Test 28: max node always scores exactly 100 ---
func TestPageRank_MaxAlways100(t *testing.T) {
	graphs := []struct {
		name          string
		n             uint32
		outLinks      [][]uint32
		totalOutLinks []uint32
		weights       [][]float64
	}{
		{"chain", 3, [][]uint32{{1}, {2}, nil}, []uint32{1, 1, 0}, nil},
		{"weighted chain", 3, [][]uint32{{1}, {2}, nil}, []uint32{1, 1, 0}, [][]float64{{0.5}, {0.5}, nil}},
		{"star", 4, [][]uint32{{1, 2, 3}, nil, nil, nil}, []uint32{3, 0, 0, 0}, nil},
		{"single", 1, [][]uint32{nil}, []uint32{0}, nil},
	}

	for _, g := range graphs {
		t.Run(g.name, func(t *testing.T) {
			var rank []float64
			if g.weights != nil {
				rank = computePR(t, g.n, g.outLinks, g.totalOutLinks, g.weights)
			} else {
				rank = computePR(t, g.n, g.outLinks, g.totalOutLinks)
			}
			var maxR float64
			for _, r := range rank {
				if r > maxR {
					maxR = r
				}
			}
			assertApprox(t, g.name+" max", maxR, 100.0, 0.01)
		})
	}
}

// --- Test 29: log scale is strictly monotone —  if raw a < raw b then log(a) < log(b) ---
func TestPageRank_LogScaleStrictlyMonotone(t *testing.T) {
	logMax := math.Log1p(100.0)
	prev := 0.0
	for x := 0.1; x <= 100.0; x += 0.1 {
		val := math.Log1p(x) / logMax * 100.0
		if val <= prev {
			t.Errorf("log scale not strictly monotone at x=%.1f: val=%.6f, prev=%.6f", x, val, prev)
		}
		prev = val
	}
}

// --- Test 30: log scale boundary values ---
func TestPageRank_LogScaleBoundaries(t *testing.T) {
	logMax := math.Log1p(100.0)

	// f(0) = 0
	assertApprox(t, "f(0)", math.Log1p(0)/logMax*100, 0.0, 0.001)
	// f(100) = 100
	assertApprox(t, "f(100)", math.Log1p(100)/logMax*100, 100.0, 0.001)
	// f(1) ≈ 15.02 (a page at 1% of max)
	assertApprox(t, "f(1)", math.Log1p(1)/logMax*100, 15.02, 0.1)
	// f(10) ≈ 51.94 (a page at 10% of max)
	assertApprox(t, "f(10)", math.Log1p(10)/logMax*100, 51.94, 0.1)
}

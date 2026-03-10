package storage

import (
	"math"
	"testing"
)

// helper: return normalized 0–100 scores for a graph.
func computePR(t *testing.T, n uint32, outLinks [][]uint32, totalOutLinks []uint32) []float64 {
	t.Helper()
	rank := ComputePageRankIterations(n, outLinks, totalOutLinks)
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

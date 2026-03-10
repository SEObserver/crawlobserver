package cfsolve

import "context"

// NullResolver is a no-op resolver that always returns unsolved.
type NullResolver struct{}

func (n *NullResolver) Solve(_ context.Context, _ string) (*SolveResult, error) {
	return &SolveResult{Solved: false}, nil
}

func (n *NullResolver) Close() {}

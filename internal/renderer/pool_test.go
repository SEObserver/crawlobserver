package renderer

import (
	"testing"
	"time"
)

func TestDefaultPoolOptions(t *testing.T) {
	opts := DefaultPoolOptions()

	if opts.MaxPages != 4 {
		t.Errorf("MaxPages = %d, want 4", opts.MaxPages)
	}
	if opts.PageTimeout != 15*time.Second {
		t.Errorf("PageTimeout = %v, want 15s", opts.PageTimeout)
	}
	if opts.UserAgent != "" {
		t.Errorf("UserAgent = %q, want empty string", opts.UserAgent)
	}
	if !opts.BlockResources {
		t.Error("BlockResources should default to true")
	}
	if !opts.Headless {
		t.Error("Headless should default to true")
	}
}

func TestDefaultPoolOptions_Immutability(t *testing.T) {
	opts1 := DefaultPoolOptions()
	opts2 := DefaultPoolOptions()

	// Modifying one should not affect the other (they are value types, not pointers)
	opts1.MaxPages = 99
	if opts2.MaxPages != 4 {
		t.Errorf("modifying opts1 affected opts2: MaxPages = %d, want 4", opts2.MaxPages)
	}
}

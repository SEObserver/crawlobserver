package frontier

import (
	"testing"
	"time"
)

func TestTimeUntilReady_UnknownHost(t *testing.T) {
	hq := NewHostQueue(500 * time.Millisecond)

	d := hq.TimeUntilReady("unknown.com")
	if d != 0 {
		t.Errorf("TimeUntilReady for unknown host = %v, want 0", d)
	}
}

func TestTimeUntilReady_AfterFetch(t *testing.T) {
	hq := NewHostQueue(500 * time.Millisecond)

	hq.RecordFetch("example.com")
	d := hq.TimeUntilReady("example.com")

	// Should be close to 500ms (just recorded, so almost full delay remaining)
	if d <= 0 {
		t.Errorf("TimeUntilReady immediately after fetch = %v, want > 0", d)
	}
	if d > 500*time.Millisecond {
		t.Errorf("TimeUntilReady = %v, should not exceed delay of 500ms", d)
	}
}

func TestTimeUntilReady_AfterDelayElapsed(t *testing.T) {
	hq := NewHostQueue(50 * time.Millisecond)

	hq.RecordFetch("example.com")
	time.Sleep(60 * time.Millisecond)

	d := hq.TimeUntilReady("example.com")
	if d != 0 {
		t.Errorf("TimeUntilReady after delay elapsed = %v, want 0", d)
	}
}

func TestTimeUntilReady_DifferentHosts(t *testing.T) {
	hq := NewHostQueue(200 * time.Millisecond)

	hq.RecordFetch("a.com")

	// a.com should have a wait
	dA := hq.TimeUntilReady("a.com")
	if dA <= 0 {
		t.Errorf("TimeUntilReady for a.com = %v, want > 0", dA)
	}

	// b.com was never fetched, should be ready
	dB := hq.TimeUntilReady("b.com")
	if dB != 0 {
		t.Errorf("TimeUntilReady for b.com = %v, want 0", dB)
	}
}

func TestSetDelay(t *testing.T) {
	hq := NewHostQueue(100 * time.Millisecond)

	hq.RecordFetch("example.com")

	// With 100ms delay, should not be ready immediately
	if hq.CanFetch("example.com") {
		t.Error("should not be able to fetch immediately with 100ms delay")
	}

	// Change delay to 0
	hq.SetDelay(0)

	// Now should be able to fetch immediately
	if !hq.CanFetch("example.com") {
		t.Error("should be able to fetch after setting delay to 0")
	}
}

func TestSetDelay_IncreasesDelay(t *testing.T) {
	hq := NewHostQueue(10 * time.Millisecond)

	hq.RecordFetch("example.com")
	time.Sleep(15 * time.Millisecond)

	// With 10ms delay, should be ready after 15ms
	if !hq.CanFetch("example.com") {
		t.Error("should be ready after 15ms with 10ms delay")
	}

	// Increase delay to 1 second
	hq.SetDelay(1 * time.Second)
	hq.RecordFetch("example.com")

	// Should NOT be ready with the new longer delay
	if hq.CanFetch("example.com") {
		t.Error("should NOT be ready with 1s delay right after fetch")
	}

	// TimeUntilReady should reflect the new delay
	d := hq.TimeUntilReady("example.com")
	if d < 900*time.Millisecond {
		t.Errorf("TimeUntilReady = %v, expected close to 1s", d)
	}
}

func TestCanFetch_FirstTime(t *testing.T) {
	hq := NewHostQueue(1 * time.Second)

	if !hq.CanFetch("new-host.com") {
		t.Error("first fetch to any host should be allowed")
	}
}

func TestCanFetch_RespectDelay(t *testing.T) {
	hq := NewHostQueue(50 * time.Millisecond)

	hq.RecordFetch("example.com")

	if hq.CanFetch("example.com") {
		t.Error("should not be able to fetch within delay period")
	}

	time.Sleep(60 * time.Millisecond)

	if !hq.CanFetch("example.com") {
		t.Error("should be able to fetch after delay elapsed")
	}
}

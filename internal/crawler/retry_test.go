package crawler

import (
	"container/heap"
	"sync"
	"testing"
	"time"
)

func newTestPolicy() *RetryPolicy {
	return &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   30 * time.Second,
	}
}

func TestShouldRetry_RetryableCodes(t *testing.T) {
	p := newTestPolicy()
	for _, code := range []int{429, 500, 502, 503, 504} {
		if !p.ShouldRetry(code, "", 0) {
			t.Errorf("expected ShouldRetry=true for status %d, attempt 0", code)
		}
	}
}

func TestShouldRetry_NonRetryableCodes(t *testing.T) {
	p := newTestPolicy()
	for _, code := range []int{200, 301, 403, 404} {
		if p.ShouldRetry(code, "", 0) {
			t.Errorf("expected ShouldRetry=false for status %d", code)
		}
	}
}

func TestShouldRetry_NetworkErrors(t *testing.T) {
	p := newTestPolicy()

	retryable := []string{"timeout", "connection_refused", "connection refused", "connection reset", "eof"}
	for _, errStr := range retryable {
		if !p.ShouldRetry(0, errStr, 0) {
			t.Errorf("expected ShouldRetry=true for error %q", errStr)
		}
	}

	nonRetryable := []string{"dns_not_found", "tls_error"}
	for _, errStr := range nonRetryable {
		if p.ShouldRetry(0, errStr, 0) {
			t.Errorf("expected ShouldRetry=false for error %q", errStr)
		}
	}
}

func TestShouldRetry_DNSTimeout_MaxOneRetry(t *testing.T) {
	p := newTestPolicy()
	if !p.ShouldRetry(0, "dns_timeout", 0) {
		t.Error("dns_timeout should retry on attempt 0")
	}
	if p.ShouldRetry(0, "dns_timeout", 1) {
		t.Error("dns_timeout should NOT retry on attempt 1")
	}
}

func TestShouldRetry_MaxAttempts(t *testing.T) {
	p := newTestPolicy()
	if p.ShouldRetry(500, "", 3) {
		t.Error("should not retry when attempt == MaxRetries")
	}
	if p.ShouldRetry(500, "", 5) {
		t.Error("should not retry when attempt > MaxRetries")
	}
}

func TestShouldRetry_Disabled(t *testing.T) {
	p := &RetryPolicy{MaxRetries: 0}
	if p.ShouldRetry(500, "", 0) {
		t.Error("should not retry when MaxRetries=0")
	}
}

func TestComputeDelay_Exponential(t *testing.T) {
	p := &RetryPolicy{
		MaxRetries: 5,
		BaseDelay:  1 * time.Second,
		MaxDelay:   120 * time.Second,
	}

	// Without jitter, delays should roughly double each time
	// We run multiple samples and check averages
	for attempt := 0; attempt < 4; attempt++ {
		var total time.Duration
		n := 100
		for i := 0; i < n; i++ {
			total += p.ComputeDelay(attempt, "")
		}
		avg := total / time.Duration(n)
		// Expected base: 1s * 2^attempt
		expectedBase := time.Duration(float64(time.Second) * float64(int(1)<<attempt))
		// Average should be around expectedBase * 1.25 (base + avg jitter of base/4)
		// Allow wide range due to randomness
		if avg < expectedBase/2 || avg > expectedBase*3 {
			t.Errorf("attempt %d: avg delay %v outside expected range [%v, %v]",
				attempt, avg, expectedBase/2, expectedBase*3)
		}
	}
}

func TestComputeDelay_CappedAtMax(t *testing.T) {
	p := &RetryPolicy{
		MaxRetries: 10,
		BaseDelay:  1 * time.Second,
		MaxDelay:   10 * time.Second,
	}

	for i := 0; i < 100; i++ {
		d := p.ComputeDelay(8, "") // 2^8 = 256s >> 10s max
		if d > p.MaxDelay {
			t.Errorf("delay %v exceeds MaxDelay %v", d, p.MaxDelay)
		}
	}
}

func TestComputeDelay_RetryAfterSeconds(t *testing.T) {
	p := newTestPolicy()
	d := p.ComputeDelay(0, "120")
	if d != 30*time.Second {
		// 120s > MaxDelay(30s), should be capped
		t.Errorf("expected 30s (capped), got %v", d)
	}

	d = p.ComputeDelay(0, "5")
	if d != 5*time.Second {
		t.Errorf("expected 5s, got %v", d)
	}
}

func TestComputeDelay_RetryAfterDate(t *testing.T) {
	p := &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   120 * time.Second,
	}

	future := time.Now().Add(10 * time.Second)
	httpDate := future.UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	d := p.ComputeDelay(0, httpDate)

	// Should be approximately 10s (allow 2s tolerance)
	if d < 8*time.Second || d > 12*time.Second {
		t.Errorf("expected ~10s for Retry-After date, got %v", d)
	}
}

func TestComputeDelay_Jitter(t *testing.T) {
	p := &RetryPolicy{
		MaxRetries: 3,
		BaseDelay:  1 * time.Second,
		MaxDelay:   60 * time.Second,
	}

	seen := make(map[time.Duration]bool)
	for i := 0; i < 100; i++ {
		d := p.ComputeDelay(1, "")
		seen[d] = true
	}

	if len(seen) < 5 {
		t.Errorf("expected variance in delays, got only %d distinct values", len(seen))
	}
}

func TestRetryQueue_Order(t *testing.T) {
	rq := NewRetryQueue()

	now := time.Now()
	rq.Push(&RetryItem{URL: "c", ReadyAt: now.Add(3 * time.Second)})
	rq.Push(&RetryItem{URL: "a", ReadyAt: now.Add(1 * time.Second)})
	rq.Push(&RetryItem{URL: "b", ReadyAt: now.Add(2 * time.Second)})

	// Pop in heap order using container/heap.Pop
	rq.mu.Lock()
	var order []string
	for rq.heap.Len() > 0 {
		item := heap.Pop(&rq.heap).(*RetryItem)
		order = append(order, item.URL)
	}
	rq.mu.Unlock()

	expected := []string{"a", "b", "c"}
	for i, url := range order {
		if url != expected[i] {
			t.Errorf("position %d: expected %s, got %s", i, expected[i], url)
		}
	}
}

func TestRetryQueue_PopReadyRespectsTime(t *testing.T) {
	rq := NewRetryQueue()

	// Item in the future
	rq.Push(&RetryItem{URL: "future", ReadyAt: time.Now().Add(1 * time.Hour)})
	// Item in the past
	rq.Push(&RetryItem{URL: "past", ReadyAt: time.Now().Add(-1 * time.Second)})

	item := rq.PopReady()
	if item == nil || item.URL != "past" {
		t.Errorf("expected 'past' item, got %v", item)
	}

	item = rq.PopReady()
	if item != nil {
		t.Errorf("expected nil for future item, got %v", item.URL)
	}

	if rq.Len() != 1 {
		t.Errorf("expected 1 remaining item, got %d", rq.Len())
	}
}

func TestRetryQueue_Concurrent(t *testing.T) {
	rq := NewRetryQueue()
	var wg sync.WaitGroup

	// Concurrent pushes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			rq.Push(&RetryItem{
				URL:     "url",
				ReadyAt: time.Now().Add(-time.Duration(i) * time.Millisecond),
			})
		}(i)
	}
	wg.Wait()

	if rq.Len() != 100 {
		t.Errorf("expected 100 items, got %d", rq.Len())
	}

	// Concurrent pops
	var popped int64
	var mu sync.Mutex
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				item := rq.PopReady()
				if item == nil {
					return
				}
				mu.Lock()
				popped++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if popped != 100 {
		t.Errorf("expected 100 pops, got %d", popped)
	}
}

// --- parseRetryAfter edge case tests ---

func TestParseRetryAfter_ValidSeconds(t *testing.T) {
	d := parseRetryAfter("5")
	if d != 5*time.Second {
		t.Errorf("parseRetryAfter(\"5\") = %v, want 5s", d)
	}
}

func TestParseRetryAfter_ZeroSeconds(t *testing.T) {
	d := parseRetryAfter("0")
	// strconv.Atoi("0") succeeds but secs=0, condition is secs > 0, so falls through
	if d != 0 {
		t.Errorf("parseRetryAfter(\"0\") = %v, want 0 (zero seconds should not be valid)", d)
	}
}

func TestParseRetryAfter_NegativeSeconds(t *testing.T) {
	d := parseRetryAfter("-10")
	// Atoi succeeds but secs < 0, condition is secs > 0, so falls through
	if d != 0 {
		t.Errorf("parseRetryAfter(\"-10\") = %v, want 0 (negative seconds should not be valid)", d)
	}
}

func TestParseRetryAfter_EmptyString(t *testing.T) {
	d := parseRetryAfter("")
	if d != 0 {
		t.Errorf("parseRetryAfter(\"\") = %v, want 0", d)
	}
}

func TestParseRetryAfter_WhitespaceOnly(t *testing.T) {
	d := parseRetryAfter("   ")
	if d != 0 {
		t.Errorf("parseRetryAfter(\"   \") = %v, want 0", d)
	}
}

func TestParseRetryAfter_Garbage(t *testing.T) {
	d := parseRetryAfter("not-a-number-or-date")
	if d != 0 {
		t.Errorf("parseRetryAfter(\"not-a-number-or-date\") = %v, want 0", d)
	}
}

func TestParseRetryAfter_PastDate(t *testing.T) {
	// A date in the past should return 0 (time.Until returns negative)
	past := time.Now().Add(-1 * time.Hour).UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	d := parseRetryAfter(past)
	if d != 0 {
		t.Errorf("parseRetryAfter(past date) = %v, want 0", d)
	}
}

func TestParseRetryAfter_FutureDate(t *testing.T) {
	future := time.Now().Add(30 * time.Second).UTC().Format("Mon, 02 Jan 2006 15:04:05 GMT")
	d := parseRetryAfter(future)
	// Should be approximately 30s (allow tolerance)
	if d < 25*time.Second || d > 35*time.Second {
		t.Errorf("parseRetryAfter(future date) = %v, want ~30s", d)
	}
}

func TestParseRetryAfter_WithLeadingTrailingSpaces(t *testing.T) {
	d := parseRetryAfter("  10  ")
	if d != 10*time.Second {
		t.Errorf("parseRetryAfter(\"  10  \") = %v, want 10s", d)
	}
}

func TestHostHealth_ConsecutiveTracking(t *testing.T) {
	hh := NewHostHealth()

	hh.RecordFailure("example.com")
	hh.RecordFailure("example.com")
	hh.RecordFailure("example.com")

	hh.mu.Lock()
	consec := hh.hosts["example.com"].consecutiveFailures
	hh.mu.Unlock()

	if consec != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", consec)
	}

	// Success resets consecutive
	hh.RecordSuccess("example.com")
	hh.mu.Lock()
	consec = hh.hosts["example.com"].consecutiveFailures
	hh.mu.Unlock()

	if consec != 0 {
		t.Errorf("expected 0 consecutive failures after success, got %d", consec)
	}
}

func TestHostHealth_Threshold(t *testing.T) {
	hh := NewHostHealth()

	// Under threshold
	for i := 0; i < 9; i++ {
		hh.RecordFailure("example.com")
	}
	if !hh.ShouldRetry("example.com", 10) {
		t.Error("should retry when consecutive < threshold")
	}

	// At threshold
	hh.RecordFailure("example.com")
	if hh.ShouldRetry("example.com", 10) {
		t.Error("should NOT retry when consecutive >= threshold")
	}

	// Unknown host should be retryable
	if !hh.ShouldRetry("unknown.com", 10) {
		t.Error("unknown host should be retryable")
	}
}

func TestHostHealth_GlobalRate(t *testing.T) {
	hh := NewHostHealth()

	// 3 successes, 1 failure on host A
	for i := 0; i < 3; i++ {
		hh.RecordSuccess("a.com")
	}
	hh.RecordFailure("a.com")

	// 1 success, 1 failure on host B
	hh.RecordSuccess("b.com")
	hh.RecordFailure("b.com")

	// Total: 4 successes, 2 failures = 2/6 ≈ 0.333
	rate := hh.GlobalErrorRate()
	expected := 2.0 / 6.0
	if rate < expected-0.01 || rate > expected+0.01 {
		t.Errorf("expected error rate ~%.3f, got %.3f", expected, rate)
	}

	// Empty should return 0
	empty := NewHostHealth()
	if empty.GlobalErrorRate() != 0 {
		t.Error("empty HostHealth should have 0 error rate")
	}
}

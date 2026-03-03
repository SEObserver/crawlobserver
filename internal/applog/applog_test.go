package applog

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"
)

type mockLogStore struct {
	mu    sync.Mutex
	logs  []LogRow
	err   error
	calls int
}

func (m *mockLogStore) InsertLogs(_ context.Context, logs []LogRow) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	if m.err != nil {
		return m.err
	}
	m.logs = append(m.logs, logs...)
	return nil
}

func (m *mockLogStore) getLogs() []LogRow {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]LogRow, len(m.logs))
	copy(cp, m.logs)
	return cp
}

func (m *mockLogStore) getCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

func setupLogger(store LogStore) func() {
	l := &Logger{
		store:     store,
		batchSize: defaultBatchSize,
		flushInt:  defaultFlushInterval,
		done:      make(chan struct{}),
	}
	l.wg.Add(1)
	go l.flushLoop()
	global = l
	return func() {
		Close()
	}
}

func TestInitAndClose(t *testing.T) {
	store := &mockLogStore{}
	Init(store)
	if global == nil {
		t.Fatal("global logger should be set after Init")
	}
	Close()
	if global != nil {
		t.Error("global logger should be nil after Close")
	}
}

func TestCloseNil(t *testing.T) {
	global = nil
	Close() // should not panic
}

func TestEmitLevels(t *testing.T) {
	store := &mockLogStore{}
	cleanup := setupLogger(store)
	defer cleanup()

	Info("comp", "info msg")
	Warn("comp", "warn msg")
	Error("comp", "error msg")
	Debug("comp", "debug msg")

	global.flush()

	logs := store.getLogs()
	if len(logs) != 4 {
		t.Fatalf("got %d logs, want 4", len(logs))
	}

	levels := []string{"info", "warn", "error", "debug"}
	for i, want := range levels {
		if logs[i].Level != want {
			t.Errorf("logs[%d].Level = %q, want %q", i, logs[i].Level, want)
		}
		if logs[i].Component != "comp" {
			t.Errorf("logs[%d].Component = %q, want %q", i, logs[i].Component, "comp")
		}
	}
}

func TestEmitWithKeyvals(t *testing.T) {
	store := &mockLogStore{}
	cleanup := setupLogger(store)
	defer cleanup()

	Info("test", "msg", "key1", "val1", "key2", 42)
	global.flush()

	logs := store.getLogs()
	if len(logs) != 1 {
		t.Fatalf("got %d logs, want 1", len(logs))
	}
	if logs[0].Context == "" {
		t.Error("expected non-empty context with keyvals")
	}
}

func TestEmitFormatted(t *testing.T) {
	store := &mockLogStore{}
	cleanup := setupLogger(store)
	defer cleanup()

	Infof("test", "count=%d", 42)
	Warnf("test", "rate=%.1f", 3.14)
	Errorf("test", "err=%s", "fail")

	global.flush()

	logs := store.getLogs()
	if len(logs) != 3 {
		t.Fatalf("got %d logs, want 3", len(logs))
	}
	if logs[0].Message != "count=42" {
		t.Errorf("Message = %q, want %q", logs[0].Message, "count=42")
	}
}

func TestBatchFlush(t *testing.T) {
	store := &mockLogStore{}
	l := &Logger{
		store:     store,
		batchSize: 3,
		flushInt:  time.Hour, // won't trigger by timer
		done:      make(chan struct{}),
	}
	l.wg.Add(1)
	go l.flushLoop()
	global = l

	// Append 3 logs to trigger batch flush
	Info("test", "msg1")
	Info("test", "msg2")
	Info("test", "msg3")

	// Give a moment for the batch flush
	time.Sleep(50 * time.Millisecond)

	logs := store.getLogs()
	if len(logs) != 3 {
		t.Errorf("got %d logs, want 3 (batch trigger)", len(logs))
	}

	Close()
}

func TestFlushErrorReBuffers(t *testing.T) {
	store := &mockLogStore{err: fmt.Errorf("insert failed")}
	l := &Logger{
		store:     store,
		batchSize: defaultBatchSize,
		flushInt:  time.Hour,
		done:      make(chan struct{}),
	}
	l.wg.Add(1)
	go l.flushLoop()
	global = l

	Info("test", "msg1")
	Info("test", "msg2")

	global.flush()

	// Logs should be re-buffered
	global.mu.Lock()
	bufLen := len(global.buf)
	global.mu.Unlock()

	if bufLen != 2 {
		t.Errorf("buffer len = %d, want 2 (re-buffered after error)", bufLen)
	}

	close(global.done)
	global.wg.Wait()
	global = nil
}

func TestBufferCap(t *testing.T) {
	store := &mockLogStore{err: fmt.Errorf("always fail")}
	l := &Logger{
		store:     store,
		batchSize: defaultBatchSize,
		flushInt:  time.Hour,
		done:      make(chan struct{}),
	}
	l.wg.Add(1)
	go l.flushLoop()
	global = l

	// Fill buffer beyond maxBufferSize
	for i := 0; i < maxBufferSize+100; i++ {
		l.mu.Lock()
		l.buf = append(l.buf, LogRow{Message: fmt.Sprintf("msg-%d", i)})
		l.mu.Unlock()
	}

	global.flush()

	global.mu.Lock()
	bufLen := len(global.buf)
	global.mu.Unlock()

	if bufLen > maxBufferSize {
		t.Errorf("buffer len = %d, should be capped at %d", bufLen, maxBufferSize)
	}

	close(global.done)
	global.wg.Wait()
	global = nil
}

func TestEmitWithoutInit(t *testing.T) {
	global = nil
	// Should not panic, just log to stderr
	Info("test", "msg without init")
	Warn("test", "warn without init")
}

func TestFlushLoop(t *testing.T) {
	store := &mockLogStore{}
	l := &Logger{
		store:     store,
		batchSize: defaultBatchSize,
		flushInt:  50 * time.Millisecond,
		done:      make(chan struct{}),
	}
	l.wg.Add(1)
	go l.flushLoop()
	global = l

	Info("test", "msg1")

	// Wait for at least one tick
	time.Sleep(150 * time.Millisecond)

	logs := store.getLogs()
	if len(logs) != 1 {
		t.Errorf("got %d logs, want 1 (timer flush)", len(logs))
	}

	Close()
}

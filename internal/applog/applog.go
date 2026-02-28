package applog

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"
)

const (
	defaultBatchSize     = 200
	defaultFlushInterval = 5 * time.Second
	maxBufferSize        = 10000
)

// LogRow represents a single log entry.
type LogRow struct {
	Timestamp time.Time `json:"timestamp"`
	Level     string    `json:"level"`
	Component string    `json:"component"`
	Message   string    `json:"message"`
	Context   string    `json:"context,omitempty"`
}

// LogStore is the interface for persisting logs.
type LogStore interface {
	InsertLogs(ctx context.Context, logs []LogRow) error
}

// Logger buffers log rows and flushes them to a LogStore.
type Logger struct {
	store     LogStore
	mu        sync.Mutex
	buf       []LogRow
	batchSize int
	flushInt  time.Duration
	done      chan struct{}
	wg        sync.WaitGroup
}

var global *Logger

// Init initializes the global logger with a store backend.
func Init(store LogStore) {
	l := &Logger{
		store:     store,
		batchSize: defaultBatchSize,
		flushInt:  defaultFlushInterval,
		done:      make(chan struct{}),
	}
	l.wg.Add(1)
	go l.flushLoop()
	global = l
}

// Close flushes remaining logs and stops the background goroutine.
func Close() {
	if global == nil {
		return
	}
	close(global.done)
	global.wg.Wait()
	global.flush()
	global = nil
}

func (l *Logger) append(row LogRow) {
	l.mu.Lock()
	l.buf = append(l.buf, row)
	shouldFlush := len(l.buf) >= l.batchSize
	l.mu.Unlock()
	if shouldFlush {
		l.flush()
	}
}

func (l *Logger) flush() {
	l.mu.Lock()
	if len(l.buf) == 0 {
		l.mu.Unlock()
		return
	}
	batch := l.buf
	l.buf = nil
	l.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := l.store.InsertLogs(ctx, batch); err != nil {
		log.Printf("applog: failed to flush %d logs: %v", len(batch), err)
		// Re-buffer the failed batch, capped to avoid OOM
		l.mu.Lock()
		l.buf = append(batch, l.buf...)
		if len(l.buf) > maxBufferSize {
			dropped := len(l.buf) - maxBufferSize
			l.buf = l.buf[:maxBufferSize]
			log.Printf("applog: buffer full, dropped %d oldest logs", dropped)
		}
		l.mu.Unlock()
	}
}

func (l *Logger) flushLoop() {
	defer l.wg.Done()
	ticker := time.NewTicker(l.flushInt)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.flush()
		case <-l.done:
			return
		}
	}
}

func emit(level, component, message string, keyvals []any) {
	// Always log to stderr
	log.Printf("[%s] %s: %s", level, component, message)

	if global == nil {
		return
	}

	var ctx string
	if len(keyvals) > 0 {
		kv := make(map[string]any, len(keyvals)/2)
		for i := 0; i+1 < len(keyvals); i += 2 {
			kv[fmt.Sprint(keyvals[i])] = keyvals[i+1]
		}
		if b, err := json.Marshal(kv); err == nil {
			ctx = string(b)
		}
	}

	global.append(LogRow{
		Timestamp: time.Now(),
		Level:     level,
		Component: component,
		Message:   message,
		Context:   ctx,
	})
}

// Debug logs at debug level.
func Debug(component, msg string, keyvals ...any) {
	emit("debug", component, msg, keyvals)
}

// Info logs at info level.
func Info(component, msg string, keyvals ...any) {
	emit("info", component, msg, keyvals)
}

// Warn logs at warn level.
func Warn(component, msg string, keyvals ...any) {
	emit("warn", component, msg, keyvals)
}

// Error logs at error level.
func Error(component, msg string, keyvals ...any) {
	emit("error", component, msg, keyvals)
}

// Infof logs at info level with format string.
func Infof(component, format string, args ...any) {
	emit("info", component, fmt.Sprintf(format, args...), nil)
}

// Warnf logs at warn level with format string.
func Warnf(component, format string, args ...any) {
	emit("warn", component, fmt.Sprintf(format, args...), nil)
}

// Errorf logs at error level with format string.
func Errorf(component, format string, args ...any) {
	emit("error", component, fmt.Sprintf(format, args...), nil)
}

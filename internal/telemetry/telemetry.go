package telemetry

import (
	"sync"

	"github.com/posthog/posthog-go"
)

const (
	posthogKey  = "phc_gbbCqXPOTdBLcN9HDvp7GYUUF7QZ4VDooPCxsJBwYcY"
	posthogHost = "https://eu.i.posthog.com"
)

var (
	mu         sync.Mutex
	client     posthog.Client
	instanceID string
	appVersion string
)

// Init initialises the PostHog client when telemetry is enabled.
// Safe to call multiple times; only the first call with enabled=true creates a client.
func Init(enabled bool, id, version string) {
	mu.Lock()
	defer mu.Unlock()

	if client != nil {
		return
	}
	if !enabled {
		return
	}

	c, err := posthog.NewWithConfig(posthogKey, posthog.Config{
		Endpoint:  posthogHost,
		BatchSize: 1,
	})
	if err != nil {
		return
	}

	client = c
	instanceID = id
	appVersion = version
}

// Track sends an event to PostHog. No-op if telemetry is disabled.
func Track(event string, props posthog.Properties) {
	mu.Lock()
	c := client
	id := instanceID
	ver := appVersion
	mu.Unlock()

	if c == nil {
		return
	}

	if props == nil {
		props = posthog.NewProperties()
	}
	props.Set("$app_version", ver)

	_ = c.Enqueue(posthog.Capture{
		DistinctId: id,
		Event:      event,
		Properties: props,
	})
}

// Close flushes pending events and shuts down the client.
// Safe to call even if telemetry is disabled.
func Close() {
	mu.Lock()
	c := client
	mu.Unlock()

	if c != nil {
		_ = c.Close()
	}
}

// IsEnabled reports whether telemetry is active.
func IsEnabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return client != nil
}

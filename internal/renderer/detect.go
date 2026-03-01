package renderer

import (
	"bytes"
	"strings"

	"github.com/SEObserver/crawlobserver/internal/parser"
)

// DetectionMode controls when JS rendering is applied.
type DetectionMode string

const (
	ModeOff    DetectionMode = "off"
	ModeAuto   DetectionMode = "auto"
	ModeAlways DetectionMode = "always"
)

// ParseDetectionMode converts a string to a DetectionMode.
func ParseDetectionMode(s string) DetectionMode {
	switch strings.ToLower(s) {
	case "auto":
		return ModeAuto
	case "always":
		return ModeAlways
	default:
		return ModeOff
	}
}

// Framework markers that indicate JS-rendered content.
var frameworkMarkers = [][]byte{
	[]byte("__next"),
	[]byte("__nuxt"),
	[]byte("__NUXT__"),
	[]byte("__NEXT_DATA__"),
	[]byte("data-reactroot"),
	[]byte("data-react-helmet"),
	[]byte("ng-app"),
	[]byte("ng-version"),
	[]byte("data-v-"),          // Vue scoped styles
	[]byte("id=\"app\""),       // common Vue/React mount
	[]byte("id=\"root\""),      // common React mount
	[]byte("id=\"__next\""),    // Next.js
	[]byte("id=\"__nuxt\""),    // Nuxt
	[]byte("data-server-rendered"), // Nuxt SSR
}

const autoThreshold = 4

// NeedsRendering determines whether a page should be JS-rendered.
func NeedsRendering(mode DetectionMode, body []byte, staticData *parser.PageData) bool {
	switch mode {
	case ModeOff:
		return false
	case ModeAlways:
		return true
	case ModeAuto:
		return scoreNeedsRendering(body, staticData) >= autoThreshold
	default:
		return false
	}
}

func scoreNeedsRendering(body []byte, pd *parser.PageData) int {
	score := 0

	// Low word count
	if pd.WordCount < 50 {
		score += 3
	} else if pd.WordCount < 150 {
		score += 1
	}

	// Missing H1
	if len(pd.H1) == 0 {
		score += 2
	}

	// Empty title
	if strings.TrimSpace(pd.Title) == "" {
		score += 2
	}

	// Framework markers
	for _, marker := range frameworkMarkers {
		if bytes.Contains(body, marker) {
			score += 2
			break
		}
	}

	// <noscript> tag presence
	if bytes.Contains(body, []byte("<noscript")) {
		score += 1
	}

	// Many scripts + little text
	scriptCount := bytes.Count(body, []byte("<script"))
	if scriptCount > 5 && pd.WordCount < 100 {
		score += 2
	}

	return score
}

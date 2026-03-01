package renderer

import (
	"strings"
	"testing"

	"github.com/SEObserver/crawlobserver/internal/parser"
)

func TestParseDetectionMode(t *testing.T) {
	tests := []struct {
		input string
		want  DetectionMode
	}{
		{"auto", ModeAuto},
		{"Auto", ModeAuto},
		{"AUTO", ModeAuto},
		{"always", ModeAlways},
		{"Always", ModeAlways},
		{"ALWAYS", ModeAlways},
		{"off", ModeOff},
		{"Off", ModeOff},
		{"OFF", ModeOff},
		{"", ModeOff},
		{"invalid", ModeOff},
		{"none", ModeOff},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := ParseDetectionMode(tt.input)
			if got != tt.want {
				t.Errorf("ParseDetectionMode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestNeedsRendering_ModeOff(t *testing.T) {
	// ModeOff always returns false regardless of content
	body := []byte(`<html><div id="__next"></div></html>`)
	pd := &parser.PageData{WordCount: 0}
	if NeedsRendering(ModeOff, body, pd) {
		t.Error("ModeOff should always return false")
	}
}

func TestNeedsRendering_ModeAlways(t *testing.T) {
	// ModeAlways always returns true regardless of content
	body := []byte(`<html><h1>Hello</h1><p>Lots of content here for testing</p></html>`)
	pd := &parser.PageData{WordCount: 500, H1: []string{"Hello"}, Title: "Page Title"}
	if !NeedsRendering(ModeAlways, body, pd) {
		t.Error("ModeAlways should always return true")
	}
}

func TestNeedsRendering_ModeAuto_HighScore(t *testing.T) {
	// Page with low word count + missing H1 + framework marker → high score → true
	body := []byte(`<html><div id="__next"><script></script></div></html>`)
	pd := &parser.PageData{WordCount: 10}
	if !NeedsRendering(ModeAuto, body, pd) {
		t.Error("ModeAuto with high-score page should return true")
	}
}

func TestNeedsRendering_ModeAuto_LowScore(t *testing.T) {
	// Page with plenty of content, H1, title, no framework markers → low score → false
	body := []byte(`<html><h1>Title</h1><p>content</p></html>`)
	pd := &parser.PageData{WordCount: 500, H1: []string{"Title"}, Title: "Page Title"}
	if NeedsRendering(ModeAuto, body, pd) {
		t.Error("ModeAuto with low-score page should return false")
	}
}

func TestNeedsRendering_UnknownMode(t *testing.T) {
	body := []byte(`<html></html>`)
	pd := &parser.PageData{}
	if NeedsRendering(DetectionMode("unknown"), body, pd) {
		t.Error("unknown mode should return false")
	}
}

func TestScoreNeedsRendering_WordCount(t *testing.T) {
	tests := []struct {
		name      string
		wordCount int
		minScore  int
	}{
		{"very low words", 10, 3},
		{"low words", 100, 1},
		{"high words", 500, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pd := &parser.PageData{
				WordCount: tt.wordCount,
				H1:        []string{"Present"},
				Title:     "Has Title",
			}
			score := scoreNeedsRendering([]byte(`<html></html>`), pd)
			if score < tt.minScore {
				t.Errorf("wordCount=%d: score=%d, want >= %d", tt.wordCount, score, tt.minScore)
			}
		})
	}
}

func TestScoreNeedsRendering_MissingH1(t *testing.T) {
	pd := &parser.PageData{WordCount: 500, Title: "Has Title"}
	score := scoreNeedsRendering([]byte(`<html></html>`), pd)
	if score < 2 {
		t.Errorf("missing H1: score=%d, want >= 2", score)
	}
}

func TestScoreNeedsRendering_EmptyTitle(t *testing.T) {
	pd := &parser.PageData{WordCount: 500, H1: []string{"Present"}, Title: ""}
	score := scoreNeedsRendering([]byte(`<html></html>`), pd)
	if score < 2 {
		t.Errorf("empty title: score=%d, want >= 2", score)
	}
}

func TestScoreNeedsRendering_FrameworkMarkers(t *testing.T) {
	markers := []string{
		"__next", "__nuxt", "__NUXT__", "__NEXT_DATA__",
		"data-reactroot", "ng-app", `data-v-`,
		`id="app"`, `id="root"`, `id="__next"`,
	}
	for _, marker := range markers {
		t.Run(marker, func(t *testing.T) {
			body := []byte(`<html>` + marker + `</html>`)
			pd := &parser.PageData{WordCount: 500, H1: []string{"H1"}, Title: "Title"}
			score := scoreNeedsRendering(body, pd)
			if score < 2 {
				t.Errorf("marker %q: score=%d, want >= 2", marker, score)
			}
		})
	}
}

func TestScoreNeedsRendering_Noscript(t *testing.T) {
	body := []byte(`<html><noscript>Enable JS</noscript></html>`)
	pd := &parser.PageData{WordCount: 500, H1: []string{"H1"}, Title: "Title"}
	score := scoreNeedsRendering(body, pd)
	if score < 1 {
		t.Errorf("noscript tag: score=%d, want >= 1", score)
	}
}

func TestScoreNeedsRendering_ManyScriptsLittleText(t *testing.T) {
	body := []byte(`<html>` + strings.Repeat("<script src='x.js'></script>", 6) + `</html>`)
	pd := &parser.PageData{WordCount: 50, H1: []string{"H1"}, Title: "Title"}
	score := scoreNeedsRendering(body, pd)
	// 50 words < 100 with 6 scripts → +2 for scripts, +1 for wordCount 50-150
	if score < 3 {
		t.Errorf("many scripts + little text: score=%d, want >= 3", score)
	}
}

func TestScoreNeedsRendering_Combined(t *testing.T) {
	// Low words + no H1 + no title + framework marker + noscript + many scripts
	body := []byte(`<html><div id="__next"><noscript>JS</noscript>` +
		strings.Repeat("<script></script>", 6) + `</div></html>`)
	pd := &parser.PageData{WordCount: 5}
	score := scoreNeedsRendering(body, pd)
	// 3 (low words) + 2 (no H1) + 2 (no title) + 2 (framework) + 1 (noscript) + 2 (scripts) = 12
	if score < 10 {
		t.Errorf("combined signals: score=%d, want >= 10", score)
	}
}

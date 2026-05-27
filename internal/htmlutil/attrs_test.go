package htmlutil

import (
	"testing"

	"golang.org/x/net/html"
)

func TestNodeAttrCaseInsensitive(t *testing.T) {
	n := &html.Node{
		Attr: []html.Attribute{
			{Key: "hrefLang", Val: "fr"},
			{Key: "DATA-SRC", Val: "/lazy.jpg"},
		},
	}

	got, ok := NodeAttr(n, "hreflang")
	if !ok {
		t.Fatal("NodeAttr() did not find mixed-case hreflang")
	}
	if got != "fr" {
		t.Errorf("NodeAttr() = %q, want %q", got, "fr")
	}

	got, ok = NodeAttr(n, "data-src")
	if !ok {
		t.Fatal("NodeAttr() did not find mixed-case data-src")
	}
	if got != "/lazy.jpg" {
		t.Errorf("NodeAttr() = %q, want %q", got, "/lazy.jpg")
	}
}

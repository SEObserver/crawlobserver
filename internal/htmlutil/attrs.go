package htmlutil

import (
	"strings"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/net/html"
)

func Attr(s *goquery.Selection, name string) (string, bool) {
	if s == nil || len(s.Nodes) == 0 {
		return "", false
	}
	return NodeAttr(s.Nodes[0], name)
}

func AttrOr(s *goquery.Selection, name, defaultValue string) string {
	if v, ok := Attr(s, name); ok {
		return v
	}
	return defaultValue
}

func NodeAttr(n *html.Node, name string) (string, bool) {
	if n == nil {
		return "", false
	}
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val, true
		}
	}
	return "", false
}

func AttrTokenContains(s *goquery.Selection, name, token string) bool {
	v, ok := Attr(s, name)
	if !ok {
		return false
	}
	for _, field := range strings.Fields(v) {
		if strings.EqualFold(field, token) {
			return true
		}
	}
	return false
}

func AttrMediaTypeEqual(s *goquery.Selection, name, want string) bool {
	v, ok := Attr(s, name)
	if !ok {
		return false
	}
	if idx := strings.IndexByte(v, ';'); idx >= 0 {
		v = v[:idx]
	}
	return strings.EqualFold(strings.TrimSpace(v), want)
}
